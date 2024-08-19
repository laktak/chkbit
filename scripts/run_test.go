package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var testDir = "/tmp/chkbit"

func getCmd() string {
	_, filename, _, _ := runtime.Caller(0)
	prjRoot := filepath.Dir(filepath.Dir(filename))
	return filepath.Join(prjRoot, "chkbit")
}

func checkOut(t *testing.T, sout string, expected string) {
	if !strings.Contains(sout, expected) {
		t.Errorf("Expected '%s' in output, got '%s'\n", expected, sout)
	}
}

// misc files

var (
	startList = []string{"time", "year", "people", "way", "day", "thing"}
	wordList  = []string{"life", "world", "school", "state", "family", "student", "group", "country", "problem", "hand", "part", "place", "case", "week", "company", "system", "program", "work", "government", "number", "night", "point", "home", "water", "room", "mother", "area", "money", "story", "fact", "month", "lot", "right", "study", "book", "eye", "job", "word", "business", "issue", "side", "kind", "head", "house", "service", "friend", "father", "power", "hour", "game", "line", "end", "member", "law", "car", "city", "community", "name", "president", "team", "minute", "idea", "kid", "body", "information", "back", "face", "others", "level", "office", "door", "health", "person", "art", "war", "history", "party", "result", "change", "morning", "reason", "research", "moment", "air", "teacher", "force", "education"}
	extList   = []string{"txt", "md", "pdf", "jpg", "jpeg", "png", "mp4", "mp3", "csv"}
	startDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate   = time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	dateList  = []time.Time{}
	wordIdx   = 0
	extIdx    = 0
	dateIdx   = 0
)

func nextWord() string {
	word := wordList[wordIdx%len(wordList)]
	wordIdx++
	return word
}

func nextExt() string {
	ext := extList[extIdx%len(extList)]
	extIdx++
	return ext
}

func setDate(filename string, r int) {
	date := dateList[dateIdx%len(dateList)]
	m := 17 * dateIdx / len(dateList)
	date = date.Add(time.Duration(m) * time.Hour)
	dateIdx++
	os.Chtimes(filename, date, date)
}

func genFile(dir string, a int) {
	os.MkdirAll(dir, 0755)
	for i := 1; i <= 5; i++ {
		size := a*i*wordIdx*100 + extIdx
		file := nextWord() + "-" + nextWord()

		if i%3 == 0 {
			file += "-" + nextWord()
		}

		file += "." + nextExt()
		path := filepath.Join(dir, file)
		os.WriteFile(path, make([]byte, size), 0644)
		setDate(path, size*size)
	}
}

func genDir(root string) {
	for _, start := range startList {

		for i := 1; i <= 5; i++ {
			dir := filepath.Join(root, start, nextWord())
			genFile(dir, 1)

			if wordIdx%3 == 0 {
				dir = filepath.Join(dir, nextWord())
				genFile(dir, 1)
			}
		}
	}
}

func setupMiscFiles() {

	var c int64 = 50
	interval := (int64)(endDate.Sub(startDate).Seconds()) / c
	for i := range make([]int64, c) {
		dateList = append(dateList, startDate.Add(time.Duration(interval*(int64)(i))*time.Second))
	}

	root := filepath.Join(testDir, "root")
	if err := os.RemoveAll(testDir); err != nil {
		fmt.Println("Failed to clean", err)
		panic(err)
	}

	genDir(root)

	rootPeople := filepath.Join(root, "people")
	testPeople := filepath.Join(testDir, "people")

	err := os.Rename(rootPeople, testPeople)
	if err != nil {
		fmt.Println("Rename failed", err)
		panic(err)
	}

	err = os.Symlink(testPeople, rootPeople)
	if err != nil {
		fmt.Println("Symlink failed", err)
		panic(err)
	}
}

func TestRoot(t *testing.T) {
	setupMiscFiles()

	tool := getCmd()
	root := filepath.Join(testDir, "root")
	cmd := exec.Command(tool, "-u", root)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cmd.Output() failed with '%s'\n", err)
	}
	sout := string(out)
	checkOut(t, sout, "60 directories were updated")
	checkOut(t, sout, "300 file hashes were added")
}

func TestDMG(t *testing.T) {

	testDmg := filepath.Join(testDir, "test_dmg")
	if err := os.RemoveAll(testDmg); err != nil {
		fmt.Println("Failed to clean", err)
		panic(err)
	}
	if err := os.MkdirAll(testDmg, 0755); err != nil {
		fmt.Println("Failed to create test directory", err)
		panic(err)
	}

	if err := os.Chdir(testDmg); err != nil {
		fmt.Println("Failed to cd test directory", err)
		panic(err)
	}

	tool := getCmd()
	testFile := filepath.Join(testDmg, "test.txt")
	t1, _ := time.Parse(time.RFC3339, "2022-02-01T11:00:00Z")
	t2, _ := time.Parse(time.RFC3339, "2022-02-01T12:00:00Z")
	t3, _ := time.Parse(time.RFC3339, "2022-02-01T13:00:00Z")

	// step1: create test and set the modified time"
	os.WriteFile(testFile, []byte("foo1"), 0644)
	os.Chtimes(testFile, t2, t2)

	cmd := exec.Command(tool, "-u", ".")
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("step1 failed with '%s'\n", err)
	} else {
		checkOut(t, string(out), "new test.txt")
	}

	// step2: update test with different content & old modified (expect 'old')"
	os.WriteFile(testFile, []byte("foo2"), 0644)
	os.Chtimes(testFile, t1, t1)

	cmd = exec.Command(tool, "-u", ".")
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("step2 failed with '%s'\n", err)
	} else {
		checkOut(t, string(out), "old test.txt")
	}

	// step3: update test & new modified (expect 'upd')"
	os.WriteFile(testFile, []byte("foo3"), 0644)
	os.Chtimes(testFile, t3, t3)

	cmd = exec.Command(tool, "-u", ".")
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("step3 failed with '%s'\n", err)
	} else {
		checkOut(t, string(out), "upd test.txt")
	}

	// step4: Now update test with the same modified to simulate damage (expect DMG)"
	os.WriteFile(testFile, []byte("foo4"), 0644)
	os.Chtimes(testFile, t3, t3)

	cmd = exec.Command(tool, "-u", ".")
	if out, err := cmd.Output(); err != nil {
		if cmd.ProcessState.ExitCode() != 1 {
			t.Fatalf("step4 expected to fail with exit code 1 vs %d!", cmd.ProcessState.ExitCode())
		}
		checkOut(t, string(out), "DMG test.txt")
	} else {
		t.Fatal("step4 expected to fail!")
	}
}
