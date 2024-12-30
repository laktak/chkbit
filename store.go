package chkbit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

type storeDbItem struct {
	key   []byte
	value []byte
}

type store struct {
	readOnly     bool
	useDb        bool
	refreshDb    bool
	dirty        bool
	dbPath       string
	indexName    string
	dbFile       string
	cacheFileR   string
	cacheFileW   string
	connR        *bolt.DB
	connW        *bolt.DB
	storeDbQueue chan *storeDbItem
	storeDbWg    sync.WaitGroup
	logQueue     chan *LogEvent
}

const (
	dbSuffix       = "-db"
	bakDbSuffix    = ".bak"
	newDbSuffix    = ".new"
	dbTxTimeoutSec = 30
	chkbitDbPrefix = `{"type":"chkbit","version":6,"data":{`
	chkbitDbSuffix = `}}`
)

func (s *store) UseDb(path string, indexName string, refresh bool) {
	s.dbPath = path
	s.indexName = indexName
	s.useDb = true
	s.refreshDb = refresh
}

func (s *store) logErr(message string) {
	s.logQueue <- &LogEvent{StatusPanic, "index-db: " + message}
}

func (s *store) Open(readOnly bool, numWorkers int) error {
	var err error
	s.readOnly = readOnly
	if s.useDb {
		s.dbFile = getDbFile(s.dbPath, s.indexName, "")

		if s.cacheFileR, err = getTempDbFile(s.indexName); err != nil {
			return err
		}
		if err = s.importCache(s.cacheFileR); err != nil {
			return err
		}
		if s.connR, err = bolt.Open(s.cacheFileR, 0600, getBoltOptions(false)); err != nil {
			return err
		}

		if !readOnly {

			// test if the new db file is writeable before failing at the end
			testWrite := getDbFile(s.dbPath, s.indexName, newDbSuffix)
			if file, err := os.Create(testWrite); err != nil {
				return err
			} else {
				defer file.Close()
			}

			if s.refreshDb {
				// write to a new db
				if s.cacheFileW, err = getTempDbFile(s.indexName); err != nil {
					return err
				}
				s.connW, err = bolt.Open(s.cacheFileW, 0600, getBoltOptions(false))
				if err == nil {
					err = s.connW.Update(func(tx *bolt.Tx) error {
						_, err := tx.CreateBucketIfNotExists([]byte("data"))
						return err
					})
				}
			} else {
				s.connW = s.connR
			}

			s.storeDbQueue = make(chan *storeDbItem, numWorkers*10)
			s.storeDbWg.Add(1)
			go s.storeDbWorker()
		}
	}

	return err
}

func (s *store) Finish() (updated bool, err error) {

	if !s.useDb {
		return
	}

	if !s.readOnly {
		s.storeDbQueue <- nil
		s.storeDbWg.Wait()
	}

	if s.connR != nil {
		if err = s.connR.Close(); err != nil {
			return
		}
		if !s.readOnly && s.refreshDb {
			if err = s.connW.Close(); err != nil {
				return
			}
		}
	}
	s.connR = nil
	s.connW = nil

	if !s.readOnly && s.dirty {

		cacheFile := s.cacheFileR
		if s.cacheFileW != "" {
			cacheFile = s.cacheFileW
		}

		var newFile string
		if newFile, err = s.exportCache(cacheFile, newDbSuffix); err != nil {
			return
		}

		if err = os.Rename(s.dbFile, getDbFile(s.dbPath, s.indexName, bakDbSuffix)); err != nil {
			return
		}
		if err = os.Rename(newFile, s.dbFile); err != nil {
			return
		}

		if s.cacheFileR != "" {
			os.Remove(s.cacheFileR)
		}
		if s.cacheFileW != "" {
			os.Remove(s.cacheFileW)
		}
		updated = true
	}
	return
}

func (s *store) Load(indexPath string) ([]byte, error) {
	var err error
	var value []byte
	if s.useDb {
		if s.connR == nil {
			return nil, errors.New("db not loaded")
		}
		err = s.connR.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("data"))
			value = b.Get([]byte(indexPath))
			return nil
		})
	} else {
		if _, err = os.Stat(indexPath); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		value, err = os.ReadFile(indexPath)
	}
	return value, err
}

func (s *store) Save(indexPath string, value []byte) error {
	var err error
	s.dirty = true
	if s.useDb {
		s.storeDbQueue <- &storeDbItem{[]byte(indexPath), value}
	} else {
		// try to preserve the directory mod time but ignore if unsupported
		dirPath := filepath.Dir(indexPath)
		dirStat, dirErr := os.Stat(dirPath)
		err = os.WriteFile(indexPath, value, 0644)
		if dirErr == nil {
			os.Chtimes(dirPath, dirStat.ModTime(), dirStat.ModTime())
		}
	}
	return err
}

