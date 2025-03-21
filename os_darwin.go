package chkbit

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// https://www.unix.com/man_page/osx/2/fcntl/

type fiemapExtent struct {
	Logical  uint64 // byte offset of the extent in the file
	Physical uint64 // byte offset of extent on disk
	Length   uint64 // length in bytes for this extent
}

type FileExtentList []fiemapExtent

func (fe *fiemapExtent) matches(o *fiemapExtent) bool {
	return fe.Logical == o.Logical && fe.Physical == o.Physical && fe.Length == o.Length
}

func (fe FileExtentList) find(offs uint64) *fiemapExtent {
	for _, o := range fe {
		if o.Logical == offs {
			return &o
		}
	}
	return nil
}

func getFileExtentsFp(file *os.File) (FileExtentList, os.FileInfo, error) {

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	var all []fiemapExtent
	start := uint64(0)
	size := uint64(fileInfo.Size())
	maxReq := uint64(100 * 1024 * 1024)

	// don't use syscall.Log2phys_t as it's alignment is incorrect

	type Log2phys_t2 struct {
		// IN: number of bytes to be queried; OUT: number of contiguous bytes allocated at this position
		Contigbytes uint64
		// IN: bytes into file; OUT: bytes into device
		Devoffset uint64
	}

	buf := make([]byte, 8*3)
	for {

		// skip flags
		lp := (*Log2phys_t2)(unsafe.Pointer(&buf[4]))
		lp.Contigbytes = maxReq
		lp.Devoffset = start

		rc, err := unix.FcntlInt(file.Fd(), syscall.F_LOG2PHYS_EXT, int(uintptr(unsafe.Pointer(&buf[0]))))
		if err != nil {
			return nil, nil, err
		}
		if rc < 0 {
			return nil, nil, errors.New("log2phys failed")
		}

		all = append(all,
			fiemapExtent{
				Logical:  start,
				Physical: lp.Devoffset,
				Length:   lp.Contigbytes,
			})

		start += lp.Contigbytes
		if start >= size {
			return all, fileInfo, nil
		}
	}
}

func GetFileExtents(filePath string) (FileExtentList, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	file.Sync()
	defer file.Close()
	fe, _, err := getFileExtentsFp(file)
	if err != nil {
		return nil, fmt.Errorf("failed to get fileextents for %s: %v", filePath, err)
	}
	return fe, err
}

func ExtentsMatch(extList1, extList2 FileExtentList) bool {
	if len(extList1) != len(extList2) {
		return false
	}
	for i := range extList1 {
		a := extList1[i]
		b := extList2[i]
		if !a.matches(&b) {
			return false
		}
	}

	return true
}

func ShowExtents(extList FileExtentList) string {
	res := ""
	for _, b := range extList {
		res += fmt.Sprintf("offs=%x len=%x phys=%x\n", b.Logical, b.Length, b.Physical)
	}
	return res
}

func DeduplicateFiles(file1, file2 string) (uint64, error) {
	return 0, errors.New("deduplicate is not supported on this OS")
}
