package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	slpath "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/laktak/chkbit/v6"
	"github.com/laktak/chkbit/v6/cmd/chkbit/util"
	"github.com/laktak/chkbit/v6/intutil"
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
	abortTip             = "> you can abort by pressing control+c"
)

const (
	cmdCheck         = "check <paths>"
	cmdAdd           = "add <paths>"
	cmdUpdate        = "update <paths>"
	cmdShowIgnored   = "show-ignored <paths>"
	cmdInit          = "init <mode> <path>"
	cmdFuse          = "fuse <path>"
	cmdDedupDetect   = "dedup detect <path>"
	cmdDedupShow     = "dedup show <path>"
	cmdDedupRun      = "dedup run <path>"
	cmdDedupRun2     = "dedup run <path> <hashes>"
	cmdUtilFileext   = "util fileext <paths>"
	cmdUtilFilededup = "util filededup <paths>"
	cmdTips          = "tips"
	cmdVersion       = "version"
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
	termDimFG   = lterm.Fg8(240)
)

type CLI struct {
	Check struct {
		Paths   []string `arg:"" name:"paths" help:"directories to check"`
		SkipNew bool     `short:"s" help:"verify index only, do not report new files"`
	} `cmd:"" help:"chkbit will verify files in readonly mode"`

	Add struct {
		Paths []string `arg:"" name:"paths" help:"directories to add"`
	} `cmd:"" help:"add and update modified files (alias for chkbit update -s)"`

	Update struct {
		Paths        []string `arg:"" name:"paths" help:"directories to update"`
		SkipExisting bool     `short:"s" help:"only add new and modified files, do not check existing (quicker)"`
		Force        bool     `help:"force update of damaged items (advanced usage only)"`
	} `cmd:"" help:"add and update modified files, also checking existing ones (see chkbit update -h)"`

	Init struct {
		Mode  string `arg:"" enum:"split,atom" help:"{split|atom} split mode creates one index per directory while in atom mode a single index is created at the given path"`
		Path  string `arg:"" help:"directory for the index"`
		Force bool   `help:"force init if a index already exists"`
	} `cmd:"" help:"initialize a new index at the given path that manages the path and all its subfolders (see chkbit init -h)"`

	Fuse struct {
		Path  string `arg:"" help:"directory for the index"`
		Force bool   `help:"force overwrite if a index already exists"`
	} `cmd:"" help:"merge all indexes (split&atom) under this path into a single atom index, will NOT delete the source indexes"`

	Dedup CLIDedup `cmd:"" help:"Deduplication commands"`

	Util struct {
		Fileext struct {
			Paths []string `arg:"" name:"paths" help:"files to check"`
		} `cmd:"" help:"check if the given files occupy the same block on disk"`

		Filededup struct {
			Paths []string `arg:"" name:"paths" help:"files to dedup"`
		} `cmd:"" help:"run deduplication for the given files, makes all duplicate file blocks point to the same space"`
	} `cmd:"" help:"Utility functions; requires supported OS & filesystem (see tips)"`

	ShowIgnored struct {
		Paths []string `arg:"" name:"paths" help:"directories to list"`
	} `cmd:"" help:"show ignored files (see tips)"`

	Tips struct {
	} `cmd:"" help:"show tips"`

	Version struct {
	} `cmd:"" help:"show version information"`

	LogDeleted   bool   `short:"x" help:"log deleted/missing files/directories since the last run" negatable:""`
	IncludeDot   bool   `short:"d" help:"include dot files" negatable:""`
	SkipSymlinks bool   `short:"S" help:"do not follow symlinks" negatable:""`
	NoRecurse    bool   `short:"R" help:"do not recurse into subdirectories" negatable:""`
	NoDirInIndex bool   `short:"D" help:"do not track directories in the index" negatable:""`
	NoConfig     bool   `help:"ignore the config file"`
	MaxDepth     int    `default:0 help:"process a directory only if it is N or fewer levels below the specified path(s); 0 for no limit"`
	LogFile      string `short:"l" help:"write to a logfile if specified"`
	LogVerbose   bool   `help:"verbose logging" negatable:""`
	Algo         string `default:"blake3" help:"hash algorithm: md5, sha512, blake3"`
	IndexName    string `default:".chkbit" help:"filename where chkbit stores its hashes, needs to start with '.'"`
	IgnoreName   string `default:".chkbitignore" help:"filename that chkbit reads its ignore list from, needs to start with '.'"`
	Workers      int    `short:"w" default:"5" help:"number of workers to use. For slow IO (like on a spinning disk) --workers=1 will be faster"`
	Plain        bool   `help:"show plain status instead of being fancy" negatable:""`
	Quiet        bool   `short:"q" help:"quiet, don't show progress/information" negatable:""`
	Verbose      bool   `short:"v" help:"verbose output" negatable:""`
}

