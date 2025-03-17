package chkbit

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// https://www.kernel.org/doc/html/latest/filesystems/fiemap.html

const (
	sizeOfFiemap       = 32
	sizeOfExtent       = 56
	fs_IOC_FIEMAP      = 0xc020660b
	fiemap_FLAG_SYNC   = 0x0001 // sync the file before mapping
	fiemap_EXTENT_LAST = 0x0001 // last extent in file
)

type fiemap struct {
	start         uint64 // byte offset (inclusive) at which to start mapping (in)
	length        uint64 // logical length of mapping which userspace wants (in)
	flags         uint32 // FIEMAP_FLAG_* flags for request (in/out)
	mappedExtents uint32 // number of extents that were mapped (out)
	extentCount   uint32 // size of fm_extents array (in)
}

type fiemapExtent struct {
	Logical   uint64 // byte offset of the extent in the file
	Physical  uint64 // byte offset of extent on disk
	Length    uint64 // length in bytes for this extent
	reserved1 uint64
	reserved2 uint64
	Flags     uint32 // FIEMAP_EXTENT_* flags for this extent
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

func ioctlFileMap(file *os.File, start uint64, length uint64) ([]fiemapExtent, bool, error) {

	if length == 0 {
		return nil, true, nil
	}

	extentCount := uint32(50)
	buf := make([]byte, sizeOfFiemap+extentCount*sizeOfExtent)
	fm := (*fiemap)(unsafe.Pointer(&buf[0]))
	fm.start = start
	fm.length = length
	fm.flags = fiemap_FLAG_SYNC
	fm.extentCount = extentCount
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), fs_IOC_FIEMAP, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return nil, true, fmt.Errorf("fiemap errno %v", errno)
	}

	extents := make([]fiemapExtent, fm.mappedExtents)
	done := fm.mappedExtents == 0
	lastOffs := start
	for i := range fm.mappedExtents {
		rawinfo := (*fiemapExtent)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(sizeOfFiemap) + uintptr(i*sizeOfExtent)))
		if rawinfo.Logical < lastOffs {
			// return nil, true, errors.New("invalid order")
			return nil, true, fmt.Errorf("invalid order %v", rawinfo.Logical)
		}
		lastOffs = rawinfo.Logical
		extents[i].Logical = rawinfo.Logical
		extents[i].Physical = rawinfo.Physical
		extents[i].Length = rawinfo.Length
		extents[i].Flags = rawinfo.Flags
		done = rawinfo.Flags&fiemap_EXTENT_LAST != 0
	}

	return extents, done, nil
}

func getFileExtentsFp(file *os.File) (FileExtentList, os.FileInfo, error) {

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	var all []fiemapExtent
	start := uint64(0)
	size := uint64(fileInfo.Size())
	for {
		part, done, err := ioctlFileMap(file, start, size-start)
		if err != nil {
			return nil, nil, err
		}

		all = append(all, part...)
		if done {
			return all, fileInfo, nil
		}

		if len(part) == 0 {
			return nil, fileInfo, errors.ErrUnsupported
		}
		last := part[len(part)-1]
		start = last.Logical + last.Length
	}
}

func GetFileExtents(filePath string) (FileExtentList, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fe, _, err := getFileExtentsFp(file)

	if err != nil {
		return nil, fmt.Errorf("failed to get fileextents for %s: %v", filePath, err)
	}
	return fe, err
}

func ExtentsMatch(extList1, extList2 FileExtentList) bool {
	// define that zero blocks can't match
	if len(extList1) == 0 || len(extList1) != len(extList2) {
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
		res += fmt.Sprintf("offs=%x len=%x phys=%x flags=%x\n", b.Logical, b.Length, b.Physical, b.Flags)
	}
	return res
}

// https://www.man7.org/linux/man-pages/man2/ioctl_fideduperange.2.html

func umin(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
}

func DeduplicateFiles(file1, file2 string) (uint64, error) {
	f1, err := os.Open(file1)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", file1, err)
	}
	defer f1.Close()

	// dest must be open for writing
	f2, err := os.OpenFile(file2, os.O_RDWR, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", file2, err)
	}
	defer f2.Close()

	el1, fileInfo1, err := getFileExtentsFp(f1)
	if err != nil {
		return 0, fmt.Errorf("failed to get fileextents for %s: %v", file1, err)
	}

	reclaimed := uint64(0)
	size := uint64(fileInfo1.Size())
	var offs uint64 = 0
	for {
		if offs >= size {
			break
		}

		el2, _, err := getFileExtentsFp(f2)
		if err != nil {
			return reclaimed, fmt.Errorf("failed to get fileextents for %s: %v", file2, err)
		}

		dlen := size - offs
		e1 := el1.find(offs)
		e2 := el2.find(offs)
		if e1 != nil {
			dlen = umin(e1.Length, dlen)
			if e2 != nil {
				if e1.matches(e2) {
					offs += e1.Length
					continue
				} else if e2.Length < e1.Length {
					dlen = umin(e2.Length, dlen)
				}
			}
		}

		dedupe := unix.FileDedupeRange{
			Src_offset: offs,
			Src_length: dlen,
			Info: []unix.FileDedupeRangeInfo{
				unix.FileDedupeRangeInfo{
					Dest_fd:     int64(f2.Fd()),
					Dest_offset: offs,
				},
			}}

		if err = unix.IoctlFileDedupeRange(int(f1.Fd()), &dedupe); err != nil {
			return reclaimed, fmt.Errorf("deduplication failed (offs=%x, len=%x): %s", offs, dlen, err)
		}

		if dedupe.Info[0].Status < 0 {
			errno := unix.Errno(-dedupe.Info[0].Status)
			if errno == unix.EOPNOTSUPP {
				return reclaimed, errNotSupported
			} else if errno == unix.EINVAL {
				return reclaimed, errors.New("deduplication status failed: EINVAL;")
			}
			return reclaimed, fmt.Errorf("deduplication status failed: %s", unix.ErrnoName(errno))
		} else if dedupe.Info[0].Status == unix.FILE_DEDUPE_RANGE_DIFFERS {
			return reclaimed, fmt.Errorf("deduplication unexpected different range (offs=%x, len=%x)", offs, dlen)
		}
		done := dedupe.Info[0].Bytes_deduped
		reclaimed += done
		if offs+done == size {
			break
		} else if offs+done < size {
			// continue
			offs += done
		} else {
			return reclaimed, fmt.Errorf("deduplication unexpected amount of bytes deduped (offs=%x, len=%x)", offs, dlen)
		}
	}

	return reclaimed, nil
}
