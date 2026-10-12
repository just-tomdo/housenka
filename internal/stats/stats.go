// Package stats aggregates entries into totals and productive/unproductive ratios.
package stats

import (
	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/period"
)

// Totals are durations in quarter-hours.
type Totals struct {
	Total        int
	Productive   int
	Unproductive int
	Unknown      int // activities missing from the config
}

// Add accounts one entry.
func (t *Totals) Add(e entry.Entry, cfg *config.Config) {
	t.Total += e.Quarters
	a, ok := cfg.Activities[e.Activity]
	switch {
	case !ok:
		t.Unknown += e.Quarters
	case a.IsProductive():
		t.Productive += e.Quarters
	default:
		t.Unproductive += e.Quarters
	}
}

// ProductiveShare returns the productive fraction of the total, 0..1.
func (t Totals) ProductiveShare() float64 {
	if t.Total == 0 {
		return 0
	}
	return float64(t.Productive) / float64(t.Total)
}

// ProductivePercent returns the productive share rounded to whole percent.
func (t Totals) ProductivePercent() int {
	if t.Total == 0 {
		return 0
	}
	return (t.Productive*100 + t.Total/2) / t.Total
}

// Compute sums entries.
func Compute(entries []entry.Entry, cfg *config.Config) Totals {
	var t Totals
	for _, e := range entries {
		t.Add(e, cfg)
	}
	return t
}

// Day holds totals of one day.
type Day struct {
	Date civil.Date
	Totals
}

// Summary describes a whole period.
type Summary struct {
	Period period.Period
	Totals
	Days       []Day // every day of the period, including empty ones
	WorkedDays int   // days with at least one entry
}

// AvgHoursPerWorkedDay returns the mean hours per day with any work.
func (s Summary) AvgHoursPerWorkedDay() float64 {
	if s.WorkedDays == 0 {
		return 0
	}
	return float64(s.Total) / 4 / float64(s.WorkedDays)
}

// Summarize aggregates the entries falling into p, overall and per day.
func Summarize(p period.Period, entries []entry.Entry, cfg *config.Config) Summary {
	s := Summary{Period: p}
	index := map[civil.Date]int{}
	for i, d := range p.Days() {
		s.Days = append(s.Days, Day{Date: d})
		index[d] = i
	}
	for _, e := range entries {
		i, ok := index[e.Date]
		if !ok {
			continue
		}
		s.Totals.Add(e, cfg)
		s.Days[i].Add(e, cfg)
	}
	for _, d := range s.Days {
		if d.Total > 0 {
			s.WorkedDays++
		}
	}
	return s
}
