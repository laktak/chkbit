package chkbit

import (
	"encoding/json"
	"errors"
	"os"
	slpath "path"
	"slices"
)

const VERSION = 2 // index version

type idxInfo struct {
	ModTime int64   `json:"mod"`
	Algo    *string `json:"a,omitempty"`
	Hash    *string `json:"h,omitempty"`
	// 2025-02-16
	Size *int64 `json:"s,omitempty"`
	// legacy python format
	LegacyHash *string `json:"md5,omitempty"`
}

type indexFile struct {
	V int `json:"v"`
	// IdxRaw -> map[string]idxInfo
	IdxRaw  json.RawMessage `json:"idx"`
	IdxHash string          `json:"idx_hash"`
	// 2024-08 optional, list of subdirectories
	Dir []string `json:"dirlist,omitempty"`
}

type indexLoadResult struct {
	fileList  map[string]idxInfo
	dirList   []string
	converted bool
	verified  bool
}

type Index struct {
	context    *Context
	path       string
	files      []string
	cur        map[string]idxInfo
	new        map[string]idxInfo
	curDirList []string
	newDirList []string
	modified   bool
	readonly   bool
}

func newIndex(context *Context, path string, files []string, dirList []string, readonly bool) *Index {
	slices.Sort(dirList)
	return &Index{
		context:    context,
		path:       path,
		files:      files,
		cur:        make(map[string]idxInfo),
		new:        make(map[string]idxInfo),
		curDirList: make([]string, 0),
		newDirList: dirList,
		readonly:   readonly,
	}
}

func getMtS(path string) (mtime, size int64, err error) {
	var info os.FileInfo
	if info, err = os.Stat(path); err == nil {
		mtime = int64(info.ModTime().UnixNano() / 1e6)
		size = info.Size()
	}
	return
}

func (i *Index) getIndexFilepath() string {
	return slpath.Join(i.path, i.context.IndexFilename)
}

func (i *Index) logFilePanic(name string, message string) {
	i.context.log(StatusPanic, slpath.Join(i.path, name)+": "+message)
}

func (i *Index) logFile(stat Status, name string) {
	i.context.log(stat, slpath.Join(i.path, name))
}

func (i *Index) logDir(stat Status, name string) {
	i.context.log(stat, slpath.Join(i.path, name)+"/")
}

func (i *Index) calcHashes(ignore *Ignore) {
	for _, name := range i.files {
		if ignore.shouldIgnore(name) {
			if !ignore.context.isChkbitFile(name) {
				i.logFile(StatusIgnore, name)
			}
			continue
		}

		var err error
		var info *idxInfo
		algo := i.context.HashAlgo
		if val, ok := i.cur[name]; ok {
			// existing file
			if val.Algo != nil {
				algo = *val.Algo
			}
			if i.context.UpdateSkipCheck && !i.mtimeChanged(name, val) {
				info = &val
			} else {
				info, err = i.calcFile(name, algo)
			}
		} else {
			// new file
			if i.readonly {
				info = &idxInfo{Algo: &algo}
			} else {
				info, err = i.calcFile(name, algo)
			}
		}
		if err != nil {
			i.logFilePanic(name, err.Error())
		} else {
			i.new[name] = *info
		}
	}
}

func (i *Index) showIgnoredOnly(ignore *Ignore) {
	for _, name := range i.files {
		if ignore.shouldIgnore(name) {
			i.logFile(StatusIgnore, name)
		}
	}
}

func (i *Index) checkFix(forceUpdateDmg bool) {
	for name, b := range i.new {
		if a, ok := i.cur[name]; !ok {
			i.logFile(StatusNew, name)
			i.modified = true
		} else {
			amod := int64(a.ModTime)
			bmod := int64(b.ModTime)
			if a.Hash != nil && b.Hash != nil && *a.Hash == *b.Hash {
				i.logFile(StatusOK, name)
				if amod != bmod {
					i.modified = true
				}
				continue
			}

			if amod == bmod {
				i.logFile(StatusErrorDamage, name)
				if !forceUpdateDmg {
					// keep DMG entry
					i.new[name] = a
				} else {
					i.modified = true
				}
			} else if amod < bmod {
				i.logFile(StatusUpdate, name)
				i.modified = true
			} else if amod > bmod {
				i.logFile(StatusUpdateWarnOld, name)
				i.modified = true
			}
		}
	}
	// track deleted files
	for name := range i.cur {
		if _, ok := i.new[name]; !ok {
			i.modified = true
			if i.context.LogDeleted {
				i.logFile(StatusDeleted, name)
			}
		}
	}

	// dirs
	m := make(map[string]bool)
	for _, n := range i.newDirList {
		m[n] = true
	}
	for _, name := range i.curDirList {
		if !m[name] {
			i.modified = true
			if i.context.LogDeleted {
				i.logDir(StatusDeleted, name+"/")
			}
		}
	}
	if len(i.newDirList) != len(i.curDirList) {
		// added
		i.modified = true
	}
}

