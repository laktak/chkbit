import hashlib


class Context:
    def __init__(self, verify_index, update, force, hash_algo):

        self.verify_index = verify_index
        self.update = update
        self.force = force
        self.hash_algo = hash_algo

        if hash_algo not in ["md5", "sha512"]:
            raise Exception(f"{hash_algo} is unknown.")
