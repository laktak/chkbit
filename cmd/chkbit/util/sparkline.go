package util

import (
	"math"
)

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func Sparkline(series []int64) string {
	out := make([]rune, len(series))
	min := Minimum(series)
	max := Maximum(series)
	dataRange := max - min
	if dataRange == 0 {
		for i := range series {
			out[i] = sparkChars[0]
		}
	} else {
		step := float64(len(sparkChars)-1) / float64(dataRange)
		for i, n := range series {
			idx := int(math.Round(float64(Clamp(min, max, n)-min) * step))
			if idx < 0 {
				out[i] = ' '
			} else if idx > len(sparkChars) {
				out[i] = sparkChars[len(sparkChars)-1]
			} else {
				out[i] = sparkChars[idx]
			}
		}
	}
	return string(out)
}
