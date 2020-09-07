import os
import sys
import time
import argparse
import queue
import threading
from chkbit import IndexThread, Stat

STATUS_CODES = """
Status codes:
  ROT: error, bitrot detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  skp: skipped (see .chkbitignore)
  EXC: internal exception
"""


class Main:
    def __init__(self):
        self.stdscr = None
        self.bitrot_list = []
        self.err_list = []
        self.modified = False
        self.verbose = False
        self.total = 0
        self._parse_args()

    def _log(self, idx, stat, path):

        if stat == Stat.FLAG_MOD:
            self.modified = True
        else:
            if stat == Stat.ERR_BITROT:
                self.bitrot_list.append(path)
            elif stat == Stat.INTERNALEXCEPTION:
                self.err_list.append(path)
            elif stat in [Stat.OK, Stat.UPDATE, Stat.NEW]:
                self.total += 1
            if self.verbose or not stat in [Stat.OK, Stat.SKIP]:
                print(stat.value, path)
            if not self.quiet and sys.stdout.isatty():
                print(self.total, end="\r")

    def _parse_args(self):
        parser = argparse.ArgumentParser(
            description="Checks files for bitrot. See https://github.com/laktak/chkbit-py",
            epilog=STATUS_CODES,
            formatter_class=argparse.RawDescriptionHelpFormatter,
        )
        parser.add_argument("PATH", nargs="+")

        parser.add_argument(
            "-u",
            "--update",
            action="store_true",
            help="update indices (without this chkbit will only verify files)",
        )

        parser.add_argument(
            "-f", "--force", action="store_true", help="force update of damaged items"
        )

        # parser.add_argument(
        #     "-d", "--delete", action="store_true", help="remove all .chkbit files from target"
        # )

        parser.add_argument(
            "-q",
            "--quiet",
            action="store_true",
            help="quiet, don't show progress/information",
        )
        parser.add_argument(
            "-v", "--verbose", action="store_true", help="verbose output"
        )

        self.args = parser.parse_args()
        self.verbose = self.args.verbose
        self.quiet = self.args.quiet

    def _res_worker(self):
        while True:
            item = self.res_queue.get()
            if not item:
                break
            self._log(*item)
            self.res_queue.task_done()

    def process(self):

        self.res_queue = queue.Queue()
        todo_queue = queue.Queue()

        for path in self.args.PATH:
            todo_queue.put(path)

        workers = [
            IndexThread(idx, self.args, self.res_queue, todo_queue) for idx in range(5)
        ]

        res_worker = threading.Thread(target=self._res_worker)
        res_worker.daemon = True
        res_worker.start()

        todo_queue.join()
        self.res_queue.join()

    def print_result(self):
        if not self.quiet:
            print(
                f"Processed {self.total} file(s){' in readonly mode' if not self.args.update else ''}."
            )
            if self.modified:
                print("Indices were updated.")

        if self.bitrot_list:
            print("chkbit detected bitrot in these files:", file=sys.stderr)
            for err in self.bitrot_list:
                print(err, file=sys.stderr)
            print(
                f"error: detected {len(self.bitrot_list)} file(s) with bitrot!",
                file=sys.stderr,
            )
        if self.err_list:
            print("chkbit ran into errors:", file=sys.stderr)
            for err in self.err_list:
                print(err, file=sys.stderr)

        if self.bitrot_list or self.err_list:
            sys.exit(1)


def main():
    try:
        m = Main()
        m.process()
        m.print_result()
    except KeyboardInterrupt:
        print("abort")
        sys.exit(1)
