//go:build !linux

package chkbit

import (
	"errors"
)

func DeduplicateFiles(_, _ string) error {
	return errors.New("deduplicate is not supported on this OS")
}

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
