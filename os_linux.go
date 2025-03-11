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

func ioctlFileMap(file *os.File, start uint64, length uint64) ([]fiemapExtent, bool, error) {

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
	for i := range fm.mappedExtents {
		rawinfo := (*fiemapExtent)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(sizeOfFiemap) + uintptr(i*sizeOfExtent)))
		extents[i].Logical = rawinfo.Logical
		extents[i].Physical = rawinfo.Physical
		extents[i].Length = rawinfo.Length
		extents[i].Flags = rawinfo.Flags
		done = rawinfo.Flags&fiemap_EXTENT_LAST != 0
	}

	return extents, done, nil
}

func GetFileExtents(filePath string) (FileExtentList, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	var all []fiemapExtent
	start := uint64(0)
	size := uint64(fileInfo.Size())
	for {
		part, done, err := ioctlFileMap(file, start, size-start)
		if err != nil {
			return nil, err
		}

		all = append(all, part...)
		if done {
			return all, nil
		}

		if len(part) == 0 {
			return nil, errors.ErrUnsupported
		}
		last := part[len(part)-1]
		start = last.Logical + last.Length
	}
}

func ExtentsMatch(blocks1, blocks2 FileExtentList) bool {
	// define that zero blocks can't match
	if len(blocks1) == 0 || len(blocks1) != len(blocks2) {
		return false
	}
	for i := range blocks1 {
		a := blocks1[i]
		b := blocks2[i]
		if a.Physical != b.Physical || a.Length != b.Length {
			return false
		}
	}

	return true
}

// https://www.man7.org/linux/man-pages/man2/ioctl_fideduperange.2.html

func DeduplicateFiles(file1, file2 string) error {
	f1, err := os.Open(file1)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", file1, err)
	}
	defer f1.Close()

	// dest must be open for writing
	f2, err := os.OpenFile(file2, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", file2, err)
	}
	defer f2.Close()

	fileInfo1, err := f1.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %v", file1, err)
	}
	size := fileInfo1.Size()

	dedupe := unix.FileDedupeRange{
		Src_offset: uint64(0),
		Src_length: uint64(size),
		Info: []unix.FileDedupeRangeInfo{
			unix.FileDedupeRangeInfo{
				Dest_fd:     int64(f2.Fd()),
				Dest_offset: uint64(0),
			},
		}}

	if err = unix.IoctlFileDedupeRange(int(f1.Fd()), &dedupe); err != nil {
		return fmt.Errorf("deduplication failed: %s", err)
	}

	if dedupe.Info[0].Status < 0 {
		errno := unix.Errno(-dedupe.Info[0].Status)
		if errno == unix.EOPNOTSUPP {
			return errors.New("deduplication not supported on this filesystem")
		} else if errno == unix.EINVAL {
			return errors.New("deduplication status failed: EINVAL;")
		}
		return fmt.Errorf("deduplication status failed: %s", unix.ErrnoName(errno))
	} else if dedupe.Info[0].Status == unix.FILE_DEDUPE_RANGE_DIFFERS {
		return fmt.Errorf("deduplication unexpected different range")
	}
	if dedupe.Info[0].Bytes_deduped != uint64(size) {
		return fmt.Errorf("deduplication unexpected amount of bytes deduped %v != %v",
			dedupe.Info[0].Bytes_deduped, size)
	}

	return nil
}
