package term

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

var (
	isTerm   = false
	noColor  = false
	stdoutFd = 0
)

func init() {
	stdoutFd = int(os.Stdout.Fd())
	isTerm = term.IsTerminal(stdoutFd)
	if isTerm {
		noColor = os.Getenv("NO_COLOR") != ""
	} else {
		noColor = true
	}
}

const (
	Reset         = "\033[0m"
	Bold          = "\033[01m"
	Disable       = "\033[02m"
	Underline     = "\033[04m"
	Reverse       = "\033[07m"
	Strikethrough = "\033[09m"
	Invisible     = "\033[08m"
)

func Write(text ...interface{}) {
	fmt.Print(text...)
}

func Printline(text ...interface{}) {
	fmt.Print(text...)
	fmt.Println(ClearLine(0))
}

func Fg4(col int) string {
	if noColor {
		return ""
	}
	if col < 8 {
		return fmt.Sprintf("\033[%dm", 30+col)
	}
	return fmt.Sprintf("\033[%dm", 90-8+col)
}

func Fg8(col int) string {
	if noColor {
		return ""
	}
	return fmt.Sprintf("\033[38;5;%dm", col)
}

func Bg8(col int) string {
	if noColor {
		return ""
	}
	return fmt.Sprintf("\033[48;5;%dm", col)
}

func ClearLine(opt int) string {
	// 0=to end, 1=from start, 2=all
	return fmt.Sprintf("\033[%dK", opt)
}

func GetWidth() int {
	if isTerm {
		width, _, err := term.GetSize(stdoutFd)
		if err == nil {
			return width
		}
	}
	return 80
}
