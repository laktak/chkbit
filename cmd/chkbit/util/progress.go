package util

import (
	"strings"
)

func Progress(percent float64, width int) (string, string) {
	if width >= 5 {
		pc := int(percent * float64(width))
		return strings.Repeat("#", pc), strings.Repeat(":", width-pc)
	}
	return "", ""
}
