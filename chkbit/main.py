import os
import sys
import time
import argparse
import queue
import threading
from chkbit import Context, IndexThread, Stat

STATUS_CODES = """
Status codes:
  DMG: error, data damage detected
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
        self.dmg_list = []
        self.err_list = []
        self.modified = False
        self.verbose = False
        self.total = 0
        self._parse_args()

    def _log(self, idx, stat, path):

        if stat == Stat.FLAG_MOD:
            self.modified = True
        else:
            if stat == Stat.ERR_DMG:
                self.dmg_list.append(path)
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
            description="Checks the data integrity of your files. See https://github.com/laktak/chkbit-py",
            epilog=STATUS_CODES,
            formatter_class=argparse.RawDescriptionHelpFormatter,
        )

        parser.add_argument(
            "paths", metavar="PATH", type=str, nargs="*", help="directories to check"
        )

        parser.add_argument(
            "-u",
            "--update",
            action="store_true",
            help="update indices (without this chkbit will only verify files)",
        )

        parser.add_argument(
            "--algo",
            type=str,
            default="md5",
            help="hash algorithm: md5, sha512",
        )

        parser.add_argument(
            "-f", "--force", action="store_true", help="force update of damaged items"
        )

        parser.add_argument(
            "-i",
            "--verify-index",
            action="store_true",
            help="verify files in the index only (will not report new files)",
        )

        parser.add_argument(
            "-w",
            "--workers",
            metavar="N",
            action="store",
            type=int,
            default=5,
            help="number of workers to use, default=5",
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

        self.args = parser.parse_args()
        self.verbose = self.args.verbose
        self.quiet = self.args.quiet
        if not self.args.paths:
            parser.print_help()

    def _res_worker(self):
        while True:
            item = self.res_queue.get()
            if not item:
                break
            self._log(*item)
            self.res_queue.task_done()

    def process(self):

        self.res_queue = queue.Queue()

        # the todo queue is used to distribute the work
        # to the index threads
        todo_queue = queue.Queue()

        # put the initial paths into the queue
        for path in self.args.paths:
            todo_queue.put(path)

        context = Context(
            self.args.verify_index,
            self.args.update,
            self.args.force,
            self.args.algo,
        )

        # start indexing
        workers = [
            IndexThread(idx, context, self.res_queue, todo_queue)
            for idx in range(self.args.workers)
        ]

        # log the results from the workers
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

        if self.dmg_list:
            print("chkbit detected damage in these files:", file=sys.stderr)
            for err in self.dmg_list:
                print(err, file=sys.stderr)
            print(
                f"error: detected {len(self.dmg_list)} file(s) with damage!",
                file=sys.stderr,
            )
        if self.err_list:
            print("chkbit ran into errors:", file=sys.stderr)
            for err in self.err_list:
                print(err, file=sys.stderr)

        if self.dmg_list or self.err_list:
            sys.exit(1)


def main():
    try:
        m = Main()
        if m.args.paths:
            m.process()
            m.print_result()
    except KeyboardInterrupt:
        print("abort")
        sys.exit(1)
