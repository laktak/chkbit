package chkbit

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

type store struct {
	readOnly  bool
	useDb     bool
	refreshDb bool
	dbPath    string
	indexName string
	dbFile    string
	newFile   string
	connR     *bolt.DB
	connW     *bolt.DB
}

const (
	dbSuffix    = "-db"
	bakDbSuffix = ".bak"
	newDbSuffix = ".new"
)

func (s *store) UseDb(path string, indexName string, refresh bool) {
	s.dbPath = path
	s.indexName = indexName
	s.useDb = true
	s.refreshDb = refresh
}

func (s *store) Open(readOnly bool) error {
	var err error
	s.readOnly = readOnly
	if s.useDb {
		s.dbFile = getDbFile(s.dbPath, s.indexName, "")
		s.connR, err = bolt.Open(s.dbFile, 0600, getBoltOptions(true))

		if !readOnly {
			s.newFile = getDbFile(s.dbPath, s.indexName, newDbSuffix)

			if s.refreshDb {
				err = clearFile(s.newFile)
			} else {
				err = copyFile(s.dbFile, s.newFile)
			}
			if err != nil {
				return err
			}

			s.connW, err = bolt.Open(s.newFile, 0600, getBoltOptions(false))
			if err == nil {
				err = s.connW.Update(func(tx *bolt.Tx) error {
					_, err := tx.CreateBucketIfNotExists([]byte("data"))
					return err
				})
			}
		}
	}
	return err
}

func (s *store) Close() {
	if s.useDb {
		if s.connW != nil {
			s.connW.Close()
		}
		if s.connR != nil {
			s.connR.Close()
		}
	}
}

func (s *store) Finish() error {
	if s.useDb && !s.readOnly {
		bakFile := getDbFile(s.dbPath, s.indexName, bakDbSuffix)
		err := os.Rename(s.dbFile, bakFile)
		if err != nil {
			return err
		}
		err = os.Rename(s.newFile, s.dbFile)
		if err != nil {
			return err
		}
	}
	return nil
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
	if s.useDb {
		err = s.connW.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("data"))
			return b.Put([]byte(indexPath), value)
		})
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

func InitializeIndexDb(path, indexName string, force bool) error {
	file := getDbFile(path, indexName, "")
	_, err := os.Stat(file)
	if !os.IsNotExist(err) {
		if force {
			err := os.Remove(file)
			if err != nil {
				return err
			}
		} else {
			return errors.New("file exists")
		}

	}
	conn, err := bolt.Open(file, 0600, nil)
	if err != nil {
		return err
	}
	return conn.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("data"))
		return err
	})
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

func ExportIndexDb(path, indexName string) error {

	dbFile := getDbFile(path, indexName, "")
	connR, err := bolt.Open(dbFile, 0600, getBoltOptions(true))
	if err != nil {
		return err
	}
	defer connR.Close()

	file, err := os.Create(getDbFile(path, indexName, ".json"))
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(`{"type":"chkbit","version":6,"data":{`)
	if err != nil {
		return err
	}

	err = connR.View(func(tx *bolt.Tx) error {
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
	})

	_, err = file.WriteString("}}")
	if err != nil {
		return err
	}

	return err
}

func getDbFile(path, indexFilename, suffix string) string {
	return filepath.Join(path, indexFilename+dbSuffix+suffix)
}

func clearFile(file string) error {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		return nil
	}
	return os.Remove(file)
}

func copyFile(src, dst string) error {

	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	return err
}

func getBoltOptions(readOnly bool) *bolt.Options {
	return &bolt.Options{
		ReadOnly:     readOnly,
		Timeout:      0,
		NoGrowSync:   false,
		FreelistType: bolt.FreelistArrayType,
	}
}
