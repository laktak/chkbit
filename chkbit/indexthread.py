import os
import sys
import time
import threading
from chkbit import Index, Stat


class IndexThread:
    def __init__(self, idx, context, res_queue, todo_queue):
        self.idx = idx
        self.verify_index_only = context.verify_index
        self.update = context.update and not self.verify_index_only
        self.context = context
        self.todo_queue = todo_queue
        self.res_queue = res_queue
        self.t = threading.Thread(target=self.run)
        self.t.daemon = True
        self.t.start()

    def _log(self, stat, path):
        if not self.verify_index_only or stat != Stat.NEW:
            self.res_queue.put((self.idx, stat, path))

    def _process_root(self, parent):
        files = []
        dirs = []

        # load files and subdirs
        for name in os.listdir(path=parent):
            path = os.path.join(parent, name)
            if name[0] == ".":
                continue
            if os.path.isdir(path):
                dirs.append(name)
            elif os.path.isfile(path):
                files.append(name)

        # load index
        e = Index(parent, files, log=self._log)
        if e.load() or not self.verify_index_only:

            # calc the new hashes
            e.update(self.context)

            # compare
            e.check_fix(self.context.force)

            # save if update is set
            if self.update:
                if e.save():
                    self._log(Stat.FLAG_MOD, "")

        # process subdirs
        for name in dirs:
            if not e.should_ignore(name):
                self.todo_queue.put(os.path.join(parent, name))
            else:
                self._log(Stat.SKIP, name + "/")

    def run(self):
        while True:
            parent = self.todo_queue.get()
            if parent is None:
                break
            try:
                self._process_root(parent)
            except Exception as e:
                self._log(Stat.INTERNALEXCEPTION, f"{parent}: {e}")
            self.todo_queue.task_done()
