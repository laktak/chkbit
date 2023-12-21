import os
import sys
import time
import threading
from . import Index, Status


class IndexThread:
    def __init__(self, thread_no, context, input_queue):
        self.thread_no = thread_no
        self.update = context.update
        self.context = context
        self.input_queue = input_queue
        self.t = threading.Thread(target=self._run)
        self.t.daemon = True
        self.t.start()

    def _process_root(self, parent):
        files = []
        dirs = []

        # load files and subdirs
        for name in os.listdir(path=parent):
            path = os.path.join(parent, name)
            if name[0] == ".":
                continue
            if os.path.isdir(path):
                if self.context.skip_symlinks and os.path.islink(path):
                    pass
                else:
                    dirs.append(name)
            elif os.path.isfile(path):
                files.append(name)

        # load index
        index = Index(self.context, parent, files)
        index.load()

        # calc the new hashes
        index.update()

        # compare
        index.check_fix(self.context.force)

        # save if update is set
        if self.update:
            if index.save():
                self.context.log(Status.UPDATE_INDEX, "")

        # process subdirs
        for name in dirs:
            if not index.should_ignore(name):
                self.input_queue.put(os.path.join(parent, name))
            else:
                self.context.log(Status.SKIP, name + "/")

    def _run(self):
        while True:
            parent = self.input_queue.get()
            if parent is None:
                break
            try:
                self._process_root(parent)
            except Exception as e:
                self.context.log(Status.INTERNALEXCEPTION, f"{parent}: {e}")
            self.input_queue.task_done()

    def join(self):
        self.t.join()
