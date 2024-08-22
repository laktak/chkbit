package chkbit

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Context struct {
	NumWorkers         int
	UpdateIndex        bool
	AddOnly            bool
	ShowIgnoredOnly    bool
	ShowMissing        bool
	ForceUpdateDmg     bool
	HashAlgo           string
	TrackDirectories   bool
	SkipSymlinks       bool
	SkipSubdirectories bool
	IndexFilename      string
	IgnoreFilename     string
	WorkQueue          chan *WorkItem
	LogQueue           chan *LogEvent
	PerfQueue          chan *PerfEvent
	wg                 sync.WaitGroup

	mutex     sync.Mutex
	NumTotal  int
	NumIdxUpd int
	NumNew    int
	NumUpd    int
	NumDel    int
}

func NewContext(numWorkers int, hashAlgo string, indexFilename string, ignoreFilename string) (*Context, error) {
	if indexFilename[0] != '.' {
		return nil, errors.New("The index filename must start with a dot!")
	}
	if ignoreFilename[0] != '.' {
		return nil, errors.New("The ignore filename must start with a dot!")
	}
	if hashAlgo != "md5" && hashAlgo != "sha512" && hashAlgo != "blake3" {
		return nil, errors.New(hashAlgo + " is unknown.")
	}
	return &Context{
		NumWorkers:     numWorkers,
		HashAlgo:       hashAlgo,
		IndexFilename:  indexFilename,
		IgnoreFilename: ignoreFilename,
		WorkQueue:      make(chan *WorkItem, numWorkers*10),
		LogQueue:       make(chan *LogEvent, numWorkers*100),
		PerfQueue:      make(chan *PerfEvent, numWorkers*10),
	}, nil
}

func (context *Context) log(stat Status, message string) {
	context.mutex.Lock()
	defer context.mutex.Unlock()
	switch stat {
	case STATUS_ERR_DMG:
		context.NumTotal++
	case STATUS_UPDATE_INDEX:
		context.NumIdxUpd++
	case STATUS_UP_WARN_OLD:
		context.NumTotal++
		context.NumUpd++
	case STATUS_UPDATE:
		context.NumTotal++
		context.NumUpd++
	case STATUS_NEW:
		context.NumTotal++
		context.NumNew++
	case STATUS_OK:
		if !context.AddOnly {
			context.NumTotal++
		}
	case STATUS_MISSING:
		context.NumDel++
		//case STATUS_PANIC:
		//case STATUS_ERR_IDX:
		//case STATUS_IGNORE:
	}

	context.LogQueue <- &LogEvent{stat, message}
}

func (context *Context) logErr(path string, err error) {
	context.LogQueue <- &LogEvent{STATUS_PANIC, path + ": " + err.Error()}
}

func (context *Context) perfMonFiles(numFiles int64) {
	context.PerfQueue <- &PerfEvent{numFiles, 0}
}

func (context *Context) perfMonBytes(numBytes int64) {
	context.PerfQueue <- &PerfEvent{0, numBytes}
}

func (context *Context) addWork(path string, filesToIndex []string, dirList []string, ignore *Ignore) {
	context.WorkQueue <- &WorkItem{path, filesToIndex, dirList, ignore}
}

func (context *Context) endWork() {
	context.WorkQueue <- nil
}

func (context *Context) isChkbitFile(name string) bool {
	return name == context.IndexFilename || name == context.IgnoreFilename
}

func (context *Context) Start(pathList []string) {
	context.NumTotal = 0
	context.NumIdxUpd = 0
	context.NumNew = 0
	context.NumUpd = 0
	context.NumDel = 0

	var wg sync.WaitGroup
	wg.Add(context.NumWorkers)
	for i := 0; i < context.NumWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			context.runWorker(id)
		}(i)
	}
	go func() {
		for _, path := range pathList {
			context.scanDir(path, nil)
		}
		for i := 0; i < context.NumWorkers; i++ {
			context.endWork()
		}
	}()
	wg.Wait()
	context.LogQueue <- nil
}

func (context *Context) scanDir(root string, parentIgnore *Ignore) {
	files, err := os.ReadDir(root)
	if err != nil {
		context.logErr(root+"/", err)
		return
	}

	isDir := func(file os.DirEntry, path string) bool {
		if file.IsDir() {
			return true
		}
		ft := file.Type()
		if !context.SkipSymlinks && ft&os.ModeSymlink != 0 {
			rpath, err := filepath.EvalSymlinks(path)
			if err == nil {
				fi, err := os.Lstat(rpath)
				return err == nil && fi.IsDir()
			}
		}
		return false
	}

	var dirList []string
	var filesToIndex []string

	ignore, err := GetIgnore(context, root, parentIgnore)
	if err != nil {
		context.logErr(root+"/", err)
	}

	for _, file := range files {
		path := filepath.Join(root, file.Name())
		if file.Name()[0] == '.' {
			if context.ShowIgnoredOnly && !context.isChkbitFile(file.Name()) {
				context.log(STATUS_IGNORE, path)
			}
			continue
		}
		if isDir(file, path) {
			if !ignore.shouldIgnore(file.Name()) {
				dirList = append(dirList, file.Name())
			} else {
				context.log(STATUS_IGNORE, file.Name()+"/")
			}
		} else if file.Type().IsRegular() {
			filesToIndex = append(filesToIndex, file.Name())
		}
	}

	context.addWork(root, filesToIndex, dirList, ignore)

	if !context.SkipSubdirectories {
		for _, name := range dirList {
			context.scanDir(filepath.Join(root, name), ignore)
		}
	}
}