type CLIDedup struct {
	Detect struct {
		Path    string `arg:"" help:"directory for the index"`
		MinSize uint64 `default:8192 help:"minimum file size"`
	} `cmd:"" help:"use the atom index to detect duplicates"`

	Show struct {
		Path    string `arg:"" help:"directory for the index"`
		Details bool   `short:"f" help:"show file details" negatable:""`
		Json    bool   `short:"j" help:"output json" negatable:""`
	} `cmd:"" help:"show detected duplicate status"`

	Run struct {
		Path   string   `arg:"" help:"directory for the index"`
		Hashes []string `arg:"" optional:"" name:"hashes" help:"hashes to select (all if not specified)"`
	} `cmd:"" help:"run deduplication, makes all duplicate file blocks point to the same space; requires supported OS & filesystem (see tips)"`
}

func toSlash(paths []string) []string {
	for i, path := range paths {
		paths[i] = filepath.ToSlash(path)
	}
	return paths
}

func (cli *CLI) toSlash() {
	cli.Check.Paths = toSlash(cli.Check.Paths)
	cli.Add.Paths = toSlash(cli.Add.Paths)
	cli.Update.Paths = toSlash(cli.Update.Paths)
	cli.Init.Path = filepath.ToSlash(cli.Init.Path)
	cli.Fuse.Path = filepath.ToSlash(cli.Fuse.Path)
	cli.Dedup.Detect.Path = filepath.ToSlash(cli.Dedup.Detect.Path)
	cli.Dedup.Show.Path = filepath.ToSlash(cli.Dedup.Show.Path)
	cli.Dedup.Run.Path = filepath.ToSlash(cli.Dedup.Run.Path)
	cli.Util.Fileext.Paths = toSlash(cli.Util.Fileext.Paths)
	cli.Util.Filededup.Paths = toSlash(cli.Util.Filededup.Paths)
	cli.ShowIgnored.Paths = toSlash(cli.ShowIgnored.Paths)
}

type Main struct {
	context    *chkbit.Context
	dedup      *chkbit.Dedup
	dmgList    []string
	errList    []string
	verbose    bool
	hideNew    bool
	logger     *log.Logger
	logVerbose bool
	progress   Progress
	termWidth  int
	fps        *util.RateCalc
	bps        *util.RateCalc
}

func (m *Main) log(text string) {
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
	m.log(text)
}

