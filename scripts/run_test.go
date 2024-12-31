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

// perform integration test using the compiled binary

const testDirBase = "/tmp/chkbit"

func runCmd(args ...string) *exec.Cmd {
	_, filename, _, _ := runtime.Caller(0)
	prjRoot := filepath.Dir(filepath.Dir(filename))
	tool := filepath.Join(prjRoot, "chkbit")
	args = append([]string{"--no-config"}, args...)
	return exec.Command(tool, args...)
}

func checkOut(t *testing.T, sout string, expected string) {
	if !strings.Contains(sout, expected) {
		t.Errorf("Expected '%s' in output, got '%s'\n", expected, sout)
	}
}

func checkNotOut(t *testing.T, sout string, notExpected string) {
	if strings.Contains(sout, notExpected) {
		t.Errorf("Did not expect '%s' in output, got '%s'\n", notExpected, sout)
	}
}

func initStore(t *testing.T, storeType, root string) {
	t.Run("init", func(t *testing.T) {
		cmd := runCmd("init", storeType, root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "chkbit init "+storeType)
		checkNotOut(t, sout, "EXC")
	})
}

func testRoot(t *testing.T, storeType string) {

	testDir := filepath.Join(testDirBase, storeType)
	root := filepath.Join(testDir, "root")
	g := genContext{}
	g.makeTestSampleFiles(testDir)

	checkPrefix := "/tmp/chkbit/split/root/"
	if storeType == "atom" {
		checkPrefix = ""
	}

	initStore(t, storeType, root)

	// update index, no recourse
	t.Run("no-recourse", func(t *testing.T) {
		cmd := runCmd("update", "-mR", filepath.Join(root, "day/office"))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 5 files")
		checkOut(t, sout, "- 1 directory was updated")
		checkOut(t, sout, "- 5 file hashes were added")
		checkOut(t, sout, "- 0 file hashes were updated")
		checkNotOut(t, sout, "removed")
	})

	// update remaining index from root
	t.Run("update-remaining", func(t *testing.T) {
		cmd := runCmd("update", "-m", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 300 files")
		checkOut(t, sout, "- 66 directories were updated")
		checkOut(t, sout, "- 295 file hashes were added")
		checkOut(t, sout, "- 0 file hashes were updated")
		checkNotOut(t, sout, "removed")
	})

	// delete files, check for missing
	t.Run("delete", func(t *testing.T) {
		os.RemoveAll(filepath.Join(root, "thing/change"))
		os.Remove(filepath.Join(root, "time/hour/minute/body-information.csv"))

		cmd := runCmd("check", "-m", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "del "+checkPrefix+"thing/change/")
		checkOut(t, sout, "2 files/directories would have been removed")
	})

	// do not report missing without -m
	t.Run("no-missing", func(t *testing.T) {
		cmd := runCmd("check", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkNotOut(t, sout, "del ")
		checkNotOut(t, sout, "removed")
	})

	// check for missing and update
	t.Run("missing", func(t *testing.T) {
		cmd := runCmd("update", "-m", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "del "+checkPrefix+"thing/change/")
		checkOut(t, sout, "2 files/directories have been removed")
	})

	// check again
	t.Run("repeat", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			cmd := runCmd("update", "-v", root)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("failed with '%s'\n", err)
			}
			sout := string(out)
			checkOut(t, sout, "Processed 289 files")
			checkNotOut(t, sout, "removed")
			checkNotOut(t, sout, "updated")
			checkNotOut(t, sout, "added")
		}
	})

	// add files only
	t.Run("add-only", func(t *testing.T) {

		g.genFiles(filepath.Join(root, "way/add"), 99)
		g.genFile(filepath.Join(root, "time/add-file.txt"), 500)

		cmd := runCmd("update", "-a", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 6 files")
		checkOut(t, sout, "- 3 directories were updated")
		checkOut(t, sout, "- 6 file hashes were added")
		checkOut(t, sout, "- 0 file hashes were updated")
	})

	// add modified files only
	t.Run("add-only-mod", func(t *testing.T) {

		// modify existing
		g.genFile(filepath.Join(root, "way/job/word-business.mp3"), 500)

		cmd := runCmd("update", "-a", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "old "+checkPrefix+"way/job/word-business.mp3")
		checkOut(t, sout, "Processed 1 file")
		checkOut(t, sout, "- 1 directory was updated")
		checkOut(t, sout, "- 0 file hashes were added")
		checkOut(t, sout, "- 1 file hash was updated")
	})

	// update remaining
	t.Run("update-remaining-add", func(t *testing.T) {
		cmd := runCmd("update", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 295 files")
	})

	// ignore dot
	t.Run("ignore-dot", func(t *testing.T) {

		g.genFiles(filepath.Join(root, "way/.hidden"), 99)
		g.genFile(filepath.Join(root, "time/.ignored"), 999)

		cmd := runCmd("update", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 295 files")
	})

	// include dot
	t.Run("include-dot", func(t *testing.T) {

		cmd := runCmd("update", "-d", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed with '%s'\n", err)
		}
		sout := string(out)
		checkOut(t, sout, "Processed 301 files")
		checkOut(t, sout, "- 3 directories were updated")
		checkOut(t, sout, "- 6 file hashes were added")
		checkOut(t, sout, "- 0 file hashes were updated")
	})
}

func testDMG(t *testing.T, storeType string) {

	testDmg := filepath.Join(testDirBase, "test_dmg", storeType)
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

	initStore(t, storeType, ".")

	testFile := filepath.Join(testDmg, "test.txt")
	t1, _ := time.Parse(time.RFC3339, "2022-02-01T11:00:00Z")
	t2, _ := time.Parse(time.RFC3339, "2022-02-01T12:00:00Z")
	t3, _ := time.Parse(time.RFC3339, "2022-02-01T13:00:00Z")

	// create test and set the modified time"
	t.Run("create", func(t *testing.T) {
		os.WriteFile(testFile, []byte("foo1"), 0644)
		os.Chtimes(testFile, t2, t2)

		cmd := runCmd("update", ".")
		if out, err := cmd.Output(); err != nil {
			t.Fatalf("failed with '%s'\n", err)
		} else {
			checkOut(t, string(out), "new test.txt")
		}
	})

	// update test with different content & old modified (expect 'old')"
	t.Run("expect-old", func(t *testing.T) {
		os.WriteFile(testFile, []byte("foo2"), 0644)
		os.Chtimes(testFile, t1, t1)

		cmd := runCmd("update", ".")
		if out, err := cmd.Output(); err != nil {
			t.Fatalf("failed with '%s'\n", err)
		} else {
			checkOut(t, string(out), "old test.txt")
		}
	})

	// update test & new modified (expect 'upd')"
	t.Run("expect-upd", func(t *testing.T) {
		os.WriteFile(testFile, []byte("foo3"), 0644)
		os.Chtimes(testFile, t3, t3)

		cmd := runCmd("update", ".")
		if out, err := cmd.Output(); err != nil {
			t.Fatalf("failed with '%s'\n", err)
		} else {
			checkOut(t, string(out), "upd test.txt")
		}
	})

	// Now update test with the same modified to simulate damage (expect DMG)"
	t.Run("expect-DMG", func(t *testing.T) {
		os.WriteFile(testFile, []byte("foo4"), 0644)
		os.Chtimes(testFile, t3, t3)

		cmd := runCmd("update", ".")
		if out, err := cmd.Output(); err != nil {
			if cmd.ProcessState.ExitCode() != 1 {
				t.Fatalf("expected to fail with exit code 1 vs %d!", cmd.ProcessState.ExitCode())
			}
			checkOut(t, string(out), "DMG test.txt")
		} else {
			t.Fatal("expected to fail!")
		}
	})
}

func TestRootAtom(t *testing.T) {
	testRoot(t, "atom")
}

func TestRootSplit(t *testing.T) {
	testRoot(t, "split")
}

func TestDmgAtom(t *testing.T) {
	testDMG(t, "atom")
}

func TestDmgSplit(t *testing.T) {
	testDMG(t, "split")
}
