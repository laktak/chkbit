package chkbit

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// https://www.man7.org/linux/man-pages/man2/ioctl_fideduperange.2.html

func deduplicateFiles(file1, file2 string) error {
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

	// fileInfo2, err := f2.Stat()
	// fmt.Println("dedup sz", size, fileInfo2.Size())

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
		return fmt.Errorf("FileDedupeRange failed: %s", err)
	}

	if dedupe.Info[0].Status < 0 {
		errno := unix.Errno(-dedupe.Info[0].Status)
		if errno == unix.EOPNOTSUPP {
			return errors.New("deduplication not supported on this filesystem")
		} else if errno == unix.EINVAL {
			return errors.New("FileDedupeRange Status failed: EINVAL;")
		}
		return fmt.Errorf("FileDedupeRange Status failed: %s", unix.ErrnoName(errno))
	} else if dedupe.Info[0].Status == unix.FILE_DEDUPE_RANGE_DIFFERS {
		return fmt.Errorf("Unexpected different range")
	}
	if dedupe.Info[0].Bytes_deduped != uint64(size) {
		return fmt.Errorf("Unexpected amount of bytes deduped %v != %v",
			dedupe.Info[0].Bytes_deduped, size)
	}

	return nil
}
