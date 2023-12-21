import queue
from chkbit import Status


class Context:
    def __init__(
        self,
        *,
        num_workers=5,
        force=False,
        update=False,
        hash_algo="blake3",
        skip_symlinks=False,
        index_filename=".chkbit",
        ignore_filename=".chkbitignore",
    ):
        self.num_workers = num_workers
        self.force = force
        self.update = update
        self.hash_algo = hash_algo
        self.skip_symlinks = skip_symlinks
        self.index_filename = index_filename
        self.ignore_filename = ignore_filename

        self.result_queue = queue.Queue()
        self.hit_queue = queue.Queue()

        if hash_algo not in ["md5", "sha512", "blake3"]:
            raise Exception(f"{hash_algo} is unknown.")

    def log(self, stat: Status, path: str):
        self.result_queue.put((0, stat, path))

    def hit(self, *, cfiles: int = 0, cbytes: int = 0):
        self.result_queue.put((1, cfiles, cbytes))
