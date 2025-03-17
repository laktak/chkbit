//go:build !linux

package chkbit

type FileExtentList []int

func GetFileExtents(_ string) (FileExtentList, error) {
	return nil, errNotSupported
}

func ExtentsMatch(_, _ FileExtentList) bool {
	return false
}

func ShowExtents(_ FileExtentList) string {
	return ""
}

func DeduplicateFiles(_, _ string) (uint64, error) {
	return 0, errNotSupported
}
