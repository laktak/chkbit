import os
import sys
import time
import threading
from chkbit import Index, Stat


class IndexThread:
    def __init__(self, idx, args, res_queue, todo_queue):
        self.idx = idx
        self.update = args.update
        self.force = args.force
        self.todo_queue = todo_queue
        self.res_queue = res_queue
        self.t = threading.Thread(target=self.run)
        self.t.daemon = True
        self.t.start()

    def _log(self, stat, path):
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
        e.load()

        # update the index from current state
        e.update()

        # compare
        e.check_fix(self.force)

        # save if update is set
        if self.update:
            if e.save():
                self._log(Stat.FLAG_MOD, "")

        # process subdirs
        for name in dirs:
            if not e.should_ignore(name):
                self.todo_queue.put(os.path.join(parent, name))

    def run(self):
        while True:
            parent = self.todo_queue.get()
            if parent is None:
                break
            try:
                self._process_root(parent)
            except Exception as e:
                self._log(Stat.INTERNALEXCEPTION, e)
            self.todo_queue.task_done()
