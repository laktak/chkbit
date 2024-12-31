package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/laktak/chkbit/v5"
	"github.com/laktak/chkbit/v5/cmd/chkbit/util"
	"github.com/laktak/lterm"
)

type Progress int

const (
	Quiet Progress = iota
	Summary
	Plain
	Fancy
)

type Command int

const (
	Check Command = iota
	Update
	Show
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

type CLI struct {
	Check struct {
		Paths []string `arg:""  name:"paths" help:"directories to check"`
	} `cmd:"" help:"chkbit will verify files in readonly mode"`

	Update struct {
		Paths   []string `arg:""  name:"paths" help:"directories to update"`
		AddOnly bool     `short:"a" help:"only add new and modified files, do not check existing (quicker)"`
		Force   bool     `help:"force update of damaged items (advanced usage only)"`
	} `cmd:"" help:"add and update indices (see flags with -h)"`

	Init struct {
		Mode  string `arg:"" enum:"split,atom" help:"the mode defines if you wish to store on index per directory (split) or one index in the given path (atom)"`
		Path  string `arg:"" help:"directory for the store root"`
		Force bool   `help:"force init if a store already exists"`
	} `cmd:"" help:"initialize a new index at the given path"`

	ShowIgnoredOnly struct {
		Paths []string `arg:""  name:"paths" help:"directories to list"`
	} `cmd:"" help:"only show ignored files"`

	Tips struct {
	} `cmd:"" help:"show tips"`

	Version struct {
	} `cmd:"" help:"show version information"`

	ShowMissing  bool   `short:"m" help:"show missing files/directories" negatable:""`
	IncludeDot   bool   `short:"d" help:"include dot files" negatable:""`
	SkipSymlinks bool   `short:"S" help:"do not follow symlinks" negatable:""`
	NoRecurse    bool   `short:"R" help:"do not recurse into subdirectories" negatable:""`
	NoDirInIndex bool   `short:"D" help:"do not track directories in the index" negatable:""`
	NoConfig     bool   `help:"ignore the config file"`
	LogFile      string `short:"l" help:"write to a logfile if specified"`
	LogVerbose   bool   `help:"verbose logging" negatable:""`
	Algo         string `default:"blake3" help:"hash algorithm: md5, sha512, blake3"`
	IndexName    string `default:".chkbit" help:"filename where chkbit stores its hashes, needs to start with '.'"`
	IgnoreName   string `default:".chkbitignore" help:"filename that chkbit reads its ignore list from, needs to start with '.'"`
	Workers      int    `short:"w" default:"5" help:"number of workers to use. For slow IO (like on a spinning disk) --workers=1 will be faster."`
	Plain        bool   `help:"show plain status instead of being fancy" negatable:""`
	Quiet        bool   `short:"q" help:"quiet, don't show progress/information" negatable:""`
	Verbose      bool   `short:"v" help:"verbose output" negatable:""`
}

type Main struct {
	context    *chkbit.Context
	dmgList    []string
	errList    []string
	verbose    bool
	logger     *log.Logger
	logVerbose bool
	progress   Progress
	termWidth  int
	fps        *util.RateCalc
	bps        *util.RateCalc
}

func (m *Main) logFile(text string) {
	m.logger.Println(time.Now().UTC().Format("2006-01-02 15:04:05"), text)
}

func (m *Main) logInfo(col, text string) {
	if m.progress != Quiet {
		if m.progress == Fancy {
			lterm.Printline(col, text, lterm.Reset)
		} else {
			fmt.Println(text)
		}
	}
	m.logFile(text)
}

func (m *Main) logError(text string) {
	text = chkbit.StatusPanic.String() + " " + text

	if m.progress == Fancy {
		lterm.Write(termAlertFG)
		fmt.Fprintln(os.Stderr, text)
		lterm.Write(lterm.Reset)
	} else {
		fmt.Fprintln(os.Stderr, text)
	}

	m.logFile(text)
}

func (m *Main) printError(text string) {
	if m.progress == Fancy {
		lterm.Write(termAlertFG)
		fmt.Fprintln(os.Stderr, text)
		lterm.Write(lterm.Reset)
	} else {
		fmt.Fprintln(os.Stderr, text)
	}
}

func (m *Main) logStatus(stat chkbit.Status, message string) bool {
	if stat == chkbit.StatusUpdateIndex {
		return false
	}

	if stat == chkbit.StatusErrorDamage {
		m.dmgList = append(m.dmgList, message)
	} else if stat == chkbit.StatusPanic {
		m.errList = append(m.errList, message)
	}

	if m.logVerbose || !stat.IsVerbose() {
		m.logFile(stat.String() + " " + message)
	}

	if m.verbose || !stat.IsVerbose() {

		if m.progress == Quiet && stat == chkbit.StatusInfo {
			return false
		}

		col := ""
		if stat.IsErrorOrWarning() {
			col = termAlertFG
		}
		lterm.Printline(col, stat.String(), " ", message, lterm.Reset)
		return true
	}
	return false
}

func (m *Main) showStatus() {
	last := time.Now().Add(-updateInterval)
	stat := ""
	for {
		select {
		case item := <-m.context.LogQueue:
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
					fmt.Print(m.context.NumTotal, "\r")
				}
			}
		case perf := <-m.context.PerfQueue:
			now := time.Now()
			m.fps.Push(now, perf.NumFiles)
			m.bps.Push(now, perf.NumBytes)
			if last.Add(updateInterval).Before(now) {
				last = now
				if m.progress == Fancy {
					statF := fmt.Sprintf("%d files/s", m.fps.Last())
					statB := fmt.Sprintf("%d MB/s", m.bps.Last()/sizeMB)
					stat = "RW"
					if !m.context.UpdateIndex {
						stat = "RO"
					}
					stat = fmt.Sprintf("[%s:%d] %5d files $ %s %-13s $ %s %-13s",
						stat, m.context.NumWorkers, m.context.NumTotal,
						util.Sparkline(m.fps.Stats), statF,
						util.Sparkline(m.bps.Stats), statB)
					stat = util.LeftTruncate(stat, m.termWidth-1)
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG2, 1)
					stat = strings.Replace(stat, "$", termSepFG+termSep+termFG3, 1)
					lterm.Write(termBG, termFG1, stat, lterm.ClearLine(0), lterm.Reset, "\r")
				} else if m.progress == Plain {
					fmt.Print(m.context.NumTotal, "\r")
				}
			}
		}
	}
}

