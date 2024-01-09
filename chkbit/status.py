from __future__ import annotations
from enum import Enum
import logging


class Status(Enum):
    ERR_DMG = "DMG"
    ERR_IDX = "EIX"
    WARN_OLD = "old"
    NEW = "new"
    UPDATE = "upd"
    OK = "ok "
    IGNORE = "ign"
    INTERNALEXCEPTION = "EXC"
    UPDATE_INDEX = "iup"

    @staticmethod
    def get_level(status: Status):
        if status == Status.INTERNALEXCEPTION:
            return logging.CRITICAL
        elif status in [Status.ERR_DMG, Status.ERR_IDX]:
            return logging.ERROR
        if status == Status.WARN_OLD:
            return logging.WARNING
        elif status in [Status.NEW, Status.UPDATE, Status.OK, Status.IGNORE]:
            return logging.INFO
        else:
            return logging.DEBUG
