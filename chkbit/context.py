from __future__ import annotations
import queue
import chkbit
from typing import Optional
from chkbit import InputItem


class Context:
    def __init__(
        self,
        *,
        num_workers=5,
        force=False,
        update=False,
        show_ignored_only=False,
        hash_algo="blake3",
        skip_symlinks=False,
        index_filename=".chkbit",
        ignore_filename=".chkbitignore",
    ):
        self.num_workers = num_workers
        self.force = force
        self.update = update
        self.show_ignored_only = show_ignored_only
        self.hash_algo = hash_algo
        self.skip_symlinks = skip_symlinks
        self.index_filename = index_filename
        self.ignore_filename = ignore_filename

        # the input queue is used to distribute the work
        # to the index threads
        self.input_queue = queue.Queue()

        self.result_queue = queue.Queue()
        self.hit_queue = queue.Queue()

        if hash_algo not in ["md5", "sha512", "blake3"]:
            raise Exception(f"{hash_algo} is unknown.")

    def log(self, stat: chkbit.Status, path: str):
        self.result_queue.put((0, stat, path))

    def hit(self, *, cfiles: int = 0, cbytes: int = 0):
        self.result_queue.put((1, cfiles, cbytes))

    def add_input(self, path: str, *, ignore: Optional[chkbit.Ignore] = None):
        self.input_queue.put(InputItem(path, ignore=ignore))

    def end_input(self):
        self.input_queue.put(None)
