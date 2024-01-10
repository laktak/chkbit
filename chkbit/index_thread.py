from __future__ import annotations
import os
import sys
import time
import threading
import chkbit
from chkbit import Index, Status, Ignore


class IndexThread:
    def __init__(self, thread_no: int, context: chkbit.Context):
        self.thread_no = thread_no
        self.update = context.update
        self.context = context
        self.input_queue = context.input_queue
        self.t = threading.Thread(target=self._run)
        self.t.daemon = True
        self.t.start()

    def _process_root(self, iitem: chkbit.InputItem):
        files = []
        dirs = []

        # load files and subdirs
        for name in os.listdir(path=iitem.path):
            path = os.path.join(iitem.path, name)
            if name[0] == ".":
                if self.context.show_ignored_only and not self.context.is_chkbit_file(
                    name
                ):
                    self.context.log(Status.IGNORE, path)
                continue
            if os.path.isdir(path):
                if self.context.skip_symlinks and os.path.islink(path):
                    pass
                else:
                    dirs.append(name)
            elif os.path.isfile(path):
                files.append(name)

        # load index
        index = Index(self.context, iitem.path, files, readonly=not self.update)
        index.load()

        # load ignore
        ignore = Ignore(self.context, iitem.path, parent_ignore=iitem.ignore)

        if self.context.show_ignored_only:
            index.show_ignored_only(ignore)
        else:
            # calc the new hashes
            index.calc_hashes(ignore=ignore)

            # compare
            index.check_fix(self.context.force)

            # save if update is set
            if self.update:
                if index.save():
                    self.context.log(Status.UPDATE_INDEX, "")

        # process subdirs
        for name in dirs:
            if not ignore.should_ignore(name):
                self.context.add_input(os.path.join(iitem.path, name), ignore=ignore)
            else:
                self.context.log(Status.IGNORE, name + "/")

    def _run(self):
        while True:
            iitem = self.input_queue.get()
            if iitem is None:
                break
            try:
                self._process_root(iitem)
            except Exception as e:
                self.context.log(Status.INTERNALEXCEPTION, f"{iitem.path}: {e}")
            self.input_queue.task_done()

    def join(self):
        self.t.join()
