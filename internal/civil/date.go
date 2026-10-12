// Package civil provides a calendar date without time-of-day or time zone.
package civil

import (
	"fmt"
	"time"
)

const isoLayout = "2006-01-02"

// Date is a calendar date. The zero value is not a valid date.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Of returns the date of t in t's location.
func Of(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// Today returns the current local date.
func Today() Date {
	return Of(time.Now())
}

// Parse parses an ISO date ("2026-10-12").
func Parse(s string) (Date, error) {
	t, err := time.Parse(isoLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("neplatné datum %q (očekáváno RRRR-MM-DD)", s)
	}
	return Of(t), nil
}

// IsZero reports whether d is the zero value.
func (d Date) IsZero() bool {
	return d == Date{}
}

// Valid reports whether d is a real calendar date.
func (d Date) Valid() bool {
	return !d.IsZero() && Of(d.Time()) == d
}

// Time returns midnight UTC of d. UTC avoids DST surprises in day arithmetic.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// AddDays returns d shifted by n days.
func (d Date) AddDays(n int) Date {
	return Of(d.Time().AddDate(0, 0, n))
}

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday {
	return d.Time().Weekday()
}

// Compare returns -1, 0 or +1 when d is before, equal to or after o.
func (d Date) Compare(o Date) int {
	return d.Time().Compare(o.Time())
}

// Before reports whether d is before o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d is after o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// String returns the ISO form "2026-10-12".
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// Czech returns the Czech form without leading zeros, e.g. "9. 10. 2026".
func (d Date) Czech() string {
	return fmt.Sprintf("%d. %d. %d", d.Day, d.Month, d.Year)
}

// MarshalText implements encoding.TextMarshaler (used by JSON and YAML).
func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Date) UnmarshalText(b []byte) error {
	p, err := Parse(string(b))
	if err != nil {
		return err
	}
	*d = p
	return nil
}
