package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/laktak/chkbit"
	"github.com/laktak/chkbit/cmd/chkbit/util"
	"github.com/laktak/lterm"
)

type Progress int

const (
	Quiet Progress = iota
	Summary
	Plain
	Fancy
)

const (
	updateInterval       = time.Millisecond * 700
	sizeMB         int64 = 1024 * 1024
)

var appVersion = "vdev"
var (
	termBG      = lterm.Bg8(240)
	termSep     = "|"
	termSepFG   = lterm.Fg8(235)
	termFG1     = lterm.Fg8(255)
	termFG2     = lterm.Fg8(228)
	termFG3     = lterm.Fg8(202)
	termOKFG    = lterm.Fg4(2)
	termAlertFG = lterm.Fg4(1)
)

var cli struct {
	Paths           []string `arg:"" optional:"" name:"paths" help:"directories to check"`
	Tips            bool     `short:"H" help:"Show tips."`
	Update          bool     `short:"u" help:"update indices (without this chkbit will verify files in readonly mode)"`
	ShowIgnoredOnly bool     `help:"only show ignored files"`
	Algo            string   `default:"blake3" help:"hash algorithm: md5, sha512, blake3 (default: blake3)"`
	Force           bool     `short:"f" help:"force update of damaged items"`
	SkipSymlinks    bool     `short:"s" help:"do not follow symlinks"`
	LogFile         string   `short:"l" help:"write to a logfile if specified"`
	LogVerbose      bool     `help:"verbose logging"`
	IndexName       string   `default:".chkbit" help:"filename where chkbit stores its hashes, needs to start with '.' (default: .chkbit)"`
	IgnoreName      string   `default:".chkbitignore" help:"filename that chkbit reads its ignore list from, needs to start with '.' (default: .chkbitignore)"`
	Workers         int      `short:"w" default:"5" help:"number of workers to use (default: 5)"`
	Plain           bool     `help:"show plain status instead of being fancy"`
	Quiet           bool     `short:"q" help:"quiet, don't show progress/information"`
	Verbose         bool     `short:"v" help:"verbose output"`
	Version         bool     `short:"V" help:"show version information"`
}

type Main struct {
	dmgList    []string
	errList    []string
	numIdxUpd  int
	numNew     int
	numUpd     int
	verbose    bool
	logger     *log.Logger
	logVerbose bool
	progress   Progress
	total      int
	termWidth  int
	fps        *util.RateCalc
	bps        *util.RateCalc
}

func (m *Main) log(text string) {
	m.logger.Println(time.Now().UTC().Format("2006-01-02 15:04:05"), text)
}

func (m *Main) logStatus(stat chkbit.Status, message string) bool {
	if stat == chkbit.STATUS_UPDATE_INDEX {
		m.numIdxUpd++
	} else {
		if stat == chkbit.STATUS_ERR_DMG {
			m.total++
			m.dmgList = append(m.dmgList, message)
		} else if stat == chkbit.STATUS_PANIC {
			m.errList = append(m.errList, message)
		} else if stat == chkbit.STATUS_OK || stat == chkbit.STATUS_UPDATE || stat == chkbit.STATUS_NEW || stat == chkbit.STATUS_UP_WARN_OLD {
			m.total++
			if stat == chkbit.STATUS_UPDATE || stat == chkbit.STATUS_UP_WARN_OLD {
				m.numUpd++
			} else if stat == chkbit.STATUS_NEW {
				m.numNew++
			}
		}

		if m.logVerbose || stat != chkbit.STATUS_OK && stat != chkbit.STATUS_IGNORE {
			m.log(stat.String() + " " + message)
		}

		if m.verbose || !stat.IsVerbose() {
			col := ""
			if stat.IsErrorOrWarning() {
				col = termAlertFG
			}
			lterm.Printline(col, stat.String(), " ", message, lterm.Reset)
			return true
		}
	}
	return false
}

func (m *Main) showStatus(context *chkbit.Context) {
	last := time.Now().Add(-updateInterval)
	stat := ""
	for {
		select {
		case item := <-context.LogQueue:
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
					fmt.Print(m.total, "\r")
				}
			}
		case perf := <-context.PerfQueue:
			now := time.Now()
			m.fps.Push(now, perf.NumFiles)
			m.bps.Push(now, perf.NumBytes)
			if last.Add(updateInterval).Before(now) {
				last = now
				if m.progress == Fancy {
					statF := fmt.Sprintf("%d files/s", m.fps.Last())
					statB := fmt.Sprintf("%d MB/s", m.bps.Last()/sizeMB)
					stat = "RW"
					if !context.Update {
						stat = "RO"
					}
					stat = fmt.Sprintf("[%s:%d] %5d files $ %s %-13s $ %s %-13s",
						stat, context.NumWorkers, m.total,
						util.Sparkline(m.fps.Stats), statF,
						util.Sparkline(m.bps.Stats), statB)
					stat = util.LeftTruncate(stat, m.termWidth-1)
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG2, 1)
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG3, 1)
					lterm.Write(termBG, termFG1, stat, lterm.ClearLine(0), lterm.Reset, "\r")
				} else if m.progress == Plain {
					fmt.Print(m.total, "\r")
				}
			}
		}
	}
}

