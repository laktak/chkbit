package chkbit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	bolt "go.etcd.io/bbolt"
)

var (
	ddStatusBucketName = []byte("status")
	ddStatusName       = []byte("1")
	ddItemBucketName   = []byte("item")
)

type dedup struct {
	rootPath  string
	indexName string
	status    ddStatus
	minSize   int64

	conn *bolt.DB

	logQueue chan *LogEvent
}

type ddStatus struct {
	Gen int `json:"gen"`
}

type ddBag struct {
	Gen      int       `json:"gen"`
	Size     int64     `json:"s"`
	ItemList []*ddItem `json:"item,omitempty"`
}

type ddItem struct {
	Path   string `json:"path"`
	Merged bool   `json:"merged"`
}

const (
	dedupSuffix = "-dedup.db"
)

func (d *dedup) logErr(message string) {
	d.logQueue <- &LogEvent{StatusPanic, "dedup: " + message}
}

func getDedupFile(path, indexName, suffix string) string {
	return filepath.Join(path, indexName+dedupSuffix+suffix)
}

func (d *dedup) Open(path string, indexName string) error {
	var err error
	d.rootPath = path
	d.indexName = indexName
	dedupFile := getDedupFile(path, indexName, d.indexName)

	d.conn, err = bolt.Open(dedupFile, 0600, getBoltOptions(false))
	if err != nil {
		return err
	}
	err = d.conn.Update(func(tx *bolt.Tx) error {
		sb, err := tx.CreateBucketIfNotExists(ddStatusBucketName)
		if err != nil {
			return err
		}

		status := sb.Get(ddStatusName)
		if status != nil {
			json.Unmarshal(status, &d.status)
		}

		_, err = tx.CreateBucketIfNotExists(ddItemBucketName)
		return err
	})
	return err
}

func (d *dedup) nextGen(tx *bolt.Tx) (err error) {
	if sb := tx.Bucket(ddStatusBucketName); sb != nil {
		d.status.Gen += 1
		if data, err := json.Marshal(&d.status); err == nil {
			err = sb.Put(ddStatusName, data)
		}
	}
	return
}

func (d *dedup) Finish() error {
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			return err
		}
	}
	d.conn = nil
	return nil
}

func (d *dedup) DetectDupes() (err error) {

	file, err := os.Open(getAtomFile(d.rootPath, d.indexName, ""))
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

	all := make(map[string]*ddBag)
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

		if key != "" {
			key += "/"
		}

		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return err
		}

		index, err := loadIndexFile(value)
		if err != nil {
			return err
		}

		for k, v := range index.fileList {

			if v.Size != nil && *v.Size >= 0 && *v.Size < d.minSize {
				continue
			}

			bag := all[*v.Hash]
			if bag == nil {
				bag = &ddBag{
					Size: -1,
				}
			}
			if bag.Size == -1 && v.Size != nil && *v.Size >= 0 {
				bag.Size = *v.Size
			}
			bag.ItemList = append(bag.ItemList,
				&ddItem{
					Path: key + k,
				})
			all[*v.Hash] = bag
		}
	}

	if t, err := decoder.Token(); err != nil || t != json.Delim('}') {
		return errors.New("invalid json (end)")
	}

	if err = d.conn.Update(func(tx *bolt.Tx) error {

		if err := d.nextGen(tx); err != nil {
			return err
		}

		b := tx.Bucket(ddItemBucketName)

		for hash, item := range all {
			if len(item.ItemList) <= 1 {
				continue
			}

			// combine with old status
			itemData := b.Get([]byte(hash))
			if itemData != nil {
				var item2 ddBag
				err := json.Unmarshal(itemData, &item2)
				if err != nil {
					for _, o := range item2.ItemList {
						for _, p := range item.ItemList {
							if o.Path == p.Path {
								p.Merged = o.Merged
							}
						}
					}
				}
			}

			item.Gen = d.status.Gen
			if data, err := json.Marshal(item); err != nil {
				return err
			} else {
				if err = b.Put([]byte(hash), data); err != nil {
					return err
				}
			}
		}

		// remove old gen (don't use c.Delete())
		c := b.Cursor()
		del := [][]byte{}
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bag ddBag
			if err := json.Unmarshal(v, &bag); err != nil {
				return err
			}
			if bag.Gen != d.status.Gen {
				del = append(del, k)
			}
		}
		for _, k := range del {
			if err := b.Delete(k); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (d *dedup) Show() error {
	var list []*ddBag
	if err := d.conn.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(ddItemBucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bag ddBag
			if err := json.Unmarshal(v, &bag); err != nil {
				return err
			}
			list = append(list, &bag)
		}
		return nil
	}); err != nil {
		return err
	}
	slices.SortFunc(list, func(a, b *ddBag) int {
		r := b.Size - a.Size
		if r < 0 {
			return -1
		} else if r > 0 {
			return 1
		} else {
			return 0
		}
	})
	for i, bag := range list {
		fmt.Println("#", i, bag.Size)
		for _, item := range bag.ItemList {
			fmt.Println("-", item.Path, item.Merged)
		}
	}
	return nil
}

func DedupDetect(path string, indexName string) error {
	d := &dedup{minSize: 8192}
	if err := d.Open(path, indexName); err != nil {
		return err
	}
	defer d.Finish()
	return d.DetectDupes()
}

func DedupShow(path string, indexName string) error {
	d := &dedup{}
	if err := d.Open(path, indexName); err != nil {
		return err
	}
	defer d.Finish()
	return d.Show()
}
