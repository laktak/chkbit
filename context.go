package chkbit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Context struct {
	NumWorkers         int
	UpdateIndex        bool
	AddOnly            bool
	ShowIgnoredOnly    bool
	ShowMissing        bool
	IncludeDot         bool
	ForceUpdateDmg     bool
	HashAlgo           string
	TrackDirectories   bool
	SkipSymlinks       bool
	SkipSubdirectories bool
	IndexFilename      string
	IgnoreFilename     string

	WorkQueue chan *WorkItem
	LogQueue  chan *LogEvent
	PerfQueue chan *PerfEvent

	store *store

	mutex     sync.Mutex
	NumTotal  int
	NumIdxUpd int
	NumNew    int
	NumUpd    int
	NumDel    int
}

func NewContext(numWorkers int, hashAlgo string, indexFilename string, ignoreFilename string) (*Context, error) {
	if indexFilename[0] != '.' {
		return nil, errors.New("the index filename must start with a dot")
	}
	if ignoreFilename[0] != '.' {
		return nil, errors.New("the ignore filename must start with a dot")
	}
	if hashAlgo != "md5" && hashAlgo != "sha512" && hashAlgo != "blake3" {
		return nil, errors.New(hashAlgo + " is unknown")
	}
	if numWorkers < 1 {
		return nil, errors.New("expected numWorkers >= 1")
	}
	logQueue := make(chan *LogEvent, numWorkers*100)
	return &Context{
		NumWorkers:     numWorkers,
		HashAlgo:       hashAlgo,
		IndexFilename:  indexFilename,
		IgnoreFilename: ignoreFilename,
		WorkQueue:      make(chan *WorkItem, numWorkers*10),
		LogQueue:       logQueue,
		PerfQueue:      make(chan *PerfEvent, numWorkers*10),
		store:          &store{logQueue: logQueue},
	}, nil
}

func (context *Context) log(stat Status, message string) {
	context.mutex.Lock()
	defer context.mutex.Unlock()
	switch stat {
	case StatusErrorDamage:
		context.NumTotal++
	case StatusUpdateIndex:
		context.NumIdxUpd++
	case StatusUpdateWarnOld:
		context.NumTotal++
		context.NumUpd++
	case StatusUpdate:
		context.NumTotal++
		context.NumUpd++
	case StatusNew:
		context.NumTotal++
		context.NumNew++
	case StatusOK:
		if !context.AddOnly {
			context.NumTotal++
		}
	case StatusMissing:
		context.NumDel++
	}

	context.LogQueue <- &LogEvent{stat, message}
}

func (context *Context) logErr(path string, err error) {
	context.log(StatusPanic, path+": "+err.Error())
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
	// any file with the index prefix is ignored (to allow for .bak and db files)
	return strings.HasPrefix(name, context.IndexFilename) || name == context.IgnoreFilename
}

func (context *Context) Process(pathList []string) {
	context.NumTotal = 0
	context.NumIdxUpd = 0
	context.NumNew = 0
	context.NumUpd = 0
	context.NumDel = 0

	err := context.store.Open(!context.UpdateIndex, context.NumWorkers)
	if err != nil {
		context.logErr("index", err)
		context.LogQueue <- nil
		return
	}

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

	if updated, err := context.store.Finish(); err != nil {
		context.logErr("index", err)
	} else if updated {
		context.log(StatusInfo, "The index db was updated")
	}
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
		if isDir(file, path) {
			if !ignore.shouldIgnore(file.Name()) {
				dirList = append(dirList, file.Name())
			} else {
				context.log(StatusIgnore, file.Name()+"/")
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

func (context *Context) UseStoreDb(pathList []string) (root string, relativePathList []string, err error) {

	if len(pathList) == 0 {
		return "", nil, errors.New("missing path(s)")
	}
	root, err = LocateIndexDb(pathList[0], context.IndexFilename)
	if err == nil {

		for _, path := range pathList {
			path, err = filepath.Abs(path)
			if err != nil {
				return "", nil, err
			}

			// below root?
			if !strings.HasPrefix(path, root) {
				return "", nil, fmt.Errorf("path %s is not below the store-db in %s", path, root)
			}

			relativePath, err := filepath.Rel(root, path)
			if err != nil {
				return "", nil, err
			}
			relativePathList = append(relativePathList, relativePath)
		}

		context.store.UseDb(root, context.IndexFilename, len(relativePathList) == 1 && relativePathList[0] == ".")
	}

	return
}
