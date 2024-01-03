from __future__ import annotations
from typing import Optional
import chkbit


class InputItem:
    def __init__(self, path: str, *, ignore: Optional[chkbit.Ignore] = None):
        self.path = path
        self.ignore = ignore
