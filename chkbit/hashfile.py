import hashlib
from typing import Callable


BLOCKSIZE = 2**10 * 128  # kb


def hashfile(path: str, hash_algo: str, *, hit: Callable[[str], None]):
    if hash_algo == "md5":
        h = hashlib.md5()
    elif hash_algo == "sha512":
        h = hashlib.sha512()
    elif hash_algo == "blake3":
        from blake3 import blake3

        h = blake3()
    else:
        raise Exception(f"algo '{hash_algo}' is unknown.")

    with open(path, "rb") as f:
        while True:
            buf = f.read(BLOCKSIZE)
            l = len(buf)
            if l <= 0:
                break
            h.update(buf)
            if hit:
                hit(l)
    return h.hexdigest()


def hashtext(text: str):
    md5 = hashlib.md5()
    md5.update(text.encode("utf-8"))
    return md5.hexdigest()