func (m *Main) process(cmd Command, cli CLI) (bool, error) {

	var err error
	m.context, err = chkbit.NewContext(cli.Workers, cli.Algo, cli.IndexName, cli.IgnoreName)
	if err != nil {
		return false, err
	}

	var pathList []string
	switch cmd {
	case Check:
		pathList = cli.Check.Paths
		m.logFile("chkbit check " + strings.Join(pathList, ", "))
	case Update:
		pathList = cli.Update.Paths
		m.context.UpdateIndex = true
		m.context.AddOnly = cli.Update.AddOnly
		m.context.ForceUpdateDmg = cli.Update.Force
		m.logFile("chkbit update " + strings.Join(pathList, ", "))
	case Show:
		pathList = cli.ShowIgnoredOnly.Paths
		m.context.ShowIgnoredOnly = true
		m.logFile("chkbit show-ignored-only " + strings.Join(pathList, ", "))
	}

	m.context.ShowMissing = cli.ShowMissing
	m.context.IncludeDot = cli.IncludeDot
	m.context.SkipSymlinks = cli.SkipSymlinks
	m.context.SkipSubdirectories = cli.NoRecurse
	m.context.TrackDirectories = !cli.NoDirInIndex

	st, root, err := chkbit.LocateStore(pathList[0], chkbit.StoreTypeAny, m.context.IndexFilename)
	if err != nil {
		return false, err
	}

	if st == chkbit.StoreTypeAtom {
		pathList, err = m.context.UseAtomStore(root, pathList)
		if err == nil {
			// pathList is relative to root
			if err = os.Chdir(root); err != nil {
				return false, err
			}
			m.logInfo("", "Using atom-store in "+root)
		} else {
			return false, err
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.showStatus()
	}()
	m.context.Process(pathList)
	wg.Wait()

	return true, nil
}

func (m *Main) printResult() error {

	if m.progress != Quiet {
		mode := ""
		if !m.context.UpdateIndex {
			mode = " in readonly mode"
		}
		status := fmt.Sprintf("Processed %s%s", util.LangNum1MutateSuffix(m.context.NumTotal, "file"), mode)
		m.logInfo(termOKFG, status)

		if m.progress == Fancy && m.context.NumTotal > 0 {
			elapsed := time.Since(m.fps.Start)
			elapsedS := elapsed.Seconds()
			m.logInfo("", fmt.Sprintf("- %s elapsed", elapsed.Truncate(time.Second)))
			m.logInfo("", fmt.Sprintf("- %.2f files/second", (float64(m.fps.Total)+float64(m.fps.Current))/elapsedS))
			m.logInfo("", fmt.Sprintf("- %.2f MB/second", (float64(m.bps.Total)+float64(m.bps.Current))/float64(sizeMB)/elapsedS))
		}

		if m.context.UpdateIndex {
			if m.context.NumIdxUpd > 0 {
				m.logInfo(termOKFG, fmt.Sprintf("- %s updated", util.LangNum1Choice(m.context.NumIdxUpd, "directory was", "directories were")))
				m.logInfo(termOKFG, fmt.Sprintf("- %s added", util.LangNum1Choice(m.context.NumNew, "file hash was", "file hashes were")))
				m.logInfo(termOKFG, fmt.Sprintf("- %s updated", util.LangNum1Choice(m.context.NumUpd, "file hash was", "file hashes were")))
				if m.context.NumDel > 0 {
					m.logInfo(termOKFG, fmt.Sprintf("- %s been removed", util.LangNum1Choice(m.context.NumDel, "file/directory has", "files/directories have")))
				}
			}
		} else if m.context.NumNew+m.context.NumUpd+m.context.NumDel > 0 {
			m.logInfo(termAlertFG, "No changes were made")
			m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been added", util.LangNum1MutateSuffix(m.context.NumNew, "file")))
			m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been updated", util.LangNum1MutateSuffix(m.context.NumUpd, "file")))
			if m.context.NumDel > 0 {
				m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been removed", util.LangNum1Choice(m.context.NumDel, "file/directory", "files/directories")))
			}
		}
	}

	// summarize errors
	if len(m.dmgList) > 0 {
		m.printError("chkbit detected damage in these files:")
		for _, err := range m.dmgList {
			fmt.Fprintln(os.Stderr, err)
		}
		n := len(m.dmgList)
		status := fmt.Sprintf("error: detected %s with damage!", util.LangNum1MutateSuffix(n, "file"))
		m.logFile(status)
		m.printError(status)
	}

	if len(m.errList) > 0 {
		status := "chkbit ran into errors"
		m.logFile(status + "!")
		m.printError(status + ":")
		for _, err := range m.errList {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	if len(m.dmgList) > 0 || len(m.errList) > 0 {
		return errors.New("fail")
	}
	return nil
}

func (m *Main) run() int {

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}

	var configPath = "chkbit-config.json"
	configRoot, err := os.UserConfigDir()
	if err == nil {
		configPath = filepath.Join(configRoot, "chkbit/config.json")
	}

	var cli CLI
	var ctx *kong.Context
	var cmd Command
	ctx = kong.Parse(&cli,
		kong.Name("chkbit"),
		kong.Description(headerHelp),
		kong.UsageOnError(),
		kong.Configuration(kong.JSON, configPath),
	)

	if cli.NoConfig {
		cli = CLI{}
		ctx = kong.Parse(&cli,
			kong.Name("chkbit"),
			kong.Description(headerHelp),
			kong.UsageOnError(),
		)
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

	switch ctx.Command() {
	case "check <paths>":
		cmd = Check
	case "update <paths>":
		cmd = Update
	case "show-ignored-only <paths>":
		cmd = Show
	case "init <mode> <path>":
		m.logInfo("", fmt.Sprintf("chkbit init %s %s", cli.Init.Mode, cli.Init.Path))
		st := chkbit.StoreTypeSplit
		if cli.Init.Mode == "atom" {
			st = chkbit.StoreTypeAtom
		}
		if err := chkbit.InitializeStore(st, cli.Init.Path, cli.IndexName, cli.Init.Force); err != nil {
			m.logError(err.Error())
			return 1
		}
		return 0
	case "tips":
		fmt.Println(strings.ReplaceAll(helpTips, "<config-file>", configPath))
		return 0
	case "version":
		fmt.Println("github.com/laktak/chkbit")
		fmt.Println(appVersion)
		return 0
	default:
		fmt.Println("unknown: " + ctx.Command())
		return 1
	}

	m.verbose = cli.Verbose || cmd == Show
	if cli.LogFile != "" {
		m.logVerbose = cli.LogVerbose
		f, err := os.OpenFile(cli.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Println("error: " + err.Error())
			return 1
		}
		defer f.Close()
		m.logger = log.New(f, "", 0)
	}

	if showRes, err := m.process(cmd, cli); err == nil {
		if showRes && cmd != Show {
			if m.printResult() != nil {
				return 1
			}
		}
	} else {
		fmt.Println("error: " + err.Error())
		return 1
	}

	return 0
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			// panic
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
	os.Exit(m.run())
}
