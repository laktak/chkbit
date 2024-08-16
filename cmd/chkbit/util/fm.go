package util

import (
	"math"
)

func Minimum(series []int64) int64 {
	var min int64 = math.MaxInt64
	for _, value := range series {
		if value < min {
			min = value
		}
	}
	return min
}

func Maximum(series []int64) int64 {
	var max int64 = math.MinInt64
	for _, value := range series {
		if value > max {
			max = value
		}
	}
	return max
}

func Clamp(min int64, max int64, n int64) int64 {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
