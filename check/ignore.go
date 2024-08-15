package check

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Ignore struct {
	parentIgnore *Ignore
	context      *Context
	path         string
	name         string
	itemList     []string
}

func GetIgnore(context *Context, path string, parentIgnore *Ignore) (*Ignore, error) {
	ignore := &Ignore{
		parentIgnore: parentIgnore,
		context:      context,
		path:         path,
		name:         filepath.Base(path) + "/",
	}
	err := ignore.loadIgnore()
	if err != nil {
		return nil, err
	}
	return ignore, nil
}

func (ignore *Ignore) getIgnoreFilepath() string {
	return filepath.Join(ignore.path, ignore.context.IgnoreFilename)
}

func (ignore *Ignore) loadIgnore() error {
	if _, err := os.Stat(ignore.getIgnoreFilepath()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	file, err := os.Open(ignore.getIgnoreFilepath())
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && line[0] != '#' {
			ignore.itemList = append(ignore.itemList, line)
		}
	}
	return scanner.Err()
}

func (ignore *Ignore) shouldIgnore(name string) bool {
	return ignore.shouldIgnore2(name, "")
}

func (ignore *Ignore) shouldIgnore2(name string, fullname string) bool {
	for _, item := range ignore.itemList {
		if item[0] == '/' {
			if len(fullname) > 0 {
				continue
			} else {
				item = item[1:]
			}
		}
		if match, _ := filepath.Match(item, name); match {
			return true
		}
		if fullname != "" {
			if match, _ := filepath.Match(item, fullname); match {
				return true
			}
		}
	}
	if ignore.parentIgnore != nil {
		if fullname != "" {
			return ignore.parentIgnore.shouldIgnore2(fullname, ignore.name+fullname)
		} else {
			return ignore.parentIgnore.shouldIgnore2(name, ignore.name+name)
		}
	}
	return false
}
