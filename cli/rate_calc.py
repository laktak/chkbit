from datetime import datetime, timedelta


class RateCalc:
    def __init__(self, interval: timedelta, max_stat: int):
        self.interval = interval
        self.max_stat = max(max_stat, 10)
        self.reset()

    def reset(self):
        self.start = datetime.now()
        self.updated = self.start
        self.total = 0
        self.current = 0
        self.stats = [0] * self.max_stat

    @property
    def last(self):
        return self.stats[-1]

    def push(self, ts: datetime, value: int):
        while self.updated + self.interval < ts:
            self.stats.append(self.current)
            self.stats = self.stats[-self.max_stat :]
            self.total += self.current
            self.current = 0
            self.updated += self.interval
        self.current += value
