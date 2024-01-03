from __future__ import annotations
import fnmatch
import os
import subprocess
import sys
import json
import chkbit
from chkbit import hashfile, hashtext, Status
from typing import Optional

VERSION = 2  # index version


class Index:
    def __init__(
        self,
        context: chkbit.Context,
        path: str,
        files: list[str],
        *,
        readonly: bool = False,
    ):
        self.context = context
        self.path = path
        self.files = files
        self.old = {}
        self.new = {}
        self.updates = []
        self.modified = None
        self.readonly = readonly

    @property
    def index_filepath(self):
        return os.path.join(self.path, self.context.index_filename)

    def _setmod(self, value=True):
        self.modified = value

    def _log(self, stat: Status, name: str):
        self.context.log(stat, os.path.join(self.path, name))

    # calc new hashes for this index
    def calc_hashes(self, *, ignore: Optional[chkbit.Ignore] = None):
        for name in self.files:
            if ignore and ignore.should_ignore(name):
                self._log(Status.IGNORE, name)
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
            else:
                if self.readonly:
                    self.new[name] = self._list_file(name, a)
                else:
                    self.new[name] = self._calc_file(name, a)

    def show_ignored_only(self, ignore: chkbit.Ignore):
        for name in self.files:
            if ignore.should_ignore(name):
                self._log(Status.IGNORE, name)

    # check/update the index (old vs new)
    def check_fix(self, force: bool):
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

    def _list_file(self, name: str, a: str):
        # produce a dummy entry for new files when the index is not updated
        return {
            "mod": None,
            "a": a,
            "h": None,
        }

    def _calc_file(self, name: str, a: str):
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
            if self.readonly:
                raise Exception("Error trying to save a readonly index.")

            data = {"v": VERSION, "idx": self.new}
            text = json.dumps(self.new, separators=(",", ":"))
            data["idx_hash"] = hashtext(text)

            with open(self.index_filepath, "w", encoding="utf-8") as f:
                json.dump(data, f, separators=(",", ":"))
            self._setmod(False)
            return True
        else:
            return False

    def load(self):
        if not os.path.exists(self.index_filepath):
            return False
        self._setmod(False)
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
                    self._setmod()
                    self._log(Status.ERR_IDX, self.index_filepath)
        return True
