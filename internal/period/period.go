// Package period models the day / week / month views.
package period

import (
	"fmt"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
)

// Kind is the length of a period.
type Kind int

const (
	Day Kind = iota
	Week
	Month
)

// String returns the Czech name of the kind.
func (k Kind) String() string {
	switch k {
	case Week:
		return "Týden"
	case Month:
		return "Měsíc"
	default:
		return "Den"
	}
}

// Next returns the following kind, cycling Day -> Week -> Month -> Day.
func (k Kind) Next() Kind { return (k + 1) % 3 }

// Period is a normalized date range of a given kind.
type Period struct {
	Kind  Kind
	Start civil.Date
}

// Of returns the period of kind k containing d. Weeks start on Monday (ISO 8601).
func Of(k Kind, d civil.Date) Period {
	switch k {
	case Week:
		offset := (int(d.Weekday()) + 6) % 7 // Monday = 0
		return Period{Week, d.AddDays(-offset)}
	case Month:
		return Period{Month, civil.Date{Year: d.Year, Month: d.Month, Day: 1}}
	default:
		return Period{Day, d}
	}
}

// End returns the last day of the period (inclusive).
func (p Period) End() civil.Date {
	switch p.Kind {
	case Week:
		return p.Start.AddDays(6)
	case Month:
		return civil.Of(p.Start.Time().AddDate(0, 1, -1))
	default:
		return p.Start
	}
}

// Next returns the following period of the same kind.
func (p Period) Next() Period { return Of(p.Kind, p.End().AddDays(1)) }

// Prev returns the preceding period of the same kind.
func (p Period) Prev() Period { return Of(p.Kind, p.Start.AddDays(-1)) }

// Contains reports whether d falls into the period.
func (p Period) Contains(d civil.Date) bool {
	return !d.Before(p.Start) && !d.After(p.End())
}

// Days returns every day of the period in order.
func (p Period) Days() []civil.Date {
	var days []civil.Date
	for d := p.Start; !d.After(p.End()); d = d.AddDays(1) {
		days = append(days, d)
	}
	return days
}

// Label returns a Czech description, e.g. "po 12. 10. 2026",
// "týden 42 · 12.–18. 10. 2026" or "říjen 2026".
func (p Period) Label() string {
	switch p.Kind {
	case Week:
		_, w := p.Start.Time().ISOWeek()
		return fmt.Sprintf("týden %d · %s", w, RangeLabel(p.Start, p.End()))
	case Month:
		return fmt.Sprintf("%s %d", MonthName(p.Start.Month), p.Start.Year)
	default:
		return WeekdayShort(p.Start.Weekday()) + " " + p.Start.Czech()
	}
}

// RangeLabel formats a date range compactly: "12.–18. 10. 2026",
// "28. 9.–4. 10. 2026" or "29. 12. 2025–4. 1. 2026".
func RangeLabel(from, to civil.Date) string {
	switch {
	case from.Year != to.Year:
		return from.Czech() + "–" + to.Czech()
	case from.Month != to.Month:
		return fmt.Sprintf("%d. %d.–%s", from.Day, from.Month, to.Czech())
	default:
		return fmt.Sprintf("%d.–%s", from.Day, to.Czech())
	}
}

var monthNames = [...]string{"leden", "únor", "březen", "duben", "květen", "červen",
	"červenec", "srpen", "září", "říjen", "listopad", "prosinec"}

// MonthName returns the Czech month name in nominative, e.g. "říjen".
func MonthName(m time.Month) string { return monthNames[m-1] }

var weekdayShort = [...]string{"ne", "po", "út", "st", "čt", "pá", "so"}

// WeekdayShort returns the Czech two-letter weekday abbreviation, e.g. "po".
func WeekdayShort(d time.Weekday) string { return weekdayShort[d] }
