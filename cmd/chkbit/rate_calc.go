package main

import (
	"time"
)

type RateCalc struct {
	Interval time.Duration
	MaxStat  int
	Start    time.Time
	Updated  time.Time
	Total    int64
	Current  int64
	Stats    []int64
}

func NewRateCalc(interval time.Duration, maxStat int) *RateCalc {
	if maxStat < 10 {
		maxStat = 10
	}
	rc := &RateCalc{
		Interval: interval,
		MaxStat:  maxStat,
	}
	rc.Reset()
	return rc
}

func (rc *RateCalc) Reset() {
	rc.Start = time.Now()
	rc.Updated = rc.Start
	rc.Total = 0
	rc.Current = 0
	rc.Stats = make([]int64, rc.MaxStat)
}

func (rc *RateCalc) Last() int64 {
	return rc.Stats[len(rc.Stats)-1]
}

func (rc *RateCalc) Push(ts time.Time, value int64) {
	for rc.Updated.Add(rc.Interval).Before(ts) {
		rc.Stats = append(rc.Stats, rc.Current)
		if len(rc.Stats) > rc.MaxStat {
			rc.Stats = rc.Stats[len(rc.Stats)-rc.MaxStat:]
		}
		rc.Total += rc.Current
		rc.Current = 0
		rc.Updated = rc.Updated.Add(rc.Interval)
	}
	rc.Current += value
}
