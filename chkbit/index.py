import fnmatch
import os
import subprocess
import sys
import json
from chkbit import hashfile, hashtext, Status

VERSION = 2  # index version


class Index:
    def __init__(self, context, path, files):
        self.context = context
        self.path = path
        self.files = files
        self.old = {}
        self.new = {}
        self.ignore = []
        self.load_ignore()
        self.updates = []
        self.modified = True

    @property
    def ignore_filepath(self):
        return os.path.join(self.path, self.context.ignore_filename)

    @property
    def index_filepath(self):
        return os.path.join(self.path, self.context.index_filename)

    def should_ignore(self, name):
        for ignore in self.ignore:
            if fnmatch.fnmatch(name, ignore):
                return True
        return False

    def _setmod(self):
        self.modified = True

    def _log(self, stat: Status, name: str):
        self.context.log(stat, os.path.join(self.path, name))

    # calc new hashes for this index
    def update(self):
        for name in self.files:
            if self.should_ignore(name):
                self._log(Status.SKIP, name)
                continue

            a = self.context.hash_algo
            # check previously used hash
            if name in self.old:
                old = self.old[name]
                if "md5" in old:
                    # legacy structure
                    a = "md5"
                    self.old[name] = {"mod": old["mod"], "a": a, "h": old["md5"]}
                elif "a" in old:
                    a = old["a"]
            self.new[name] = self._calc_file(name, a)

    # check/update the index (old vs new)
    def check_fix(self, force):
        for name in self.new.keys():
            if not name in self.old:
                self._log(Status.NEW, name)
                self._setmod()
                continue

            a = self.old[name]
            b = self.new[name]
            amod = a["mod"]
            bmod = b["mod"]
            if a["h"] == b["h"]:
                # ok, if the content stays the same the mod time does not matter
                self._log(Status.OK, name)
                if amod != bmod:
                    self._setmod()
                continue

            if amod == bmod:
                # damage detected
                self._log(Status.ERR_DMG, name)
                # replace with old so we don't loose the information on the next run
                # unless force is set
                if not force:
                    self.new[name] = a
                else:
                    self._setmod()
            elif amod < bmod:
                # ok, the file was updated
                self._log(Status.UPDATE, name)
                self._setmod()
            elif amod > bmod:
                self._log(Status.WARN_OLD, name)
                self._setmod()

    def _calc_file(self, name, a):
        path = os.path.join(self.path, name)
        info = os.stat(path)
        mtime = int(info.st_mtime * 1000)
        res = {
            "mod": mtime,
            "a": a,
            "h": hashfile(path, a, hit=lambda l: self.context.hit(cbytes=l)),
        }
        self.context.hit(cfiles=1)
        return res

    def save(self):
        if self.modified:
            data = {"v": VERSION, "idx": self.new}
            text = json.dumps(self.new, separators=(",", ":"))
            data["idx_hash"] = hashtext(text)

            with open(self.index_filepath, "w", encoding="utf-8") as f:
                json.dump(data, f, separators=(",", ":"))
            self.modified = False
            return True
        else:
            return False

    def load(self):
        if not os.path.exists(self.index_filepath):
            return False
        self.modified = False
        with open(self.index_filepath, "r", encoding="utf-8") as f:
            data = json.load(f)
            if "data" in data:
                # extract old format from js version
                for item in json.loads(data["data"]):
                    self.old[item["name"]] = {
                        "mod": item["mod"],
                        "a": "md5",
                        "h": item["md5"],
                    }
            elif "idx" in data:
                self.old = data["idx"]
                text = json.dumps(self.old, separators=(",", ":"))
                if data.get("idx_hash") != hashtext(text):
                    self.modified = True
                    self._log(Status.ERR_IDX, self.index_filepath)
        return True

    def load_ignore(self):
        if not os.path.exists(self.ignore_filepath):
            return
        with open(self.ignore_filepath, "r", encoding="utf-8") as f:
            text = f.read()

        self.ignore = list(
            filter(
                lambda x: x and x[0] != "#" and len(x.strip()) > 0, text.splitlines()
            )
        )
