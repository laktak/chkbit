package chkbit

import (
	"testing"
)

func TestShouldIgnore(t *testing.T) {
	context := &Context{
		IncludeDot: true,
	}

	ignore1 := &Ignore{
		parentIgnore: nil,
		context:      context,
		path:         "na",
		name:         "vienna/",
		itemList:     []string{"*.txt", "/photo.jpg", "tokyo", "/sydney", "berlin/oslo"},
	}

	ignore2 := &Ignore{
		parentIgnore: ignore1,
		context:      context,
		path:         "na",
		name:         "berlin/",
		itemList:     []string{"/*.md"},
	}

	ignore3 := &Ignore{
		parentIgnore: ignore2,
		context:      context,
		path:         "na",
		name:         "sydney/",
		itemList:     []string{},
	}

	tests := []struct {
		ignore   *Ignore
		filename string
		expected bool
	}{
		// test root
		{ignore1, "all.txt", true},
		{ignore1, "readme.md", false},
		{ignore1, "photo.jpg", true},
		{ignore1, "berlin", false},
		// test directories
		{ignore1, "tokyo", true},
		{ignore1, "sydney", true},
		// test in berlin
		{ignore2, "all.txt", true},
		{ignore2, "readme.md", true},
		{ignore2, "photo.jpg", false},
		// test directories
		{ignore2, "tokyo", true},
		{ignore2, "sydney", false},
		{ignore2, "oslo", true},
		// test in sydney
		{ignore3, "all.txt", true},
		{ignore3, "readme.md", false},
		{ignore3, "photo.jpg", false},
	}

	for _, tt := range tests {
		t.Run("test "+tt.filename+" in "+tt.ignore.name, func(t *testing.T) {
			if tt.ignore.shouldIgnore(tt.filename) != tt.expected {
				t.Errorf("shouldIgnore(%s) = %v, want %v", tt.filename, !tt.expected, tt.expected)
			}
		})
	}
}
