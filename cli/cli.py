import os
import sys


class CLI:
    NO_COLOR = os.environ.get("NO_COLOR", "")

    class style:
        reset = "\033[0m"
        bold = "\033[01m"
        disable = "\033[02m"
        underline = "\033[04m"
        reverse = "\033[07m"
        strikethrough = "\033[09m"
        invisible = "\033[08m"

    class esc:
        up = "\033[A"
        down = "\033[B"
        right = "\033[C"
        left = "\033[D"

        @staticmethod
        def clear_line(opt=0):
            # 0=to end, 1=from start, 2=all
            return "\033[" + str(opt) + "K"

    @staticmethod
    def write(*text):
        for t in text:
            sys.stdout.write(str(t))
        sys.stdout.flush()

    @staticmethod
    def printline(*text):
        CLI.write(*text, CLI.esc.clear_line(), "\n")

    # 4bit system colors
    @staticmethod
    def fg4(col):
        # black=0,red=1,green=2,orange=3,blue=4,purple=5,cyan=6,lightgrey=7
        # darkgrey=8,lightred=9,lightgreen=10,yellow=11,lightblue=12,pink=13,lightcyan=14
        if CLI.NO_COLOR:
            return ""
        else:
            return f"\033[{(30+col) if col<8 else (90-8+col)}m"

    # 8bit xterm colors
    @staticmethod
    def fg8(col):
        if CLI.NO_COLOR:
            return ""
        else:
            return f"\033[38;5;{col}m"

    @staticmethod
    def bg8(col):
        if CLI.NO_COLOR:
            return ""
        else:
            return f"\033[48;5;{col}m"
