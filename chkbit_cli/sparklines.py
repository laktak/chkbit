import math, os, re, string, sys

"""
Copyright (c) 2021, Brandon Whaley <redkrieg@gmail.com>, et al.
All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
"""


spark_chars = "▁▂▃▄▅▆▇█"
"""Eight unicode characters of (nearly) steadily increasing height."""


def sparkify(series, minimum=None, maximum=None, rows=1):
    """Converts <series> to a sparkline string.

    Example:
    >>> sparkify([ 0.5, 1.2, 3.5, 7.3, 8.0, 12.5, float("nan"), 15.0, 14.2, 11.8, 6.1,
    ... 1.9 ])
    u'▁▁▂▄▅▇ ██▆▄▂'

    >>> sparkify([1, 1, -2, 3, -5, 8, -13])
    u'▆▆▅▆▄█▁'

    Raises ValueError if input data cannot be converted to float.
    Raises TypeError if series is not an iterable.
    """
    series = [float(n) for n in series]
    if all(not math.isfinite(n) for n in series):
        return " " * len(series)

    minimum = min(filter(math.isfinite, series)) if minimum is None else minimum
    maximum = max(filter(math.isfinite, series)) if maximum is None else maximum
    data_range = maximum - minimum
    if data_range == 0.0:
        # Graph a baseline if every input value is equal.
        return "".join([spark_chars[0] if math.isfinite(i) else " " for i in series])
    row_res = len(spark_chars)
    resolution = row_res * rows
    coefficient = (resolution - 1.0) / data_range

    def clamp(n):
        return min(max(n, minimum), maximum)

    def spark_index(n):
        """An integer from 0 to (resolution-1) proportional to the data range"""
        return int(round((clamp(n) - minimum) * coefficient))

    output = []
    for r in range(rows - 1, -1, -1):
        row_out = []
        row_min = row_res * r
        row_max = row_min + row_res - 1
        for n in series:
            if not math.isfinite(n):
                row_out.append(" ")
                continue
            i = spark_index(n)
            if i < row_min:
                row_out.append(" ")
            elif i > row_max:
                row_out.append(spark_chars[-1])
            else:
                row_out.append(spark_chars[i % row_res])
        output.append("".join(row_out))
    return os.linesep.join(output)
