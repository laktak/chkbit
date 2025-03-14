//go:build !linux

package chkbit

import (
	"errors"
)

type FileExtentList []int

func GetFileExtents(_ string) (FileExtentList, error) {
	return nil, errors.New("fileblocks is not supported on this OS")
}

func ExtentsMatch(_, _ FileExtentList) bool {
	return false
}

func ShowExtents(blocks FileExtentList) string {
	return ""
}

func DeduplicateFiles(_, _ string) (uint64, error) {
	return 0, errors.New("deduplicate is not supported on this OS")
}
