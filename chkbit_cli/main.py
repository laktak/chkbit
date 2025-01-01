import argparse
import logging
import os
import queue
import shutil
import sys
import threading
import time
from datetime import datetime, timedelta
from chkbit import Context, Status, IndexThread
from . import CLI, Progress, RateCalc, sparkify, __version__


EPILOG = """
.chkbitignore rules:
  each line should contain exactly one name
  you may use Unix shell-style wildcards (see README)
  lines starting with `#` are skipped
  lines starting with `/` are only applied to the current directory

Status codes:
  DMG: error, data damage detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  ign: ignored (see .chkbitignore)
  EXC: internal exception
"""

UPDATE_INTERVAL = timedelta(milliseconds=700)
MB = 1024 * 1024

CLI_BG = CLI.bg8(240)
CLI_SEP = "|"
CLI_SEP_FG = CLI.fg8(235)
CLI_FG1 = CLI.fg8(255)
CLI_FG2 = CLI.fg8(228)
CLI_FG3 = CLI.fg8(202)
CLI_OK_FG = CLI.fg4(2)
CLI_ALERT_FG = CLI.fg4(1)


class Main:
    def __init__(self):
        self.stdscr = None
        self.dmg_list = []
        self.err_list = []
        self.num_idx_upd = 0
        self.num_new = 0
        self.num_upd = 0
        self.verbose = False
        self.log = logging.getLogger("")
        self.log_verbose = False
        self.progress = Progress.Fancy
        self.total = 0
        self.term_width = shutil.get_terminal_size()[0]
        max_stat = int((self.term_width - 70) / 2)
        self.fps = RateCalc(timedelta(seconds=1), max_stat=max_stat)
        self.bps = RateCalc(timedelta(seconds=1), max_stat=max_stat)
        # disable
        self.log.setLevel(logging.CRITICAL + 1)

    def _log(self, stat: Status, path: str):
        if stat == Status.UPDATE_INDEX:
            self.num_idx_upd += 1
        else:
            if stat == Status.ERR_DMG:
                self.total += 1
                self.dmg_list.append(path)
            elif stat == Status.INTERNALEXCEPTION:
                self.err_list.append(path)
            elif stat in [Status.OK, Status.UPDATE, Status.NEW]:
                self.total += 1
                if stat == Status.UPDATE:
                    self.num_upd += 1
                elif stat == Status.NEW:
                    self.num_new += 1

            lvl = Status.get_level(stat)
            if self.log_verbose or not stat in [Status.OK, Status.IGNORE]:
                self.log.log(lvl, f"{stat.value} {path}")

            if self.verbose or not stat in [Status.OK, Status.IGNORE]:
                CLI.printline(
                    CLI_ALERT_FG if lvl >= logging.WARNING else "",
                    stat.value,
                    " ",
                    path,
                    CLI.style.reset,
                )

    def _res_worker(self, context: Context):
        last = datetime.now()
        while True:
            try:
                item = self.result_queue.get(timeout=0.2)
                now = datetime.now()
                if not item:
                    if self.progress == Progress.Fancy:
                        CLI.printline("")
                    break
                t, *p = item
                if t == 0:
                    self._log(*p)
                    last = datetime.min
                else:
                    self.fps.push(now, p[0])
                    self.bps.push(now, p[1])
                self.result_queue.task_done()
            except queue.Empty:
                now = datetime.now()
                pass
            if last + UPDATE_INTERVAL < now:
                last = now

                if self.progress == Progress.Fancy:
                    stat_f = f"{self.fps.last} files/s"
                    stat_b = f"{int(self.bps.last/MB)} MB/s"
                    stat = f"[{'RW' if context.update else 'RO'}:{context.num_workers}] {self.total:>5} files $ {sparkify(self.fps.stats)} {stat_f:13} $ {sparkify(self.bps.stats)} {stat_b}"
                    stat = stat[: self.term_width - 1]
                    stat = stat.replace("$", CLI_SEP_FG + CLI_SEP + CLI_FG2, 1)
                    stat = stat.replace("$", CLI_SEP_FG + CLI_SEP + CLI_FG3, 1)
                    CLI.write(
                        CLI_BG,
                        CLI_FG1,
                        stat,
                        CLI.esc.clear_line(),
                        CLI.style.reset,
                        "\r",
                    )
                elif self.progress == Progress.Plain:
                    print(self.total, end="\r")

    def process(self, args):
        if args.update and args.show_ignored_only:
            print("Error: use either --update or --show-ignored-only!", file=sys.stderr)
            return None

        context = Context(
            num_workers=args.workers,
            force=args.force,
            update=args.update,
            show_ignored_only=args.show_ignored_only,
            hash_algo=args.algo,
            skip_symlinks=args.skip_symlinks,
            index_filename=args.index_name,
            ignore_filename=args.ignore_name,
        )
        self.result_queue = context.result_queue

        # put the initial paths into the queue
        for path in args.paths:
            context.add_input(path)

        # start indexing
        workers = [IndexThread(i, context) for i in range(context.num_workers)]

        # log the results from the workers
        res_worker = threading.Thread(target=self._res_worker, args=(context,))
        res_worker.daemon = True
        res_worker.start()

        # wait for work to finish
        context.input_queue.join()

        # signal workers to exit
        for worker in workers:
            context.end_input()

        # signal res_worker to exit
        self.result_queue.put(None)

        for worker in workers:
            worker.join()
        res_worker.join()

        return context

    def print_result(self, context):
        def cprint(col, text):
            if self.progress == Progress.Fancy:
                CLI.printline(col, text, CLI.style.reset)
            else:
                print(text)

        def eprint(col, text):
            if self.progress == Progress.Fancy:
                CLI.write(col)
                print(text, file=sys.stderr)
                CLI.write(CLI.style.reset)
            else:
                print(text, file=sys.stderr)

        iunit = lambda x, u: f"{x} {u}{'s' if x!=1 else ''}"
        iunit2 = lambda x, u1, u2: f"{x} {u2 if x!=1 else u1}"

        if self.progress != Progress.Quiet:
            status = f"Processed {iunit(self.total, 'file')}{' in readonly mode' if not context.update else ''}."
            cprint(CLI_OK_FG, status)
            self.log.info(status)

            if self.progress == Progress.Fancy and self.total > 0:
                elapsed = datetime.now() - self.fps.start
                elapsed_s = elapsed.total_seconds()
                print(f"- {str(elapsed).split('.')[0]} elapsed")
                print(
                    f"- {(self.fps.total+self.fps.current)/elapsed_s:.2f} files/second"
                )
                print(
                    f"- {(self.bps.total+self.bps.current)/MB/elapsed_s:.2f} MB/second"
                )

            if context.update:
                if self.num_idx_upd:
                    cprint(
                        CLI_OK_FG,
                        f"- {iunit2(self.num_idx_upd, 'directory was', 'directories were')} updated\n"
                        + f"- {iunit2(self.num_new, 'file hash was', 'file hashes were')} added\n"
                        + f"- {iunit2(self.num_upd, 'file hash was', 'file hashes were')} updated",
                    )
            elif self.num_new + self.num_upd > 0:
                cprint(
                    CLI_ALERT_FG,
                    f"No changes were made (specify -u to update):\n"
                    + f"- {iunit(self.num_new, 'file')} would have been added and\n"
                    + f"- {iunit(self.num_upd, 'file')} would have been updated.",
                )

        if self.dmg_list:
            eprint(CLI_ALERT_FG, "chkbit detected damage in these files:")
            for err in self.dmg_list:
                print(err, file=sys.stderr)
            n = len(self.dmg_list)
            status = f"error: detected {iunit(n, 'file')} with damage!"
            self.log.error(status)
            eprint(CLI_ALERT_FG, status)

        if self.err_list:
            status = "chkbit ran into errors"
            self.log.error(status + "!")
            eprint(CLI_ALERT_FG, status + ":")
            for err in self.err_list:
                print(err, file=sys.stderr)

        if self.dmg_list or self.err_list:
            sys.exit(1)

    def run(self):
        parser = argparse.ArgumentParser(
            prog="chkbit",
            description="Checks the data integrity of your files. See https://github.com/laktak/chkbit-py",
            epilog=EPILOG,
            formatter_class=argparse.RawDescriptionHelpFormatter,
        )

        parser.add_argument(
            "paths", metavar="PATH", type=str, nargs="*", help="directories to check"
        )

        parser.add_argument(
            "-u",
            "--update",
            action="store_true",
            help="update indices (without this chkbit will verify files in readonly mode)",
        )

        parser.add_argument(
            "--show-ignored-only", action="store_true", help="only show ignored files"
        )

        parser.add_argument(
            "--algo",
            type=str,
            default="blake3",
            help="hash algorithm: md5, sha512, blake3 (default: blake3)",
        )

        parser.add_argument(
            "-f", "--force", action="store_true", help="force update of damaged items"
        )

        parser.add_argument(
            "-s", "--skip-symlinks", action="store_true", help="do not follow symlinks"
        )

        parser.add_argument(
            "-l",
            "--log-file",
            metavar="FILE",
            type=str,
            help="write to a logfile if specified",
        )

        parser.add_argument(
            "--log-verbose", action="store_true", help="verbose logging"
        )

        parser.add_argument(
            "--index-name",
            metavar="NAME",
            type=str,
            default=".chkbit",
            help="filename where chkbit stores its hashes, needs to start with '.' (default: .chkbit)",
        )
        parser.add_argument(
            "--ignore-name",
            metavar="NAME",
            type=str,
            default=".chkbitignore",
            help="filename that chkbit reads its ignore list from, needs to start with '.' (default: .chkbitignore)",
        )

        parser.add_argument(
            "-w",
            "--workers",
            metavar="N",
            action="store",
            type=int,
            default=5,
            help="number of workers to use (default: 5)",
        )

        parser.add_argument(
            "--plain",
            action="store_true",
            help="show plain status instead of being fancy",
        )

        parser.add_argument(
            "-q",
            "--quiet",
            action="store_true",
            help="quiet, don't show progress/information",
        )

        parser.add_argument(
            "-v", "--verbose", action="store_true", help="verbose output"
        )

        parser.add_argument(
            "-V", "--version", action="store_true", help="show version information"
        )

        args = parser.parse_args()

        if args.version:
            print(__version__)
            return

        self.verbose = args.verbose or args.show_ignored_only
        if args.log_file:
            self.log_verbose = args.log_verbose
            self.log.setLevel(logging.INFO)
            fh = logging.FileHandler(args.log_file)
            fh.setFormatter(
                logging.Formatter(
                    "%(asctime)s %(levelname).4s %(message)s",
                    datefmt="%Y-%m-%d %H:%M:%S",
                )
            )
            self.log.addHandler(fh)

        if args.quiet:
            self.progress = Progress.Quiet
        elif not sys.stdout.isatty():
            self.progress = Progress.Summary
        elif args.plain:
            self.progress = Progress.Plain

        if args.paths:
            self.log.info(f"chkbit {', '.join(args.paths)}")
            context = self.process(args)
            if context and not context.show_ignored_only:
                self.print_result(context)
        else:
            parser.print_help()


def main():
    try:
        Main().run()

        print(
            "\nNotice: you are using an obsolete version of this tool, there will be no further upgrades via pip!"
        )
        print("To upgrade go to https://github.com/laktak/chkbit")
    except KeyboardInterrupt:
        print("abort")
        sys.exit(1)
    except Exception as e:
        print(e, file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