func (m *Main) process() *chkbit.Context {
	if cli.Update && cli.ShowIgnoredOnly {
		fmt.Println("Error: use either --update or --show-ignored-only!")
		return nil
	}

	context, err := chkbit.NewContext(cli.Workers, cli.Force, cli.Update, cli.ShowIgnoredOnly, cli.Algo, cli.SkipSymlinks, cli.IndexName, cli.IgnoreName)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.showStatus(context)
	}()
	context.Start(cli.Paths)
	wg.Wait()

	return context
}

func (m *Main) printResult(context *chkbit.Context) {
	cprint := func(col, text string) {
		if m.progress != Quiet {
			if m.progress == Fancy {
				lterm.Printline(col, text, lterm.Reset)
			} else {
				fmt.Println(text)
			}
		}
	}

	eprint := func(col, text string) {
		if m.progress == Fancy {
			lterm.Write(col)
			fmt.Fprintln(os.Stderr, text)
			lterm.Write(lterm.Reset)
		} else {
			fmt.Fprintln(os.Stderr, text)
		}
	}

	if m.progress != Quiet {
		mode := ""
		if !context.Update {
			mode = " in readonly mode"
		}
		status := fmt.Sprintf("Processed %s%s.", util.LangNum1MutateSuffix(m.total, "file"), mode)
		cprint(termOKFG, status)
		m.log(status)

		if m.progress == Fancy && m.total > 0 {
			elapsed := time.Since(m.fps.Start)
			elapsedS := elapsed.Seconds()
			fmt.Println("-", elapsed.Truncate(time.Second), "elapsed")
			fmt.Printf("- %.2f files/second\n", (float64(m.fps.Total)+float64(m.fps.Current))/elapsedS)
			fmt.Printf("- %.2f MB/second\n", (float64(m.bps.Total)+float64(m.bps.Current))/float64(sizeMB)/elapsedS)
		}

		if context.Update {
			if m.numIdxUpd > 0 {
				cprint(termOKFG, fmt.Sprintf("- %s updated\n- %s added\n- %s updated",
					util.LangNum1Choice(m.numIdxUpd, "directory was", "directories were"),
					util.LangNum1Choice(m.numNew, "file hash was", "file hashes were"),
					util.LangNum1Choice(m.numUpd, "file hash was", "file hashes were")))
			}
		} else if m.numNew+m.numUpd > 0 {
			cprint(termAlertFG, fmt.Sprintf("No changes were made (specify -u to update):\n- %s would have been added and\n- %s would have been updated.",
				util.LangNum1MutateSuffix(m.numNew, "file"),
				util.LangNum1MutateSuffix(m.numUpd, "file")))
		}
	}

	if len(m.dmgList) > 0 {
		eprint(termAlertFG, "chkbit detected damage in these files:")
		for _, err := range m.dmgList {
			fmt.Fprintln(os.Stderr, err)
		}
		n := len(m.dmgList)
		status := fmt.Sprintf("error: detected %s with damage!", util.LangNum1MutateSuffix(n, "file"))
		m.log(status)
		eprint(termAlertFG, status)
	}

	if len(m.errList) > 0 {
		status := "chkbit ran into errors"
		m.log(status + "!")
		eprint(termAlertFG, status+":")
		for _, err := range m.errList {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	if len(m.dmgList) > 0 || len(m.errList) > 0 {
		os.Exit(1)
	}
}

func (m *Main) run() {

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}

	kong.Parse(&cli,
		kong.Name("chkbit"),
		kong.Description(""),
		kong.UsageOnError(),
	)

	if cli.Tips {
		fmt.Println(helpTips)
		os.Exit(0)
	}

	if cli.Version {
		fmt.Println("github.com/laktak/chkbit")
		fmt.Println(appVersion)
		return
	}

	m.verbose = cli.Verbose || cli.ShowIgnoredOnly
	if cli.LogFile != "" {
		m.logVerbose = cli.LogVerbose
		f, err := os.OpenFile(cli.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer f.Close()
		m.logger = log.New(f, "", 0)
	}

	if cli.Quiet {
		m.progress = Quiet
	} else if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		m.progress = Summary
	} else if cli.Plain {
		m.progress = Plain
	} else {
		m.progress = Fancy
	}

	if len(cli.Paths) > 0 {
		m.log("chkbit " + strings.Join(cli.Paths, ", "))
		context := m.process()
		if context != nil && !context.ShowIgnoredOnly {
			m.printResult(context)
		}
	} else {
		fmt.Println("specify a path to check, see -h")
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			os.Exit(1)
		}
	}()

	termWidth := lterm.GetWidth()
	m := &Main{
		logger:    log.New(io.Discard, "", 0),
		termWidth: termWidth,
		fps:       util.NewRateCalc(time.Second, (termWidth-70)/2),
		bps:       util.NewRateCalc(time.Second, (termWidth-70)/2),
	}
	m.run()
}
