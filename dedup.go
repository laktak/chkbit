package chkbit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/laktak/chkbit/v6/intutil"
	bolt "go.etcd.io/bbolt"
)

type Dedup struct {
	rootPath  string
	indexName string

	LogQueue  chan *LogEvent
	PerfQueue chan *DedupPerfEvent

	status ddStatus
	conn   *bolt.DB

	doAbort  bool
	NumTotal int
}

type ddStatus struct {
	Gen int `json:"gen"`
}

type ddBag struct {
	Gen           int          `json:"gen"`
	Size          int64        `json:"size"`
	SizeShared    int64        `json:"shared"`
	SizeExclusive int64        `json:"exclusive"`
	ItemList      []*DedupItem `json:"item"`
}

type DedupBag struct {
	Hash          string       `json:"hash"`
	Size          int64        `json:"size"`
	SizeShared    int64        `json:"shared"`
	SizeExclusive int64        `json:"exclusive"`
	ItemList      []*DedupItem `json:"item"`
}

type DedupItem struct {
	Path   string `json:"path"`
	Merged bool   `json:"done"`
}

const (
	dedupSuffix = "-dedup.db"
)

var (
	ddStatusBucketName = []byte("status")
	ddStatusName       = []byte("1")
	ddItemBucketName   = []byte("item")

	errAborted = errors.New("aborted")
)

func (d *Dedup) Abort() {
	d.doAbort = true
}
func (d *Dedup) DidAbort() bool {
	return d.doAbort
}

func (d *Dedup) log(stat Status, message string) {
	d.LogQueue <- &LogEvent{stat, message}
}

func (d *Dedup) logMsg(message string) {
	d.log(StatusInfo, message)
}

func (d *Dedup) perfMonFiles(numFiles, i, l int) {
	d.NumTotal += numFiles
	pc := 0.0
	if l > 0 {
		pc = float64(i) / float64(l)
	}
	d.PerfQueue <- &DedupPerfEvent{int64(numFiles), pc}
}

func NewDedup(path string, indexName string) (*Dedup, error) {
	var err error
	d := &Dedup{
		rootPath:  path,
		indexName: indexName,
		LogQueue:  make(chan *LogEvent),
		PerfQueue: make(chan *DedupPerfEvent, 100),
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

func (d *Dedup) Finish() error {
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			return err
		}
	}
	d.conn = nil
	return nil
}

func (d *Dedup) nextGen(tx *bolt.Tx) (err error) {
	if sb := tx.Bucket(ddStatusBucketName); sb != nil {
		d.status.Gen += 1
		if data, err := json.Marshal(&d.status); err == nil {
			err = sb.Put(ddStatusName, data)
		}
	}
	return
}

func (d *Dedup) DetectDupes(minSize int64, verbose bool) (err error) {

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

	d.logMsg("collect matching hashes")
	all := make(map[string]*ddBag)
	for {
		if d.doAbort {
			return errAborted
		}

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

	// legacy index items don't contain a file size
	d.logMsg("update file sizes (for legacy indexes)")
	for hash, bag := range all {
		if bag.Size == -1 {
			for _, p := range bag.ItemList {
				if s, err := os.Stat(filepath.Join(d.rootPath, p.Path)); err == nil {
					bag.Size = s.Size()
					break
				}
			}
		}
		if bag.Size < minSize {
			delete(all, hash)
		}
	}

	markDelete := [][]byte{}

	// now check resultset for exclusive/shared space
	d.logMsg("collect matching files")
	if err = d.conn.Update(func(tx *bolt.Tx) error {

		if err := d.nextGen(tx); err != nil {
			return err
		}

		b := tx.Bucket(ddItemBucketName)
		i := 0
		d.perfMonFiles(0, 0, len(all))
		for hash, bag := range all {
			i += 1

			if d.doAbort {
				return errAborted
			}

			if len(bag.ItemList) <= 1 {
				continue
			}
			bhash := []byte(hash)

			// combine with old status
			/*
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
			*/

			type match struct {
				id   int
				el   FileExtentList
				item *DedupItem
			}

			var matches []match
			d.perfMonFiles(len(bag.ItemList), i, len(all))
			for _, item := range bag.ItemList {
				if res, err := GetFileExtents(filepath.Join(d.rootPath, item.Path)); err == nil {
					matches = append(matches, match{-1, res, item})
				} else {
					if !os.IsNotExist(err) {
						// todo err
						d.log(StatusPanic, err.Error())
					}
				}
			}

			// compare extents and set id for matching
			for i := range matches {
				if matches[i].id != -1 {
					continue
				}
				matches[i].id = i
				for j := i + 1; j < len(matches); j++ {
					if matches[j].id == -1 && ExtentsMatch(matches[i].el, matches[j].el) {
						matches[j].id = i
					}
				}
			}

			// count matches and get maxId
			maxId := -1
			maxCount := 1
			count := map[int]int{}
			for _, o := range matches {
				count[o.id] += 1
			}
			for id, c := range count {
				if c > maxCount {
					maxId = id
					maxCount = c
				}
			}

			bag.SizeShared = 0
			bag.SizeExclusive = 0
			bag.ItemList = []*DedupItem{}
			for i := range matches {
				merged := matches[i].id == maxId

				matches[i].item.Merged = merged
				bag.ItemList = append(bag.ItemList, matches[i].item)
				if merged {
					bag.SizeShared += bag.Size
				} else if matches[i].id == i {
					bag.SizeExclusive += bag.Size
				}
			}

			if len(bag.ItemList) < 2 {
				// remove because of missing files
				markDelete = append(markDelete, bhash)
				continue
			}

			bag.Gen = d.status.Gen
			if data, err := json.Marshal(bag); err != nil {
				return err
			} else {
				if err = b.Put(bhash, data); err != nil {
					return err
				}
			}
		}

		// remove old gen (don't use c.Delete())
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bag ddBag
			if err := json.Unmarshal(v, &bag); err != nil {
				return err
			}
			if bag.Gen != d.status.Gen {
				markDelete = append(markDelete, k)
			}
		}
		for _, k := range markDelete {
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

func (d *Dedup) Show() ([]*DedupBag, error) {
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
				Hash:          string(k),
				Size:          bag.Size,
				SizeShared:    bag.SizeShared,
				SizeExclusive: bag.SizeExclusive,
				ItemList:      bag.ItemList,
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

func (d *Dedup) Dedup(hashes []string) error {

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
		c := 0
		d.perfMonFiles(0, 0, len(hashes))

		for _, hash := range hashes {
			c += 1

			if d.doAbort {
				return errAborted
			}

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
				if d.doAbort {
					return errAborted
				}

				if !list[i].Merged {
					a := filepath.Join(d.rootPath, list[0].Path)
					b := filepath.Join(d.rootPath, list[i].Path)
					d.logMsg(fmt.Sprintf("dedup %s %s \"%s\" -- \"%s\"", hash, intutil.FormatSize(bag.Size), a, b))
					if err := DeduplicateFiles(a, b); err == nil {
						list[0].Merged = true
						list[i].Merged = true
					} else {
						d.log(StatusPanic, err.Error())
					}
				}
			}
			d.perfMonFiles(1, c, len(hashes))

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
