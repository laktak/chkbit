package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/laktak/chkbit/v6"
	"github.com/laktak/chkbit/v6/cmd/chkbit/util"
	"github.com/laktak/chkbit/v6/intutil"
	"github.com/laktak/lterm"
)

func (m *Main) handleDedupProgress(showFps bool) {

	abortChan := make(chan os.Signal, 1)
	signal.Notify(abortChan, os.Interrupt)

	last := time.Now().Add(-updateInterval)
	spinnerChan := util.Spinner(500 * time.Millisecond)
	spin := " "
	stat := ""
	for {
		select {
		case <-abortChan:
			if m.dedup.DidAbort() {
				m.printStderr("Immediate abort!")
				os.Exit(1)
			}
			m.dedup.Abort()
			m.dedup.LogQueue <- &chkbit.LogEvent{Stat: chkbit.StatusPanic,
				Message: "Aborting after current operation (press again for immediate exit)"}
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
					pa, pb := util.Progress(perf.Percent, m.termWidth/4)
					stat = fmt.Sprintf("[$%s$%s$]$ %5.0f%% $ # %7d ", pa, pb, perf.Percent*100, m.dedup.NumTotal)

					if showFps {
						statF := fmt.Sprintf("%d files/s", m.fps.Last())
						stat += fmt.Sprintf("$ %s $%-13s ", util.Sparkline(m.fps.Stats), statF)
					}

					stat = util.LeftTruncate(stat, m.termWidth-1+5) // extra for col tokens

					stat = strings.Replace(stat, "$", termFG2, 1)                   // progress1
					stat = strings.Replace(stat, "$", termFG3, 1)                   // progress2
					stat = strings.Replace(stat, "$", termFG1, 1)                   // ]
					stat = strings.Replace(stat, "$", termFG1, 1)                   // text
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG1, 1) // count
					if showFps {
						stat = strings.Replace(stat, "$", termSepFG+termSep+termFG2, 1)
						stat = strings.Replace(stat, "$", termFG1, 1) // text
					}
				}
			}
		case spin = <-spinnerChan:
			if m.progress == Fancy {
				lterm.Write(termBG, termFG1, stat, spin, lterm.ClearLine(0), lterm.Reset, "\r")
			} else if m.progress == Plain {
				fmt.Print(m.dedup.NumTotal, "\r")
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

	showFps := true
	resultCh := make(chan error, 1)
	go func() {
		var err error
		switch dd.Mode {
		case "detect":
			err = m.dedup.DetectDupes(dd.MinSize, m.verbose)
		case "show":
			if list, err := m.dedup.Show(); err == nil {
				if dd.Json {
					if data, err := json.Marshal(&list); err == nil {
						fmt.Println(string(data))
					}
				} else {
					for i, bag := range list {
						fmt.Printf("#%d %s [%s, shared=%s, exclusive=%s]\n",
							i, bag.Hash, intutil.FormatSize(bag.Size),
							intutil.FormatSize(bag.SizeShared), intutil.FormatSize(bag.SizeExclusive))
						for _, item := range bag.ItemList {
							c := "-"
							if item.Merged {
								c = "+"
							}
							fmt.Println(c, item.Path)
						}
					}
				}
			}
		case "go":
			fmt.Printf("run dedup %s\n", dd.Hashes)
			err = m.dedup.Dedup(dd.Hashes)
		}
		resultCh <- err
		m.dedup.LogQueue <- nil
	}()

	if dd.Mode == "go" {
		showFps = false
	}

	m.handleDedupProgress(showFps)

	if err = <-resultCh; err != nil {
		m.printError(err)
		return 1
	}

	if m.progress == Fancy && m.dedup.NumTotal > 0 {
		elapsed := time.Since(m.fps.Start)
		elapsedS := elapsed.Seconds()
		m.logInfo("", fmt.Sprintf("- %s elapsed", elapsed.Truncate(time.Second)))
		m.logInfo("", fmt.Sprintf("- %.2f files/second", (float64(m.fps.Total)+float64(m.fps.Current))/elapsedS))
	}

	return 0
}
