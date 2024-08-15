package check

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Context struct {
	NumWorkers      int
	Force           bool
	Update          bool
	ShowIgnoredOnly bool
	HashAlgo        string
	SkipSymlinks    bool
	IndexFilename   string
	IgnoreFilename  string
	WorkQueue       chan *WorkItem
	LogQueue        chan *LogEvent
	PerfQueue       chan *PerfEvent
	wg              sync.WaitGroup
}

func NewContext(numWorkers int, force bool, update bool, showIgnoredOnly bool, hashAlgo string, skipSymlinks bool, indexFilename string, ignoreFilename string) (*Context, error) {
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
		NumWorkers:      numWorkers,
		Force:           force,
		Update:          update,
		ShowIgnoredOnly: showIgnoredOnly,
		HashAlgo:        hashAlgo,
		SkipSymlinks:    skipSymlinks,
		IndexFilename:   indexFilename,
		IgnoreFilename:  ignoreFilename,
		WorkQueue:       make(chan *WorkItem, numWorkers*10),
		LogQueue:        make(chan *LogEvent, numWorkers*100),
		PerfQueue:       make(chan *PerfEvent, numWorkers*10),
	}, nil
}

func (context *Context) log(stat Status, message string) {
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

func (context *Context) addWork(path string, filesToIndex []string, ignore *Ignore) {
	context.WorkQueue <- &WorkItem{path, filesToIndex, ignore}
}

func (context *Context) endWork() {
	context.WorkQueue <- nil
}

func (context *Context) isChkbitFile(name string) bool {
	return name == context.IndexFilename || name == context.IgnoreFilename
}

func (context *Context) Start(pathList []string) {
	var wg sync.WaitGroup
	wg.Add(context.NumWorkers)
	for i := 0; i < context.NumWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			context.RunWorker(id)
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

	for _, file := range files {
		path := filepath.Join(root, file.Name())
		if file.Name()[0] == '.' {
			if context.ShowIgnoredOnly && !context.isChkbitFile(file.Name()) {
				context.log(STATUS_IGNORE, path)
			}
			continue
		}
		if isDir(file, path) {
			dirList = append(dirList, file.Name())
		} else if file.Type().IsRegular() {
			filesToIndex = append(filesToIndex, file.Name())
		}
	}

	ignore, err := GetIgnore(context, root, parentIgnore)
	if err != nil {
		context.logErr(root+"/", err)
	}

	context.addWork(root, filesToIndex, ignore)

	for _, name := range dirList {
		if !ignore.shouldIgnore(name) {
			context.scanDir(filepath.Join(root, name), ignore)
		} else {
			context.log(STATUS_IGNORE, name+"/")
		}
	}
}
