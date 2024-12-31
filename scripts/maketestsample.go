package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// perform integration test using the compiled binary

// misc files

var (
	startList = []string{"time", "year", "people", "way", "day", "thing"}
	wordList  = []string{"life", "world", "school", "state", "family", "student", "group", "country", "problem", "hand", "part", "place", "case", "week", "company", "system", "program", "work", "government", "number", "night", "point", "home", "water", "room", "mother", "area", "money", "story", "fact", "month", "lot", "right", "study", "book", "eye", "job", "word", "business", "issue", "side", "kind", "head", "house", "service", "friend", "father", "power", "hour", "game", "line", "end", "member", "law", "car", "city", "community", "name", "president", "team", "minute", "idea", "kid", "body", "information", "back", "face", "others", "level", "office", "door", "health", "person", "art", "war", "history", "party", "result", "change", "morning", "reason", "research", "moment", "air", "teacher", "force", "education"}
	extList   = []string{"txt", "md", "pdf", "jpg", "jpeg", "png", "mp4", "mp3", "csv"}
	startDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate   = time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	dateList  = []time.Time{}
)

type genContext struct {
	wordIdx int
	extIdx  int
	dateIdx int
}

func init() {
	var c int64 = 50
	interval := (int64)(endDate.Sub(startDate).Seconds()) / c
	for i := range make([]int64, c) {
		dateList = append(dateList, startDate.Add(time.Duration(interval*(int64)(i))*time.Second))
	}
}

func (g *genContext) nextWord() string {
	word := wordList[g.wordIdx%len(wordList)]
	g.wordIdx++
	return word
}

func (g *genContext) nextExt() string {
	ext := extList[g.extIdx%len(extList)]
	g.extIdx++
	return ext
}

func (g *genContext) setDate(filename string, r int) {
	date := dateList[g.dateIdx%len(dateList)]
	m := 17 * g.dateIdx / len(dateList)
	date = date.Add(time.Duration(m) * time.Hour)
	g.dateIdx++
	os.Chtimes(filename, date, date)
}

func (g *genContext) genFile(path string, size int) {
	os.WriteFile(path, make([]byte, size), 0644)
	g.setDate(path, size*size)
}

func (g *genContext) genFiles(dir string, a int) {
	os.MkdirAll(dir, 0755)
	for i := 1; i <= 5; i++ {
		size := a*i*g.wordIdx*100 + g.extIdx
		file := g.nextWord() + "-" + g.nextWord()

		if i%3 == 0 {
			file += "-" + g.nextWord()
		}

		file += "." + g.nextExt()
		g.genFile(filepath.Join(dir, file), size)
	}
}

func (g *genContext) genDir(root string) {
	for _, start := range startList {

		for i := 1; i <= 5; i++ {
			dir := filepath.Join(root, start, g.nextWord())
			g.genFiles(dir, 1)

			if g.wordIdx%3 == 0 {
				dir = filepath.Join(dir, g.nextWord())
				g.genFiles(dir, 1)
			}
		}
	}
}

func (g *genContext) makeTestSampleFiles(testDir string) {

	if err := os.RemoveAll(testDir); err != nil {
		fmt.Println("Failed to clean", err)
		panic(err)
	}

	root := filepath.Join(testDir, "root")
	g.genDir(root)

	os.MkdirAll(filepath.Join(root, "day/car/empty"), 0755)

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

func main() {
	root := flag.String("root", "", "root path to sample data (will be cleared)")
	flag.Parse()
	if *root == "" {
		fmt.Println("error: root parameter is required")
		os.Exit(1)
	}
	fmt.Printf("Clearing and generating test data in %s\n", *root)
	g := genContext{}
	g.makeTestSampleFiles(*root)
}
