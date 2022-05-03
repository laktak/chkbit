import hashlib


BLOCKSIZE = 2 ** 10 * 128  # kb


def hashfile(path, hash_algo=None):

    if not hash_algo or hash_algo == "md5":
        h = hashlib.md5()
    elif hash_algo == "sha512":
        h = hashlib.sha512()
    else:
        raise Exception(f"{hash_algo} is unknown.")

    with open(path, "rb") as f:
        while True:
            buf = f.read(BLOCKSIZE)
            if len(buf) <= 0:
                break
            h.update(buf)
    return h.hexdigest()


def hashtext(text):
    md5 = hashlib.md5()
    md5.update(text.encode("utf-8"))
    return md5.hexdigest()
