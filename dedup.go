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

type dedup struct {
	rootPath  string
	indexName string

	LogChan chan *DedupEvent

	status ddStatus
	conn   *bolt.DB
}

type ddStatus struct {
	Gen int `json:"gen"`
}

type ddBag struct {
	Gen      int          `json:"gen"`
	Size     int64        `json:"size"`
	ItemList []*DedupItem `json:"item"`
}

type DedupBag struct {
	Hash     string       `json:"hash"`
	Size     int64        `json:"size"`
	ItemList []*DedupItem `json:"item"`
}

type DedupItem struct {
	Path   string `json:"path"`
	Merged bool   `json:"done"`
}

type DedupEvent struct {
	Message string
}

const (
	dedupSuffix = "-dedup.db"
)

var (
	ddStatusBucketName = []byte("status")
	ddStatusName       = []byte("1")
	ddItemBucketName   = []byte("item")
)

func (d *dedup) logMsg(message string) {
	d.LogChan <- &DedupEvent{message}
}

func NewDedup(path string, indexName string) (*dedup, error) {
	var err error
	d := &dedup{
		rootPath:  path,
		indexName: indexName,
		LogChan:   make(chan *DedupEvent),
	}
	dedupFile := filepath.Join(path, d.indexName+dedupSuffix)

	d.conn, err = bolt.Open(dedupFile, 0600, getBoltOptions(false))
	if err != nil {
		return nil, err
	}
	if err = d.conn.Update(func(tx *bolt.Tx) error {
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
	}); err != nil {
		return nil, err
	}
	return d, nil
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

func (d *dedup) nextGen(tx *bolt.Tx) (err error) {
	if sb := tx.Bucket(ddStatusBucketName); sb != nil {
		d.status.Gen += 1
		if data, err := json.Marshal(&d.status); err == nil {
			err = sb.Put(ddStatusName, data)
		}
	}
	return
}

func (d *dedup) DetectDupes(minSize int64) (err error) {

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

			if v.Size != nil && *v.Size >= 0 && *v.Size < minSize {
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
				&DedupItem{
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
			bhash := []byte(hash)

			// combine with old status
			prevData := b.Get(bhash)
			if prevData != nil {
				var prevItem ddBag
				err := json.Unmarshal(prevData, &prevItem)
				if err == nil {
					for _, o := range prevItem.ItemList {
						for i, p := range item.ItemList {
							if o.Path == p.Path {
								item.ItemList[i].Merged = o.Merged
							}
						}
					}
				} else {
					// todo
					return err
				}
			}

			item.Gen = d.status.Gen
			if data, err := json.Marshal(item); err != nil {
				return err
			} else {
				if err = b.Put(bhash, data); err != nil {
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

func (d *dedup) Show() ([]*DedupBag, error) {
	var list []*DedupBag
	if err := d.conn.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(ddItemBucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bag ddBag
			if err := json.Unmarshal(v, &bag); err != nil {
				return err
			}
			list = append(list, &DedupBag{
				Hash:     string(k),
				Size:     bag.Size,
				ItemList: bag.ItemList,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	slices.SortFunc(list, func(a, b *DedupBag) int {
		r := b.Size - a.Size
		if r < 0 {
			return -1
		} else if r > 0 {
			return 1
		} else {
			return 0
		}
	})
	return list, nil
}

func (d *dedup) Dedup(hashes []string) error {

	// todo
	if err := os.Chdir(d.rootPath); err != nil {
		return err
	}

	if len(hashes) == 0 {
		if bags, err := d.Show(); err == nil {
			for _, o := range bags {
				hashes = append(hashes, o.Hash)
			}
		} else {
			return err
		}
	}

	if err := d.conn.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(ddItemBucketName)

		for _, hash := range hashes {
			var bag ddBag
			bhash := []byte(hash)
			v := b.Get(bhash)
			if err := json.Unmarshal(v, &bag); err != nil {
				return err
			}
			list := bag.ItemList
			slices.SortFunc(list, func(a, b *DedupItem) int {
				if a.Merged == b.Merged {
					return 0
				}
				if a.Merged {
					return -1
				}
				return 1
			})
			// merged are at the top
			for i := 1; i < len(list); i++ {
				if !list[i].Merged {
					//todo
					// a := filepath.Join(d.rootPath, list[0].Path)
					// b := filepath.Join(d.rootPath, list[i].Path)

					a := list[0].Path
					b := list[i].Path
					d.logMsg(fmt.Sprintf("dedup %s %s", a, b))
					if err := deduplicateFiles(a, b); err == nil {
						list[0].Merged = true
						list[i].Merged = true
					} else {
						d.logMsg(fmt.Sprintf("fail %s", err))
						// log fail
					}
				}
			}

			if data, err := json.Marshal(&bag); err == nil {
				if err := b.Put(bhash, data); err != nil {
					return err
				}
			} else {
				return err
			}

		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
