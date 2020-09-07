import fnmatch
import os
import subprocess
import sys
import json
from enum import Enum
from chkbit import hashfile, hashtext

VERSION = 2  # index version
INDEX = ".chkbit"
IGNORE = ".chkbitignore"


class Stat(Enum):
    ERR_BITROT = "ROT"
    ERR_IDX = "EIX"
    WARN_OLD = "old"
    NEW = "new"
    UPDATE = "upd"
    OK = "ok "
    SKIP = "skp"
    INTERNALEXCEPTION = "EXC"
    FLAG_MOD = "fmod"


class Index:
    def __init__(self, path, files, *, log=None):
        self.path = path
        self.files = files
        self.old = {}
        self.new = {}
        self.ignore = []
        self.load_ignore()
        self.updates = []
        self.modified = True
        self.log = log

    @property
    def ignore_file(self):
        return os.path.join(self.path, IGNORE)

    @property
    def idx_file(self):
        return os.path.join(self.path, INDEX)

    def should_ignore(self, name):
        for ignore in self.ignore:
            if fnmatch.fnmatch(name, ignore):
                return True
        return False

    def _setmod(self):
        self.modified = True

    def _log(self, stat, name):
        if self.log:
            self.log(stat, os.path.join(self.path, name))

    def update(self):
        for name in self.files:
            if self.should_ignore(name):
                self._log(Stat.SKIP, name)
                continue
            self.new[name] = self._calc_file(name)

    def check_fix(self, force):
        for name in self.new.keys():
            if not name in self.old:
                self._log(Stat.NEW, name)
                self._setmod()
                continue

            a = self.old[name]
            b = self.new[name]
            amod = a["mod"]
            bmod = b["mod"]
            if a["md5"] == b["md5"]:
                # ok, if the content stays the same the mod time does not matter
                self._log(Stat.OK, name)
                if amod != bmod:
                    self._setmod()
                continue

            if amod == bmod:
                # rot detected
                self._log(Stat.ERR_BITROT, name)
                # replace with old so we don't loose the information on the next run
                # unless force is set
                if not force:
                    self.new[name] = a
                else:
                    self._setmod()
            elif amod < bmod:
                # ok, the file was updated
                self._log(Stat.UPDATE, name)
                self._setmod()
            elif amod > bmod:
                self._log(Stat.WARN_OLD, name)
                self._setmod()

    def _calc_file(self, name):
        path = os.path.join(self.path, name)
        info = os.stat(path)
        mtime = int(info.st_mtime * 1000)
        return {"mod": mtime, "md5": hashfile(path)}

    def save(self):
        if self.modified:
            data = {"v": VERSION, "idx": self.new}
            text = json.dumps(self.new, separators=(",", ":"))
            data["idx_hash"] = hashtext(text)

            with open(self.idx_file, "w", encoding="utf-8") as f:
                json.dump(data, f)
            self.modified = False
            return True
        else:
            return False

    def load(self):
        if not os.path.exists(self.idx_file):
            return
        self.modified = False
        with open(self.idx_file, "r", encoding="utf-8") as f:
            data = json.load(f)
            if "data" in data:
                # extract old format from js version
                for item in json.loads(data["data"]):
                    self.old[item["name"]] = {"mod": item["mod"], "md5": item["md5"]}
            elif "idx" in data:
                self.old = data["idx"]
                text = json.dumps(self.old, separators=(",", ":"))
                if data.get("idx_hash") != hashtext(text):
                    self.modified = True
                    self._log(Stat.ERR_IDX, self.idx_file)

    def load_ignore(self):
        if not os.path.exists(self.ignore_file):
            return
        with open(self.ignore_file, "r", encoding="utf-8") as f:
            text = f.read()

        self.ignore = list(
            filter(
                lambda x: x and x[0] != "#" and len(x.strip()) > 0, text.splitlines()
            )
        )
