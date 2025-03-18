package chkbit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

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

	doAbort        bool
	numTotal       int
	reclaimedTotal uint64
}

type ddStatus struct {
	Gen     int       `json:"gen"`
	Updated time.Time `json:"mod"`
}

type ddBag struct {
	Gen           int          `json:"gen"`
	Size          int64        `json:"size"`
	SizeShared    uint64       `json:"shared"`
	SizeExclusive uint64       `json:"exclusive"`
	ExtUnknown    *bool        `json:"extUnknown,omitempty"`
	ItemList      []*DedupItem `json:"item"`
}

type DedupBag struct {
	Hash          string       `json:"hash"`
	Size          uint64       `json:"size"`
	SizeShared    uint64       `json:"shared"`
	SizeExclusive uint64       `json:"exclusive"`
	ExtUnknown    *bool        `json:"extUnknown,omitempty"`
	ItemList      []*DedupItem `json:"item"`
}

type DedupItem struct {
	Path   string `json:"path"`
	Merged bool   `json:"merged"`
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

func IsAborted(err error) bool {
	return err == errAborted
}

func (d *Dedup) Abort() {
	d.doAbort = true
}
func (d *Dedup) DidAbort() bool {
	return d.doAbort
}

func (d *Dedup) NumTotal() int {
	return d.numTotal
}

func (d *Dedup) ReclaimedTotal() uint64 {
	return d.reclaimedTotal
}

func (d *Dedup) LastUpdated() time.Time {
	return d.status.Updated
}

func (d *Dedup) log(stat Status, message string) {
	d.LogQueue <- &LogEvent{stat, message}
}

func (d *Dedup) logMsg(message string) {
	d.log(StatusInfo, message)
}

func (d *Dedup) perfMonFiles(numFiles int, i float64, l int) {
	d.numTotal += numFiles
	pc := 0.0
	if l > 0 {
		pc = i / float64(l)
	}
	d.PerfQueue <- &DedupPerfEvent{int64(numFiles), pc}
}

func NewDedup(path string, indexName string, createIfNotExists bool) (*Dedup, error) {
	var err error
	d := &Dedup{
		rootPath:  path,
		indexName: indexName,
		LogQueue:  make(chan *LogEvent, 100),
		PerfQueue: make(chan *DedupPerfEvent, 100),
	}
	dedupFile := filepath.Join(path, d.indexName+dedupSuffix)

	_, err = os.Stat(dedupFile)
	if err != nil {
		if !os.IsNotExist(err) || !createIfNotExists {
			return nil, err
		}
	}

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

func (d *Dedup) nextGen(tx *bolt.Tx) error {
	if sb := tx.Bucket(ddStatusBucketName); sb != nil {
		d.status.Gen += 1
		d.status.Updated = time.Now()
		if data, err := json.Marshal(&d.status); err == nil {
			return sb.Put(ddStatusName, data)
		} else {
			return err
		}
	}
	return errors.New("missing bucket")
}

func (d *Dedup) DetectDupes(minSize uint64, verbose bool) (err error) {

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

	d.logMsg(fmt.Sprintf("collect matching hashes (min=%s)", intutil.FormatSize(minSize)))
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

			if v.Size != nil && *v.Size >= 0 && uint64(*v.Size) < minSize {
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
		if bag.Size < int64(minSize) {
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
			// todo, ignore for now
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
					} // else ignore
				}
			*/

			type match struct {
				id   int
				el   FileExtentList
				item *DedupItem
			}

			extUnknown := false
			var matches []match
			d.perfMonFiles(len(bag.ItemList), float64(i), len(all))
			for _, item := range bag.ItemList {
				if res, err := GetFileExtents(filepath.Join(d.rootPath, item.Path)); err == nil {
					matches = append(matches, match{-1, res, item})
				} else if IsNotSupported(err) {
					matches = append(matches, match{-1, nil, item})
					extUnknown = true
				} else {
					// file is ignored
					if !os.IsNotExist(err) {
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
			if extUnknown {
				bag.ExtUnknown = &extUnknown
			}
			bag.SizeShared = 0
			bag.SizeExclusive = 0
			bag.ItemList = []*DedupItem{}
			for i := range matches {
				merged := matches[i].id == maxId

				matches[i].item.Merged = merged
				bag.ItemList = append(bag.ItemList, matches[i].item)
				if merged {
					bag.SizeShared += uint64(bag.Size)
				}
				if matches[i].id == i {
					bag.SizeExclusive += uint64(bag.Size)
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
				Size:          uint64(bag.Size),
				SizeShared:    bag.SizeShared,
				SizeExclusive: bag.SizeExclusive,
				ExtUnknown:    bag.ExtUnknown,
				ItemList:      bag.ItemList,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	slices.SortFunc(list, func(a, b *DedupBag) int {
		r := int64(b.Size) - int64(a.Size)
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

func (d *Dedup) Dedup(hashes []string, verbose bool) error {

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
		done := 0
		d.perfMonFiles(0, 0, len(hashes))

		for _, hash := range hashes {

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
			todoCount := 0.0
			for i := 1; i < len(list); i++ {
				if !list[i].Merged {
					todoCount += 1
				}
			}
			listDone := 0.0

			// merged are at the top
			for i := 1; i < len(list); i++ {
				if d.doAbort {
					return errAborted
				}

				if !list[i].Merged {
					a := filepath.Join(d.rootPath, list[0].Path)
					b := filepath.Join(d.rootPath, list[i].Path)
					if verbose {
						d.logMsg(fmt.Sprintf("dedup %s %s \"%s\" -- \"%s\"", hash, intutil.FormatSize(uint64(bag.Size)), a, b))
					} else {
						d.logMsg(fmt.Sprintf("dedup %s %s", intutil.FormatSize(uint64(bag.Size)), a))
					}
					if reclaimed, err := DeduplicateFiles(a, b); err == nil {
						if !list[0].Merged {
							bag.SizeShared += uint64(bag.Size)
						}
						list[0].Merged = true
						list[i].Merged = true
						bag.SizeShared += uint64(bag.Size)
						bag.SizeExclusive -= uint64(bag.Size)
						d.reclaimedTotal += reclaimed
					} else if IsNotSupported(err) {
						d.log(StatusPanic, "Dedupliate is not supported for this OS/fs, please see https://laktak.github.io/chkbit/dedup/")
						return err
					} else {
						d.log(StatusPanic, err.Error())
					}
					listDone += 1
					d.perfMonFiles(1, float64(done)+listDone/todoCount, len(hashes))
				}
			}
			done += 1

			if data, err := json.Marshal(&bag); err == nil {
				if err := b.Put(bhash, data); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		d.perfMonFiles(0, float64(done), len(hashes))
		return nil
	}); err != nil {
		return err
	}
	return nil
}
