// Package hours converts between human-entered durations and quarter-hours.
//
// Durations are stored as an integer number of quarter-hours so that no
// floating-point arithmetic is ever involved (2.75 h == 11 quarters).
package hours

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PerHour is the number of quarter-hours in one hour.
const PerHour = 4

var errFormat = errors.New("neplatný čas – zadej např. 2.75, 2,75 nebo 2:45")

// Parse parses a duration and returns it in quarter-hours.
//
// Accepted forms: "2", "2.75", "2,75", ".5", "2:45". The value must be
// a non-negative multiple of a quarter-hour.
func Parse(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("čas není vyplněný")
	}
	if strings.HasPrefix(s, "-") {
		return 0, errors.New("čas nesmí být záporný")
	}
	if h, m, ok := strings.Cut(s, ":"); ok {
		return parseClock(h, m)
	}
	return parseDecimal(strings.Replace(s, ",", ".", 1))
}

func parseClock(h, m string) (int, error) {
	if h == "" || len(m) != 2 {
		return 0, errFormat
	}
	hh, err := parseDigits(h)
	if err != nil {
		return 0, errFormat
	}
	mm, err := parseDigits(m)
	if err != nil || mm > 59 {
		return 0, errFormat
	}
	if mm%15 != 0 {
		return 0, fmt.Errorf("čas %s:%s není po čtvrthodinách (minuty 00, 15, 30, 45)", h, m)
	}
	return hh*PerHour + mm/15, nil
}

func parseDecimal(s string) (int, error) {
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" && frac == "" {
		return 0, errFormat
	}
	whole := 0
	if intPart != "" {
		var err error
		if whole, err = parseDigits(intPart); err != nil {
			return 0, errFormat
		}
	}
	// Normalise the fraction to hundredths: "5" -> 50, "750" -> 75.
	frac = strings.TrimRight(frac, "0")
	if len(frac) > 2 {
		return 0, fmt.Errorf("čas %s není po čtvrthodinách (.00, .25, .50, .75)", s)
	}
	hundredths := 0
	if frac != "" {
		var err error
		if hundredths, err = parseDigits(frac + strings.Repeat("0", 2-len(frac))); err != nil {
			return 0, errFormat
		}
	}
	if hundredths%25 != 0 {
		return 0, fmt.Errorf("čas %s není po čtvrthodinách (.00, .25, .50, .75)", s)
	}
	return whole*PerHour + hundredths/25, nil
}

// parseDigits accepts only ASCII digits (no signs, spaces or exponents).
func parseDigits(s string) (int, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, errFormat
	}
	return strconv.Atoi(s)
}

// Format renders quarter-hours with two decimals and a decimal comma, e.g. "2,75".
func Format(q int) string {
	return FormatSep(q, ",")
}

// FormatSep renders quarter-hours with two decimals and the given separator.
func FormatSep(q int, sep string) string {
	sign := ""
	if q < 0 {
		sign, q = "-", -q
	}
	return fmt.Sprintf("%s%d%s%02d", sign, q/PerHour, sep, q%PerHour*25)
}
