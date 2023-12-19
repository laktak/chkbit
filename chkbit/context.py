class Context:
    def __init__(self, verify_index, update, force, hash_algo, skip_symlinks):
        self.verify_index = verify_index
        self.update = update
        self.force = force
        self.hash_algo = hash_algo
        self.skip_symlinks = skip_symlinks

        if hash_algo not in ["md5", "sha512", "blake3"]:
            raise Exception(f"{hash_algo} is unknown.")
