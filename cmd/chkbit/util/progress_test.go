package util

import (
	"testing"
)

func TestProgress(t *testing.T) {
	expected := "###########:::::::::::::::::::"
	a, b := Progress(.375, 30)
	actual := a + b
	if expected != actual {
		t.Error("expected:", expected, "actual:", actual)
	}
}