func (m *Main) printStderr(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

func (m *Main) printErr(text string) {
	if m.progress == Fancy {
		lterm.Write(termAlertFG)
		m.printStderr(text)
		lterm.Write(lterm.Reset)
	} else {
		m.printStderr(text)
	}
}

func (m *Main) printError(err error) {
	m.printErr("error: " + err.Error())
}

func (m *Main) logStatus(stat chkbit.Status, message string) bool {
	if stat == chkbit.StatusUpdateIndex || m.hideNew && stat == chkbit.StatusNew {
		return false
	}

	if stat == chkbit.StatusErrorDamage {
		m.dmgList = append(m.dmgList, message)
	} else if stat == chkbit.StatusPanic {
		m.errList = append(m.errList, message)
	}

	if m.logVerbose || !stat.IsVerbose() {
		m.log(stat.String() + " " + message)
	}

	if m.verbose || !stat.IsVerbose() {

		if m.progress == Quiet && stat == chkbit.StatusInfo {
			return false
		}

		col := lterm.Reset
		col1 := termDimFG
		if stat.IsErrorOrWarning() {
			col = termAlertFG
			col1 = col
		}
		lterm.Printline(col1, stat.String(), " ", col, message, lterm.Reset)
		return true
	}
	return false
}

func (m *Main) handleProgress() {

	abortChan := make(chan os.Signal, 1)
	signal.Notify(abortChan, os.Interrupt)

	last := time.Now().Add(-updateInterval)
	stat := ""
	for {
		select {
		case <-abortChan:
			m.context.Abort()
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

func (m *Main) runCmd(command string, cli CLI) int {
	var err error
	m.context, err = chkbit.NewContext(cli.Workers, cli.Algo, cli.IndexName, cli.IgnoreName)
	if err != nil {
		m.printError(err)
		return 1
	}

	var pathList []string
	switch command {
	case cmdCheck:
		pathList = cli.Check.Paths
		m.log("chkbit check " + strings.Join(pathList, ", "))
		m.hideNew = cli.Check.SkipNew
	case cmdUpdate:
		pathList = cli.Update.Paths
		m.context.UpdateIndex = true
		m.context.UpdateSkipCheck = cli.Update.SkipExisting
		m.context.ForceUpdateDmg = cli.Update.Force
		m.log("chkbit update " + strings.Join(pathList, ", "))
	case cmdShowIgnored:
		pathList = cli.ShowIgnored.Paths
		m.verbose = true
		m.context.ShowIgnoredOnly = true
		m.log("chkbit show-ignored " + strings.Join(pathList, ", "))
	}

	m.context.LogDeleted = cli.LogDeleted
	m.context.IncludeDot = cli.IncludeDot
	m.context.SkipSymlinks = cli.SkipSymlinks
	m.context.SkipSubdirectories = cli.NoRecurse
	m.context.TrackDirectories = !cli.NoDirInIndex
	m.context.MaxDepth = cli.MaxDepth

	st, root, err := chkbit.LocateIndex(pathList[0], chkbit.IndexTypeAny, m.context.IndexFilename)
	if err != nil {
		m.printError(err)
		return 1
	}

	if st == chkbit.IndexTypeAtom {
		pathList, err = m.context.UseAtomIndexStore(root, pathList)
		if err == nil {
			// pathList is relative to root
			if err = os.Chdir(root); err != nil {
				m.printError(err)
				return 1
			}
			m.logInfo("", "Using atom-index in "+root)
		} else {
			m.printError(err)
			return 1
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.handleProgress()
	}()
	m.context.Process(pathList)
	wg.Wait()

	if command == cmdShowIgnored {
		return 0
	}

	// result
	numIdxUpd := m.context.NumIdxUpd
	numNew := m.context.NumNew
	numUpd := m.context.NumUpd
	if m.hideNew {
		numNew = 0
	}

	didUpdate := m.context.UpdateIndex
	if m.context.DidAbort() {
		if m.context.GetIndexType() == chkbit.IndexTypeAtom {
			didUpdate = false
		}
	}

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

		if didUpdate {
			if numIdxUpd > 0 {
				m.logInfo(termOKFG, fmt.Sprintf("- %s updated", util.LangNum1Choice(numIdxUpd, "directory was", "directories were")))
				m.logInfo(termOKFG, fmt.Sprintf("- %s added", util.LangNum1Choice(numNew, "file hash was", "file hashes were")))
				m.logInfo(termOKFG, fmt.Sprintf("- %s updated", util.LangNum1Choice(numUpd, "file hash was", "file hashes were")))
				if m.context.NumDel > 0 {
					m.logInfo(termOKFG, fmt.Sprintf("- %s been removed", util.LangNum1Choice(m.context.NumDel, "file/directory has", "files/directories have")))
				}
			}
		} else if numNew+numUpd+m.context.NumDel > 0 {
			m.logInfo(termAlertFG, "No changes were made")
			m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been added", util.LangNum1MutateSuffix(numNew, "file")))
			m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been updated", util.LangNum1MutateSuffix(numUpd, "file")))
			if m.context.NumDel > 0 {
				m.logInfo(termAlertFG, fmt.Sprintf("- %s would have been removed", util.LangNum1Choice(m.context.NumDel, "file/directory", "files/directories")))
			}
		}
	}

	// summarize errors
	if len(m.dmgList) > 0 {
		m.printErr("chkbit detected damage in these files:")
		for _, item := range m.dmgList {
			m.printStderr(item)
		}
		n := len(m.dmgList)
		status := fmt.Sprintf("error: detected %s with damage!", util.LangNum1MutateSuffix(n, "file"))
		m.log(status)
		m.printErr(status)
	}

	if len(m.errList) > 0 {
		status := "chkbit ran into errors"
		m.log(status + "!")
		m.printErr(status + ":")
		for _, item := range m.errList {
			m.printStderr(item)
		}
	}

	if m.context.DidAbort() {
		m.logInfo(termAlertFG, "Aborted")
	}

	if len(m.dmgList) > 0 || len(m.errList) > 0 {
		return 1
	}
	return 0
}

func (m *Main) run() int {

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}

	var configPath = "chkbit-config.json"
	configRoot, err := os.UserConfigDir()
	if err == nil {
		configPath = slpath.Join(configRoot, "chkbit/config.json")
	}

	var cli CLI
	var ctx *kong.Context
	kongOptions := []kong.Option{
		kong.Name("chkbit"),
		kong.Description(headerHelp),
		kong.ConfigureHelp(kong.HelpOptions{Tree: true, FlagsLast: true}),
		kong.UsageOnError(),
	}

	ctx = kong.Parse(&cli, append(kongOptions, kong.Configuration(kong.JSON, configPath))...)

	if cli.NoConfig {
		cli = CLI{}
		ctx = kong.Parse(&cli, kongOptions...)
	}

	cli.toSlash()

	if cli.Quiet {
		m.progress = Quiet
	} else if fileInfo, err := os.Stdout.Stat(); err == nil && (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		m.progress = Summary
	} else if cli.Plain {
		m.progress = Plain
	} else {
		m.progress = Fancy
	}

	m.verbose = cli.Verbose
	if cli.LogFile != "" {
		m.logVerbose = cli.LogVerbose
		f, err := os.OpenFile(cli.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			m.printError(err)
			return 1
		}
		defer f.Close()
		m.logger = log.New(f, "", 0)
	}

	cmd := ctx.Command()
	switch cmd {
	case cmdCheck, cmdUpdate, cmdShowIgnored:
		return m.runCmd(cmd, cli)
	case cmdAdd:
		cli.Update.Paths = cli.Add.Paths
		cli.Update.SkipExisting = true
		return m.runCmd(cmdUpdate, cli)
	case cmdInit:
		m.logInfo("", fmt.Sprintf("chkbit init %s %s", cli.Init.Mode, cli.Init.Path))
		st := chkbit.IndexTypeSplit
		if cli.Init.Mode == "atom" {
			st = chkbit.IndexTypeAtom
		}
		if err := chkbit.InitializeIndexStore(st, cli.Init.Path, cli.IndexName, cli.Init.Force); err != nil {
			text := chkbit.StatusPanic.String() + " " + err.Error()
			m.printErr(text)
			m.log(text)
			return 1
		}
		return 0
	case cmdFuse:
		m.logInfo("", fmt.Sprintf("chkbit fuse %s", cli.Fuse.Path))
		log := func(text string) {
			m.logInfo("", text)
		}
		if err := chkbit.FuseIndexStore(cli.Fuse.Path, cli.IndexName, cli.SkipSymlinks, cli.Verbose, cli.Fuse.Force, log); err != nil {
			text := chkbit.StatusPanic.String() + " " + err.Error()
			m.printErr(text)
			m.log(text)
			return 1
		}
		return 0
	case cmdDedupDetect, cmdDedupShow, cmdDedupRun, cmdDedupRun2:
		return m.runDedup(cmd, &cli.Dedup, cli.IndexName)

	case cmdUtilFileext:
		paths := cli.Util.Fileext.Paths
		allMatch := true
		var first chkbit.FileExtentList
		for i, path := range paths {
			blocks, err := chkbit.GetFileExtents(path)
			if err != nil {
				m.printError(err)
				return 1
			}
			if i == 0 {
				first = blocks
			} else {
				if !chkbit.ExtentsMatch(first, blocks) {
					m.printErr(fmt.Sprintf("Files do not occupie the same blocks (%s, %s).", paths[0], path))
					allMatch = false
				}
			}
			if m.verbose || len(paths) == 1 {
				fmt.Println(path)
				fmt.Print(chkbit.ShowExtents(blocks))
			}
		}
		if len(paths) > 1 && allMatch {
			fmt.Println("Files occupie the same blocks.")
			return 0
		}
		return 1

	case cmdUtilFilededup:
		paths := cli.Util.Filededup.Paths
		if len(paths) < 2 {
			fmt.Println("error: supply two or more paths")
			return 1
		}
		var reclaimedTotal uint64
		var first string
		for i, path := range paths {
			if i == 0 {
				first = path
			} else {
				if reclaimed, err := chkbit.DeduplicateFiles(first, path); err != nil {
					m.printErr(fmt.Sprintf("Unable to deduplicate (%s, %s): %s", paths[0], path, err.Error()))
					return 1
				} else {
					reclaimedTotal += reclaimed
				}
			}
		}
		fmt.Printf("Dedup success, reclaimed %s.\n", intutil.FormatSize(reclaimedTotal))
		return 0

	case cmdTips:
		fmt.Println(strings.ReplaceAll(helpTips, "<config-file>", configPath))
		return 0
	case cmdVersion:
		fmt.Println("github.com/laktak/chkbit")
		fmt.Println(appVersion)
		return 0
	default:
		fmt.Println("unknown: " + cmd)
		return 1
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			// panic
			fmt.Fprintln(os.Stderr, r)
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
