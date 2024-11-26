package chkbit

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type indexDb struct {
	useSql bool
	conn   *sql.DB
}

func (db *indexDb) GetDbPath() string {
	return ".chkbitdb"
}

func (db *indexDb) Open() error {
	var err error
	if db.useSql {
		db.conn, err = sql.Open("sqlite3", db.GetDbPath())
		if err != nil {
			return err
		}

		create := `create table if not exists data (
			key text primary key,
			value blob
		);`
		_, err = db.conn.Exec(create)
	}
	return err
}

func (db *indexDb) Close() {
	if db.useSql {
		db.conn.Close()
	}
}

func (db *indexDb) Load(indexPath string) ([]byte, error) {
	var err error
	var value []byte
	if db.useSql {
		err = db.conn.QueryRow("select value from data where key = ?", indexPath).Scan(&value)
		if err == sql.ErrNoRows {
			return nil, nil
		}

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
	if db.useSql {
		insert := "insert or replace into data (key, value) values (?, ?)"
		_, err = db.conn.Exec(insert, indexPath, value)
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
