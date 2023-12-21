from enum import Enum


class Status(Enum):
    ERR_DMG = "DMG"
    ERR_IDX = "EIX"
    WARN_OLD = "old"
    NEW = "new"
    UPDATE = "upd"
    OK = "ok "
    SKIP = "skp"
    INTERNALEXCEPTION = "EXC"
    UPDATE_INDEX = "iup"
