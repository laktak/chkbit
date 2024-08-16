package util

import (
	"testing"
)

func TestTrunc(t *testing.T) {
	expected := "ab©def"
	actual := LeftTruncate(expected+"ghijk", 6)
	if expected != actual {
		t.Error("expected:", expected, "actual:", actual)
	}
}
