package chkbit

import (
	"errors"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

type store struct {
	useDb  bool
	dbPath string
	conn   *bolt.DB
}

const (
	indexDbName = ".chkbitdb"
)

func (s *store) UseDb(path string) error {
	var err error
	s.dbPath, err = LocateIndexDb(path)
	if err == nil {
		s.useDb = true
	}
	return err
}

func (s *store) GetDbFile() string {
	return filepath.Join(s.dbPath, indexDbName)
}

func (s *store) Open(readOnly bool) error {
	var err error
	if s.useDb {
		opt := &bolt.Options{
			ReadOnly:     readOnly,
			Timeout:      0,
			NoGrowSync:   false,
			FreelistType: bolt.FreelistArrayType,
		}
		_, err = os.Stat(s.GetDbFile())
		if os.IsNotExist(err) {
			return nil
		}
		s.conn, err = bolt.Open(s.GetDbFile(), 0600, opt)
		if err == nil && !readOnly {
			err = s.conn.Update(func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("data"))
				return err
			})
		}
	}
	return err
}

func (s *store) Close() {
	if s.useDb && s.conn != nil {
		s.conn.Close()
	}
}

func (s *store) Load(indexPath string) ([]byte, error) {
	var err error
	var value []byte
	if s.useDb {
		if s.conn == nil {
			// readOnly without db
			return nil, nil
		}
		err = s.conn.View(func(tx *bolt.Tx) error {
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
		err = s.conn.Update(func(tx *bolt.Tx) error {
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

func InitializeIndexDb(path string, force bool) error {
	file := filepath.Join(path, indexDbName)
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

func LocateIndexDb(path string) (string, error) {
	var err error
	if path, err = filepath.Abs(path); err != nil {
		return "", err
	}
	for {
		file := filepath.Join(path, indexDbName)
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
