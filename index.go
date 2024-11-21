package chkbit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

const VERSION = 2 // index version
var (
	algoMd5 = "md5"
)

type idxInfo struct {
	ModTime    int64   `json:"mod"`
	Algo       *string `json:"a,omitempty"`
	Hash       *string `json:"h,omitempty"`
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

type idxInfo1 struct {
	ModTime int64  `json:"mod"`
	Hash    string `json:"md5"`
}

type indexFile1 struct {
	Data map[string]idxInfo1 `json:"data"`
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

func getMtime(path string) int64 {
	info, _ := os.Stat(path)
	return int64(info.ModTime().UnixNano() / 1e6)
}

func (i *Index) getIndexFilepath() string {
	return filepath.Join(i.path, i.context.IndexFilename)
}

func (i *Index) logFilePanic(name string, message string) {
	i.context.log(STATUS_PANIC, filepath.Join(i.path, name)+": "+message)
}

func (i *Index) logFile(stat Status, name string) {
	i.context.log(stat, filepath.Join(i.path, name))
}

func (i *Index) logDir(stat Status, name string) {
	i.context.log(stat, filepath.Join(i.path, name)+"/")
}

func (i *Index) calcHashes(ignore *Ignore) {
	for _, name := range i.files {
		if ignore.shouldIgnore(name) {
			if !ignore.context.isChkbitFile(name) {
				i.logFile(STATUS_IGNORE, name)
			}
			continue
		}

		var err error
		var info *idxInfo
		algo := i.context.HashAlgo
		if val, ok := i.cur[name]; ok {
			// existing file
			if val.LegacyHash != nil {
				// convert from py1 to new format
				val = idxInfo{
					ModTime: val.ModTime,
					Algo:    &algoMd5,
					Hash:    val.LegacyHash,
				}
				i.cur[name] = val
			}
			if val.Algo != nil {
				algo = *val.Algo
			}
			if i.context.AddOnly && !i.mtimeChanged(name, val) {
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
			i.logFile(STATUS_IGNORE, name)
		}
	}
}

func (i *Index) checkFix(forceUpdateDmg bool) {
	for name, b := range i.new {
		if a, ok := i.cur[name]; !ok {
			i.logFile(STATUS_NEW, name)
			i.modified = true
		} else {
			amod := int64(a.ModTime)
			bmod := int64(b.ModTime)
			if a.Hash != nil && b.Hash != nil && *a.Hash == *b.Hash {
				i.logFile(STATUS_OK, name)
				if amod != bmod {
					i.modified = true
				}
				continue
			}

			if amod == bmod {
				i.logFile(STATUS_ERR_DMG, name)
				if !forceUpdateDmg {
					// keep DMG entry
					i.new[name] = a
				} else {
					i.modified = true
				}
			} else if amod < bmod {
				i.logFile(STATUS_UPDATE, name)
				i.modified = true
			} else if amod > bmod {
				i.logFile(STATUS_UP_WARN_OLD, name)
				i.modified = true
			}
		}
	}
	// track missing
	for name := range i.cur {
		if _, ok := i.new[name]; !ok {
			i.modified = true
			if i.context.ShowMissing {
				i.logFile(STATUS_MISSING, name)
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
			if i.context.ShowMissing {
				i.logDir(STATUS_MISSING, name+"/")
			}
		}
	}
	if len(i.newDirList) != len(i.curDirList) {
		// added
		i.modified = true
	}
}

func (i *Index) mtimeChanged(name string, ii idxInfo) bool {
	mtime := getMtime(filepath.Join(i.path, name))
	return ii.ModTime != mtime
}

func (i *Index) calcFile(name string, a string) (*idxInfo, error) {
	path := filepath.Join(i.path, name)
	mtime := getMtime(path)
	h, err := Hashfile(path, a, i.context.perfMonBytes)
	if err != nil {
		return nil, err
	}
	i.context.perfMonFiles(1)
	return &idxInfo{
		ModTime: mtime,
		Algo:    &a,
		Hash:    &h,
	}, nil
}

func (i *Index) save() (bool, error) {
	if i.modified {
		if i.readonly {
			return false, errors.New("Error trying to save a readonly index.")
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
		err = os.WriteFile(i.getIndexFilepath(), file, 0644)
		if err != nil {
			return false, err
		}
		i.modified = false
		return true, nil
	} else {
		return false, nil
	}
}

func (i *Index) load() error {
	if _, err := os.Stat(i.getIndexFilepath()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	i.modified = false
	file, err := os.ReadFile(i.getIndexFilepath())
	if err != nil {
		return err
	}
	var data indexFile
	err = json.Unmarshal(file, &data)
	if err != nil {
		return err
	}
	if data.IdxRaw != nil {
		err = json.Unmarshal(data.IdxRaw, &i.cur)
		if err != nil {
			return err
		}
		text := data.IdxRaw
		if data.IdxHash != hashMd5(text) {
			// old versions may have saved the JSON encoded with extra spaces
			text, _ = json.Marshal(data.IdxRaw)
		} else {
		}
		if data.IdxHash != hashMd5(text) {
			i.modified = true
			i.logFile(STATUS_ERR_IDX, i.getIndexFilepath())
		}
	} else {
		var data1 indexFile1
		json.Unmarshal(file, &data1)
		if data1.Data != nil {
			// convert from js to new format
			for name, item := range data1.Data {
				i.cur[name] = idxInfo{
					ModTime: item.ModTime,
					Algo:    &algoMd5,
					Hash:    &item.Hash,
				}
			}
		}
	}

	// dirs
	if data.Dir != nil {
		slices.Sort(data.Dir)
		i.curDirList = data.Dir
	}

	return nil
}
