package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/laktak/chkbit/v6"
	"github.com/laktak/chkbit/v6/cmd/chkbit/util"
	"github.com/laktak/lterm"
)

func (m *Main) handleDedupProgress() {

	abortChan := make(chan os.Signal, 1)
	signal.Notify(abortChan, os.Interrupt)

	last := time.Now().Add(-updateInterval)
	stat := ""
	for {
		select {
		case <-abortChan:
			m.dedup.Abort()
		case item := <-m.dedup.LogQueue:
			if item == nil {
				if m.progress == Fancy {
					lterm.Printline("")
				}
				return
			}
			if m.logStatus(item.Stat, item.Message) {
				if m.progress == Fancy {
					lterm.Write(termBG, termFG1, stat, lterm.ClearLine(0), lterm.Reset, "\r")
				} else {
					fmt.Print(m.dedup.NumTotal, "\r")
				}
			}
		case perf := <-m.dedup.PerfQueue:
			now := time.Now()
			m.fps.Push(now, perf.NumFiles)
			if last.Add(updateInterval).Before(now) {
				last = now
				if m.progress == Fancy {
					statF := fmt.Sprintf("%d files/s", m.fps.Last())
					stat = fmt.Sprintf("%5d files $ %s %-13s", m.dedup.NumTotal, util.Sparkline(m.fps.Stats), statF)
					stat = util.LeftTruncate(stat, m.termWidth-1)
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG2, 1)
					lterm.Write(termBG, termFG1, stat, lterm.ClearLine(0), lterm.Reset, "\r")
				} else if m.progress == Plain {
					fmt.Print(m.dedup.NumTotal, "\r")
				}
			}
		}
	}
}

func (m *Main) runDedup(dd *CLIDedup, indexName string, root string) int {
	var err error
	m.dedup, err = chkbit.NewDedup(root, indexName)
	if err != nil {
		m.printError(err)
		return 1
	}
	defer m.dedup.Finish()

	resultCh := make(chan error, 1)
	go func() {
		var err error
		switch dd.Mode {
		case "detect":
			// todo fmt.Printf("collect matching hashes (minimum size %s)"
			err = m.dedup.DetectDupes(dd.MinSize, m.verbose)
		case "show":
			if list, err := m.dedup.Show(); err == nil {
				for i, bag := range list {
					fmt.Printf("#%d %s [%s, shared=%s, exclusive=%s]\n",
						i, bag.Hash, util.FormatSize(bag.Size),
						util.FormatSize(bag.SizeShared), util.FormatSize(bag.SizeExclusive))
					for _, item := range bag.ItemList {
						c := "-"
						if item.Merged {
							c = "+"
						}
						fmt.Println(c, item.Path)
					}
				}
			}
			// todo show json
		case "go":
			err = m.dedup.Dedup(dd.Hashes)
		}
		resultCh <- err
		m.dedup.LogQueue <- nil
	}()

	m.handleDedupProgress()

	if err = <-resultCh; err != nil {
		m.printError(err)
		return 1
	}
	return 0
}
