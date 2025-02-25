package chkbit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type fuseStore struct {
	indexName    string
	skipSymlinks bool
	verbose      bool
	store        *indexStore
	count        int
	log          FuseLogFunc
}

type FuseLogFunc func(string)

func FuseIndexStore(path, indexName string, skipSymlinks, verbose bool, log FuseLogFunc) error {
	fileName := getMarkerFile(IndexTypeAtom, path, indexName)
	if _, err := os.Stat(fileName); err != nil {
		return errMissingIndex
	}

	fuse := &fuseStore{
		indexName:    indexName,
		skipSymlinks: skipSymlinks,
		verbose:      verbose,
		log:          log,
	}

	store := &indexStore{
		logErr: func(message string) { fuse.logErr("(indexstore) " + message) },
	}
	fuse.store = store

	store.UseAtom(path, indexName, false)
	if err := store.Open(false, 100); err != nil {
		return err
	}

	fuse.fuseScanDir(path, "")

	if _, err := store.Finish(false); err != nil {
		return err
	}

	fuse.log(fmt.Sprintf("fused %d indexes", fuse.count))
	return nil
}

func (f *fuseStore) logErr(message string) {
	f.log("panic: " + message)
}

func (f *fuseStore) fuseScanDir(root, prefix string) {
	files, err := os.ReadDir(root)
	if err != nil {
		f.logErr(root + "/:" + err.Error())
		return
	}

	for _, file := range files {
		path := filepath.Join(root, file.Name())
		if isDir(file, path, f.skipSymlinks) {
			newPrefix := prefix + file.Name() + "/"
			if fileName, ok, _ := existsMarkerFile(IndexTypeAtom, path, f.indexName); ok {
				if err = f.fuseAtom(fileName, newPrefix); err != nil {
					f.logErr("fuse " + path + "/:" + err.Error())
				}
			} else if fileName, ok, _ := existsMarkerFile(IndexTypeSplit, path, f.indexName); ok {
				if err = f.fuseSplit(fileName, newPrefix); err != nil {
					f.logErr("fuse " + path + "/:" + err.Error())
				}
			}
			f.fuseScanDir(path, newPrefix)
		}
	}
}

func (f *fuseStore) fuseAtom(fileName, prefix string) error {

	if f.verbose {
		f.log("fusing " + fileName)
	}

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	if t, err := decoder.Token(); err != nil || t != json.Delim('{') {
		return errors.New("invalid json (start)")
	}

	if err = verifyAtomJsonHead(decoder); err != nil {
		return err
	}

	for {
		t, err := decoder.Token()
		if err != nil {
			return err
		}
		if t == json.Delim('}') {
			break
		}
		key, ok := t.(string)
		if !ok {
			return errors.New("invalid json (loop)")
		}

		// add prefix + index filename
		if key != "" {
			key += "/"
		}
		key = prefix + key + f.indexName

		// test
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return err
		}

		if err = f.store.Save(key, value); err != nil {
			return err
		}

		f.count++
	}

	if t, err := decoder.Token(); err != nil || t != json.Delim('}') {
		return errors.New("invalid json (end)")
	}

	return nil
}

func (f *fuseStore) fuseSplit(fileName, prefix string) error {
	if f.verbose {
		f.log("fusing " + fileName + " prefix: " + prefix)
	}

	value, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}

	// test
	if _, err = loadIndexFile(value); err != nil {
		return err
	}

	key := prefix + f.indexName

	if err = f.store.Save(key, value); err != nil {
		return err
	}

	f.count++

	return nil
}
