package chkbit

import (
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

type indexDb struct {
	useSingleDb bool
	conn        *bolt.DB
}

func (db *indexDb) GetDbPath() string {
	return ".chkbitdb"
}

func (db *indexDb) Open(useSingleDb, readOnly bool) error {
	var err error
	db.useSingleDb = useSingleDb
	if useSingleDb {
		opt := &bolt.Options{
			ReadOnly:     readOnly,
			Timeout:      0,
			NoGrowSync:   false,
			FreelistType: bolt.FreelistArrayType,
		}
		if readOnly {
			_, err := os.Stat(db.GetDbPath())
			if os.IsNotExist(err) {
				return nil
			}
		}
		// todo: write to new db
		db.conn, err = bolt.Open(db.GetDbPath(), 0600, opt)
		if err == nil && !readOnly {
			err = db.conn.Update(func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("data"))
				return err
			})
		}
	}
	return err
}

func (db *indexDb) Close() {
	if db.useSingleDb && db.conn != nil {
		db.conn.Close()
	}
}

func (db *indexDb) Load(indexPath string) ([]byte, error) {
	var err error
	var value []byte
	if db.useSingleDb {
		if db.conn == nil {
			// readOnly without db
			return nil, nil
		}
		err = db.conn.View(func(tx *bolt.Tx) error {
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

func (db *indexDb) Save(indexPath string, value []byte) error {
	var err error
	if db.useSingleDb {
		err = db.conn.Update(func(tx *bolt.Tx) error {
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
