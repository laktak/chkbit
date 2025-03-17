package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/laktak/chkbit/v6"
	"github.com/laktak/chkbit/v6/cmd/chkbit/util"
	"github.com/laktak/chkbit/v6/intutil"
	"github.com/laktak/lterm"
)

func (m *Main) handleDedupProgress(mode1 bool) {

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
					pa, pb := util.Progress(perf.Percent, int(math.Min(12, float64(m.termWidth/4))))
					stat = fmt.Sprintf("[$%s$%s$]$ %5.0f%% ", pa, pb, perf.Percent*100)

					if mode1 {
						stat += fmt.Sprintf("$ # %7d ", m.dedup.NumTotal)
						statF := fmt.Sprintf("%d files/s", m.fps.Last())
						stat += fmt.Sprintf("$ %s $%-13s ", util.Sparkline(m.fps.Stats), statF)
					} else {
						stat += fmt.Sprintf("$ # %d ", m.dedup.NumTotal)
						stat += fmt.Sprintf("$ %sB reclaimed ", intutil.FormatSize(m.dedup.ReclaimedTotal))
					}

					stat = util.LeftTruncate(stat, m.termWidth-1+5) // extra for col tokens

					stat = strings.Replace(stat, "$", termFG2, 1)                   // progress1
					stat = strings.Replace(stat, "$", termFG3, 1)                   // progress2
					stat = strings.Replace(stat, "$", termFG1, 1)                   // ]
					stat = strings.Replace(stat, "$", termFG1, 1)                   // text
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG1, 1) // count
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG2, 1)
					if mode1 {
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

func (m *Main) showDedupStatus(list []*chkbit.DedupBag, showDetails bool) {

	chash := uint64(0)
	cfile := uint64(0)
	minsize := uint64(0)
	maxsize := uint64(0)
	actsize := uint64(0)
	extUnknownCount := 0
	for i, bag := range list {

		bagLen := uint64(len(bag.ItemList))
		chash += 1
		cfile += bagLen
		minsize += bag.Size
		maxsize += bag.Size * bagLen
		actsize += bag.SizeExclusive
		bagUnknown := bag.ExtUnknown != nil && *bag.ExtUnknown
		if bagUnknown {
			extUnknownCount++
		}

		if showDetails {
			if !bagUnknown {
				fmt.Printf("#%d %s [%s, shared=%s, exclusive=%s]\n",
					i, bag.Hash, intutil.FormatSize(bag.Size),
					intutil.FormatSize(bag.SizeShared), intutil.FormatSize(bag.SizeExclusive))
			} else {
				fmt.Printf("#%d %s [%s*]\n",
					i, bag.Hash, intutil.FormatSize(bag.Size))
			}
			for _, item := range bag.ItemList {
				c := "-"
				if item.Merged {
					c = "+"
				}
				fmt.Println(c, item.Path)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Detected %d hashes that are shared by %d files:\n", chash, cfile)
	if extUnknownCount*2 > len(list) {
		fmt.Printf("- Used space:             %s\n", intutil.FormatSize(actsize))
		fmt.Printf("\n*) failed to load file-extents on this OS/filesystem for\n"+
			"   %.2f%% of files, cannot show details and reclaimable\n   space\n", (float64(extUnknownCount)/float64(len(list)))*100)
	} else {
		fmt.Printf("- Minimum required space: %s\n", intutil.FormatSize(minsize))
		fmt.Printf("- Maximum required space: %s\n", intutil.FormatSize(maxsize))
		fmt.Printf("- Actual used space:      %s\n", intutil.FormatSize(actsize))
		fmt.Printf("- Reclaimable space:      %s\n", intutil.FormatSize(actsize-minsize))
		if maxsize-minsize > 0 {
			fmt.Printf("- Efficiency:             %.2f%%\n", (1-(float64(actsize-minsize)/float64(maxsize-minsize)))*100)
		}
		if extUnknownCount > 0 {
			fmt.Printf("\n*) failed to load file-extents on this OS/filesystem for\n"+
				"   %.2f%% of files, shown data is not accurate\n", (float64(extUnknownCount)/float64(len(list)))*100)

		}
	}
}

func (m *Main) runDedup(command string, dd *CLIDedup, indexName string) int {
	var err error

	var argPath string
	switch command {
	case cmdDedupDetect:
		argPath = dd.Detect.Path
	case cmdDedupShow:
		argPath = dd.Show.Path
	case cmdDedupRun:
		argPath = dd.Run.Path
	}

	st, root, err := chkbit.LocateIndex(argPath, chkbit.IndexTypeAny, indexName)
	if err != nil {
		m.printError(err)
		return 1
	}
	if st != chkbit.IndexTypeAtom {
		fmt.Println("error: dedup is incompatible with split mode")
		return 1
	}

	m.dedup, err = chkbit.NewDedup(root, indexName)
	if err != nil {
		m.printError(err)
		return 1
	}
	defer m.dedup.Finish()

	mode1 := true
	resultCh := make(chan error, 1)
	launchFunc := func() {
		var err error
		switch command {
		case cmdDedupDetect:
			err = m.dedup.DetectDupes(dd.Detect.MinSize, m.verbose)
		case cmdDedupRun, cmdDedupRun2:
			err = m.dedup.Dedup(dd.Run.Hashes, m.verbose)
		}
		resultCh <- err
		m.dedup.LogQueue <- nil
	}

	switch command {
	case cmdDedupShow:
		if list, err := m.dedup.Show(); err == nil {
			if dd.Show.Json {
				if data, err := json.Marshal(&list); err == nil {
					fmt.Println(string(data))
				}
			} else {
				m.logInfo("", "chkbit dedup show "+argPath)
				m.showDedupStatus(list, dd.Show.Details)
			}
		}
		return 0
	case cmdDedupDetect:
		m.logInfo("", "chkbit dedup detect "+argPath)
		fmt.Println(abortTip)
	case cmdDedupRun, cmdDedupRun2:
		m.logInfo("", fmt.Sprintf("chkbit dedup detect %s %s", argPath, dd.Run.Hashes))
		fmt.Println(abortTip)
		mode1 = false
	}

	go launchFunc()
	m.handleDedupProgress(mode1)

	if err = <-resultCh; err != nil {
		m.printError(err)
		if !chkbit.IsAborted(err) {
			return 1
		}
	}

	if m.progress == Fancy && m.dedup.NumTotal > 0 {
		elapsed := time.Since(m.fps.Start)
		elapsedS := elapsed.Seconds()
		m.logInfo("", fmt.Sprintf("- %s elapsed", elapsed.Truncate(time.Second)))
		m.logInfo("", fmt.Sprintf("- %d file(s) processed", m.dedup.NumTotal))
		m.logInfo("", fmt.Sprintf("- %.2f files/second", (float64(m.fps.Total)+float64(m.fps.Current))/elapsedS))
		if m.dedup.ReclaimedTotal > 0 {
			m.logInfo("", fmt.Sprintf("- %sB reclaimed", intutil.FormatSize(m.dedup.ReclaimedTotal)))
		}
	}

	switch command {
	case cmdDedupDetect:
		if list, err := m.dedup.Show(); err == nil {
			m.showDedupStatus(list, dd.Show.Details)
		}
	}

	return 0
}
