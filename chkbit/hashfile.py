import hashlib


BLOCKSIZE = 2 ** 10 * 128  # kb


def hashfile(path):
    md5 = hashlib.md5()
    with open(path, "rb") as f:
        while True:
            buf = f.read(BLOCKSIZE)
            if len(buf) <= 0:
                break
            md5.update(buf)
    return md5.hexdigest()


def hashtext(text):
    md5 = hashlib.md5()
    md5.update(text.encode("utf-8"))
    return md5.hexdigest()
