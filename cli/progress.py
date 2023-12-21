from enum import Enum


class Progress(Enum):
    Quiet = (0,)
    Summary = (1,)
    Plain = (2,)
    Fancy = (3,)
