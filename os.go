//go:build !linux

package chkbit

import (
	"errors"
)

func deduplicateFiles(file1, file2 string) error {
	return errors.New("deduplicate not supported on this OS")
}
