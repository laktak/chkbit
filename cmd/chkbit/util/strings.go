package util

func LeftTruncate(s string, nMax int) string {
	for i := range s {
		nMax--
		if nMax < 0 {
			return s[:i]
		}
	}
	return s
}
