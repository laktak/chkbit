package chkbit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const VERSION = 2 // index version
var (
	algoMd5 = "md5"
)

type IdxInfo struct {
	ModTime    int64   `json:"mod"`
	Algo       *string `json:"a,omitempty"`
	Hash       *string `json:"h,omitempty"`
	LegacyHash *string `json:"md5,omitempty"`
}

type IndexFile struct {
	V       int             `json:"v"`
	IdxRaw  json.RawMessage `json:"idx"`
	IdxHash string          `json:"idx_hash"`
}

type IdxInfo1 struct {
	ModTime int64  `json:"mod"`
	Hash    string `json:"md5"`
}

type IndexFile1 struct {
	Data map[string]IdxInfo1 `json:"data"`
}

type Index struct {
	context  *Context
	path     string
	files    []string
	cur      map[string]IdxInfo
	new      map[string]IdxInfo
	updates  []string
	modified bool
	readonly bool
}

func NewIndex(context *Context, path string, files []string, readonly bool) *Index {
	return &Index{
		context:  context,
		path:     path,
		files:    files,
		cur:      make(map[string]IdxInfo),
		new:      make(map[string]IdxInfo),
		readonly: readonly,
	}
}

func (i *Index) getIndexFilepath() string {
	return filepath.Join(i.path, i.context.IndexFilename)
}

func (i *Index) setMod(value bool) {
	i.modified = value
}

func (i *Index) logFilePanic(name string, message string) {
	i.context.log(STATUS_PANIC, filepath.Join(i.path, name)+": "+message)
}

func (i *Index) logFile(stat Status, name string) {
	i.context.log(stat, filepath.Join(i.path, name))
}

func (i *Index) calcHashes(ignore *Ignore) {
	for _, name := range i.files {
		if ignore != nil && ignore.shouldIgnore(name) {
			i.logFile(STATUS_IGNORE, name)
			continue
		}

		var err error
		var info *IdxInfo
		algo := i.context.HashAlgo
		if val, ok := i.cur[name]; ok {
			// existing
			if val.LegacyHash != nil {
				// convert from py1 to new format
				val = IdxInfo{
					ModTime: val.ModTime,
					Algo:    &algoMd5,
					Hash:    val.LegacyHash,
				}
				i.cur[name] = val
			}
			if val.Algo != nil {
				algo = *val.Algo
			}
			info, err = i.calcFile(name, algo)
		} else {
			if i.readonly {
				info = &IdxInfo{Algo: &algo}
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

func (i *Index) checkFix(force bool) {
	for name, b := range i.new {
		if a, ok := i.cur[name]; !ok {
			i.logFile(STATUS_NEW, name)
			i.setMod(true)
		} else {
			amod := int64(a.ModTime)
			bmod := int64(b.ModTime)
			if a.Hash != nil && b.Hash != nil && *a.Hash == *b.Hash {
				i.logFile(STATUS_OK, name)
				if amod != bmod {
					i.setMod(true)
				}
				continue
			}

			if amod == bmod {
				i.logFile(STATUS_ERR_DMG, name)
				if !force {
					i.new[name] = a
				} else {
					i.setMod(true)
				}
			} else if amod < bmod {
				i.logFile(STATUS_UPDATE, name)
				i.setMod(true)
			} else if amod > bmod {
				i.logFile(STATUS_UP_WARN_OLD, name)
				i.setMod(true)
			}
		}
	}
}

func (i *Index) calcFile(name string, a string) (*IdxInfo, error) {
	path := filepath.Join(i.path, name)
	info, _ := os.Stat(path)
	mtime := int64(info.ModTime().UnixNano() / 1e6)
	h, err := Hashfile(path, a, i.context.perfMonBytes)
	if err != nil {
		return nil, err
	}
	i.context.perfMonFiles(1)
	return &IdxInfo{
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
		data := IndexFile{
			V:       VERSION,
			IdxRaw:  text,
			IdxHash: HashMd5(text),
		}

		file, err := json.Marshal(data)
		if err != nil {
			return false, err
		}
		err = os.WriteFile(i.getIndexFilepath(), file, 0644)
		if err != nil {
			return false, err
		}
		i.setMod(false)
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
	i.setMod(false)
	file, err := os.ReadFile(i.getIndexFilepath())
	if err != nil {
		return err
	}
	var data IndexFile
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
		if data.IdxHash != HashMd5(text) {
			// old versions may have save the JSON encoded with extra spaces
			text, _ = json.Marshal(data.IdxRaw)
		} else {
		}
		if data.IdxHash != HashMd5(text) {
			i.setMod(true)
			i.logFile(STATUS_ERR_IDX, i.getIndexFilepath())
		}
	} else {
		var data1 IndexFile1
		json.Unmarshal(file, &data1)
		if data1.Data != nil {
			// convert from js to new format
			for name, item := range data1.Data {
				i.cur[name] = IdxInfo{
					ModTime: item.ModTime,
					Algo:    &algoMd5,
					Hash:    &item.Hash,
				}
			}
		}
	}
	return nil
}
