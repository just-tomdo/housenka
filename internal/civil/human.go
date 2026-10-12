package civil

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

var errHuman = errors.New("neplatné datum – zadej např. 12. 10. 2026, 12.10., dnes, včera nebo -1")

// ParseHuman parses dates as typed by people, relative to today:
// "12. 10. 2026", "12.10.2026", "12.10.", "12." (this month), "2026-10-12",
// "dnes", "včera", "-1", "+2".
func ParseHuman(s string, today Date) (Date, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "":
		return Date{}, errors.New("datum není vyplněné")
	case "dnes", "d":
		return today, nil
	case "včera", "vcera", "v":
		return today.AddDays(-1), nil
	case "předevčírem", "predevcirem":
		return today.AddDays(-2), nil
	}
	if s[0] == '-' || s[0] == '+' {
		n, err := strconv.Atoi(s)
		if err != nil {
			return Date{}, errHuman
		}
		return today.AddDays(n), nil
	}
	if strings.Contains(s, "-") {
		d, err := Parse(s)
		if err != nil {
			return Date{}, errHuman
		}
		return d, nil
	}

	parts := strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, " ", ""), "."), ".")
	if len(parts) > 3 {
		return Date{}, errHuman
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Date{}, errHuman
		}
		nums[i] = n
	}
	d := Date{Year: today.Year, Month: today.Month, Day: nums[0]}
	if len(nums) > 1 {
		d.Month = time.Month(nums[1])
	}
	if len(nums) > 2 {
		d.Year = nums[2]
		if d.Year < 100 {
			d.Year += 2000
		}
	}
	if !d.Valid() {
		return Date{}, errHuman
	}
	return d, nil
}