func (s *store) storeDbWorker() {

	var tx *bolt.Tx
	var b *bolt.Bucket
	var txExpires time.Time
	var err error
	defer s.storeDbWg.Done()

	for item := range s.storeDbQueue {

		if item == nil {
			break
		}

		if tx != nil && time.Now().After(txExpires) {
			if err = tx.Commit(); err != nil {
				break
			}
			tx = nil
		}

		if tx == nil {
			txExpires = time.Now().Add(dbTxTimeoutSec * time.Second)
			if tx, err = s.connW.Begin(true); err != nil {
				break
			}
			b = tx.Bucket([]byte("data"))
		}

		if err = b.Put(item.key, item.value); err != nil {
			break
		}
	}

	if err != nil {
		s.logErr(err.Error())
	} else if tx != nil {
		if err = tx.Commit(); err != nil {
			s.logErr(err.Error())
		}
	}
}

func (s *store) exportCache(dbFile, suffix string) (exportFile string, err error) {
	connR, err := bolt.Open(dbFile, 0600, getBoltOptions(true))
	if err != nil {
		return
	}
	defer connR.Close()

	exportFile = getDbFile(s.dbPath, s.indexName, suffix)
	file, err := os.Create(exportFile)
	if err != nil {
		return
	}
	defer file.Close()

	// export version 6 database
	if _, err = file.WriteString(chkbitDbPrefix); err != nil {
		return
	}

	if err = connR.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		c := b.Cursor()
		var ierr error
		first := true
		for k, v := c.First(); k != nil; k, v = c.Next() {

			if first {
				first = false
			} else {
				if _, ierr = file.WriteString(","); ierr != nil {
					break
				}
			}

			// remove index filename
			key := filepath.Dir(string(k))
			if key == "." {
				key = ""
			}
			if idxPath, ierr := json.Marshal(key); ierr == nil {
				if _, ierr = file.Write(idxPath); ierr != nil {
					break
				}
			} else {
				break
			}

			if _, ierr = file.WriteString(":"); ierr != nil {
				break
			}

			if _, ierr = file.Write(v); ierr != nil {
				break
			}
		}
		return ierr
	}); err != nil {
		return
	}

	if _, err = file.WriteString(chkbitDbSuffix); err != nil {
		return
	}

	return
}

func (s *store) importCache(dbFile string) error {

	connW, err := bolt.Open(dbFile, 0600, getBoltOptions(false))
	if err != nil {
		return err
	}
	defer connW.Close()
	if err = connW.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists([]byte("data"))
		return err
	}); err != nil {
		return err
	}

	file, err := os.Open(getDbFile(s.dbPath, s.indexName, ""))
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	if t, err := decoder.Token(); err != nil || t != json.Delim('{') {
		return errors.New("invalid json (start)")
	}

	// we only accept our fixed json, in this order:

	// type: chkbit
	var jsonType string
	if t, err := decoder.Token(); err != nil || t != "type" {
		return errors.New("invalid json (type)")
	}
	if err := decoder.Decode(&jsonType); err != nil || jsonType != "chkbit" {
		return errors.New("invalid json (chkbit)")
	}

	// version: 6
	var jsonVersion int
	if t, err := decoder.Token(); err != nil || t != "version" {
		return errors.New("invalid json (version)")
	}
	if err := decoder.Decode(&jsonVersion); err != nil || jsonVersion != 6 {
		return errors.New("invalid json (version 6)")
	}

	// data:
	if t, err := decoder.Token(); err != nil || t != "data" {
		return errors.New("invalid json (data)")
	}
	if t, err := decoder.Token(); err != nil || t != json.Delim('{') {
		return errors.New("invalid json (data start)")
	}

	if err = connW.Update(func(tx *bolt.Tx) error {
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

			// append index filename for compability with file based version
			if key != "" {
				key += "/"
			}
			key += s.indexName

			var value json.RawMessage
			if err = decoder.Decode(&value); err != nil {
				return err
			}

			b := tx.Bucket([]byte("data"))
			if err = b.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if t, err := decoder.Token(); err != nil || t != json.Delim('}') {
		return errors.New("invalid json (end)")
	}

	return err
}

func getDbFile(path, indexFilename, suffix string) string {
	return filepath.Join(path, indexFilename+dbSuffix+suffix)
}

func getTempDbFile(indexFilename string) (string, error) {
	tempFile, err := os.CreateTemp("", "*"+indexFilename)
	if err == nil {
		tempFile.Close()
	}
	return tempFile.Name(), err
}

func getBoltOptions(readOnly bool) *bolt.Options {
	return &bolt.Options{
		ReadOnly:     readOnly,
		Timeout:      0,
		NoGrowSync:   false,
		FreelistType: bolt.FreelistArrayType,
	}
}

func InitializeIndexDb(path, indexName string, force bool) error {
	fileName := getDbFile(path, indexName, "")
	_, err := os.Stat(fileName)
	if !os.IsNotExist(err) {
		if force {
			if err := os.Remove(fileName); err != nil {
				return err
			}
		} else {
			return errors.New("file exists")
		}
	}
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(chkbitDbPrefix + chkbitDbSuffix)
	return err
}

func LocateIndexDb(path, indexName string) (string, error) {
	var err error
	if path, err = filepath.Abs(path); err != nil {
		return "", err
	}
	for {
		file := getDbFile(path, indexName, "")
		_, err = os.Stat(file)
		if !os.IsNotExist(err) {
			return path, nil
		}
		path = filepath.Dir(path)
		if len(path) < 1 || path[len(path)-1] == filepath.Separator {
			// reached root
			return "", errors.New("index db could not be located (forgot to initialize?)")
		}
	}
}
