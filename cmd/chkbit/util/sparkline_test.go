package util

import (
	"testing"
)

func TestSpark(t *testing.T) {
	expected := "▁▁▂▄▅▇██▆▄▂"
	actual := Sparkline([]int64{5, 12, 35, 73, 80, 125, 150, 142, 118, 61, 19})
	if expected != actual {
		t.Error("expected:", expected, "actual:", actual)
	}
}