func (i *Index) mtimeChanged(name string, ii idxInfo) bool {
	mtime, _, _ := getMtS(slpath.Join(i.path, name))
	return ii.ModTime != mtime
}

func (i *Index) calcFile(name string, algo string) (*idxInfo, error) {
	path := slpath.Join(i.path, name)
	mtime, size, err := getMtS(path)
	if err != nil {
		return nil, err
	}
	hash, err := Hashfile(path, algo, i.context.perfMonBytes)
	if err != nil {
		return nil, err
	}
	i.context.perfMonFiles(1)
	return &idxInfo{
		ModTime: mtime,
		Algo:    &algo,
		Hash:    &hash,
		Size:    &size,
	}, nil
}

func (i *Index) save() (bool, error) {
	if i.modified || !i.readonly && i.context.store.refresh {
		if i.readonly {
			return false, errors.New("error trying to save a readonly index")
		}

		text, err := json.Marshal(i.new)
		if err != nil {
			return false, err
		}
		data := indexFile{
			V:       VERSION,
			IdxRaw:  text,
			IdxHash: hashMd5(text),
		}
		if i.context.TrackDirectories {
			data.Dir = i.newDirList
		}

		file, err := json.Marshal(data)
		if err != nil {
			return false, err
		}

		err = i.context.store.Save(i.getIndexFilepath(), file)
		if err != nil {
			return false, err
		}

		// only report actual modifications
		if i.modified {
			i.modified = false
			return true, nil
		}
	}
	return false, nil
}

func (i *Index) load() error {
	fileData, err := i.context.store.Load(i.getIndexFilepath())
	if fileData == nil || err != nil {
		return err
	}
	i.modified = false

	res, err := loadIndexFile(fileData)
	if err != nil {
		return err
	}
	i.cur = res.fileList
	if !res.verified {
		i.logFile(StatusErrorIdx, i.getIndexFilepath())
	}
	i.modified = !res.verified || res.converted

	// dirs
	if res.dirList != nil {
		slices.Sort(res.dirList)
		i.curDirList = res.dirList
	}

	return nil
}

func loadIndexFile(fileData []byte) (*indexLoadResult, error) {

	type idxInfo1 struct {
		ModTime int64  `json:"mod"`
		Hash    string `json:"md5"`
	}

	type indexFile1 struct {
		Data map[string]idxInfo1 `json:"data"`
	}

	var legacyAlgoMd5 = "md5"

	if fileData == nil {
		return nil, errors.New("fileData is nil")
	}
	res := &indexLoadResult{}

	var data indexFile
	err := json.Unmarshal(fileData, &data)
	if err != nil {
		return nil, err
	}
	if data.IdxRaw != nil {
		err = json.Unmarshal(data.IdxRaw, &res.fileList)
		if err != nil {
			return nil, err
		}
		text := data.IdxRaw
		if data.IdxHash != hashMd5(text) {
			// old versions may have saved the JSON encoded with extra spaces
			text, _ = json.Marshal(data.IdxRaw)
		}
		res.verified = data.IdxHash == hashMd5(text)

		// convert from py1/md5 to new format
		for name, item := range res.fileList {
			if item.LegacyHash != nil {
				item2 := idxInfo{
					ModTime: item.ModTime,
					Algo:    &legacyAlgoMd5,
					Hash:    item.LegacyHash,
				}
				res.fileList[name] = item2
			}
		}
	} else {
		var data1 indexFile1
		json.Unmarshal(fileData, &data1)
		res.fileList = make(map[string]idxInfo)
		if data1.Data != nil {
			// convert from js to new format
			for name, item := range data1.Data {
				res.fileList[name] = idxInfo{
					ModTime: item.ModTime,
					Algo:    &legacyAlgoMd5,
					Hash:    &item.Hash,
				}
			}
		}
		res.converted = true
		res.verified = true
	}

	res.dirList = data.Dir
	return res, nil
}
