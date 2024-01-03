from __future__ import annotations
import fnmatch
import os
import sys
import chkbit
from enum import Enum
from typing import Optional


class Ignore:
    def __init__(
        self,
        context: chkbit.Context,
        path: str,
        *,
        parent_ignore: Optional[chkbit.Ignore],
    ):
        self.parent_ignore = parent_ignore
        self.context = context
        self.path = path
        self.name = os.path.basename(path) + "/"
        self.ignore = []
        self.load_ignore()

    @property
    def ignore_filepath(self):
        return os.path.join(self.path, self.context.ignore_filename)

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

    def should_ignore(self, name: str, *, fullname: str = None):
        for ignore in self.ignore:
            if ignore.startswith("/"):
                if fullname:
                    continue
                else:
                    ignore = ignore[1:]
            if fnmatch.fnmatch(name, ignore):
                return True
            if fullname and fnmatch.fnmatch(fullname, ignore):
                return True
        if self.parent_ignore:
            return self.parent_ignore.should_ignore(
                fullname or name, fullname=self.name + (fullname or name)
            )
        return False
