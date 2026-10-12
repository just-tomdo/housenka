package civil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// DefaultLayout is the Czech form without leading zeros, e.g. "9. 10. 2026".
const DefaultLayout = "d. m. yyyy"

// Layout is a date format given by a pattern such as "d. m. yyyy",
// "dd.mm.yyyy" or "yyyy-mm-dd". Tokens (case-insensitive) are d / dd
// (day), m / mm (month) and yy / yyyy (year); everything else is literal.
type Layout struct {
	pattern string
	parts   []layoutPart
}

type layoutPart struct {
	field   byte // 'd', 'm', 'y', or 0 for a literal
	width   int  // digits of a field: 1 = no padding, 2, 4
	literal string
}

// ParseLayout parses a date format pattern.
func ParseLayout(pattern string) (Layout, error) {
	l := Layout{pattern: pattern}
	seen := map[byte]bool{}
	r := []rune(pattern)
	for i := 0; i < len(r); {
		c := unicode.ToLower(r[i])
		if !unicode.IsLetter(c) {
			j := i
			for j < len(r) && !unicode.IsLetter(r[j]) {
				j++
			}
			l.parts = append(l.parts, layoutPart{literal: string(r[i:j])})
			i = j
			continue
		}
		j := i
		for j < len(r) && unicode.ToLower(r[j]) == c {
			j++
		}
		n := j - i
		switch {
		case c != 'd' && c != 'm' && c != 'y':
			return Layout{}, fmt.Errorf("neznámý znak %q (povolené jsou d, m, y)", r[i])
		case c == 'y' && n != 2 && n != 4, c != 'y' && n > 2:
			return Layout{}, fmt.Errorf("%q není platný údaj (použij d/dd, m/mm, yy/yyyy)", string(r[i:j]))
		case seen[byte(c)]:
			return Layout{}, fmt.Errorf("%q je ve formátu dvakrát", string(c))
		}
		seen[byte(c)] = true
		l.parts = append(l.parts, layoutPart{field: byte(c), width: n})
		i = j
	}
	if !seen['d'] || !seen['m'] || !seen['y'] {
		return Layout{}, fmt.Errorf("formát musí obsahovat den, měsíc i rok (např. %s)", DefaultLayout)
	}
	return l, nil
}

// MustLayout is ParseLayout that panics; for constants and tests.
func MustLayout(pattern string) Layout {
	l, err := ParseLayout(pattern)
	if err != nil {
		panic(err)
	}
	return l
}

// String returns the pattern.
func (l Layout) String() string { return l.orDefault().pattern }

// orDefault makes the zero Layout behave as DefaultLayout.
func (l Layout) orDefault() Layout {
	if l.parts == nil {
		return MustLayout(DefaultLayout)
	}
	return l
}

// Format formats d.
func (l Layout) Format(d Date) string {
	l = l.orDefault()
	return l.format(d, len(l.parts))
}

// format renders the first n parts.
func (l Layout) format(d Date, n int) string {
	var b strings.Builder
	for _, p := range l.parts[:n] {
		v := 0
		switch p.field {
		case 0:
			b.WriteString(p.literal)
			continue
		case 'd':
			v = d.Day
		case 'm':
			v = int(d.Month)
		case 'y':
			v = d.Year
			if p.width == 2 {
				v %= 100
			}
		}
		fmt.Fprintf(&b, "%0*d", p.width, v)
	}
	return b.String()
}

// FormatRange formats a date range. Day-first layouts are shortened like
// "12.–18. 10. 2026" or "28. 9.–4. 10. 2026"; others are written in full.
func (l Layout) FormatRange(from, to Date) string {
	l = l.orDefault()
	var order []byte
	for _, p := range l.parts {
		if p.field != 0 {
			order = append(order, p.field)
		}
	}
	if string(order) != "dmy" || from.Year != to.Year {
		sep := "–"
		if string(order) != "dmy" {
			sep = " – " // "2026-10-12–2026-10-18" is hard to read
		}
		return l.Format(from) + sep + l.Format(to)
	}
	keep := 2 // fields of from: day and month
	if from.Month == to.Month {
		keep = 1
	}
	// Cut after the literal following the kept fields, without its
	// trailing spaces: "12. 10. 2026" → "12."
	n := 0
	for fields := 0; n < len(l.parts); n++ {
		if l.parts[n].field != 0 {
			if fields == keep {
				break
			}
			fields++
		}
	}
	return strings.TrimRight(l.format(from, n), " ") + "–" + l.Format(to)
}

// Parse parses s in the layout. Whitespace is ignored, numbers may be
// written without leading zeros.
func (l Layout) Parse(s string) (Date, bool) {
	l = l.orDefault()
	s = strings.Join(strings.Fields(s), "")
	var d Date
	for _, p := range l.parts {
		if p.field == 0 {
			lit := strings.Join(strings.Fields(p.literal), "")
			if !strings.HasPrefix(s, lit) {
				return Date{}, false
			}
			s = s[len(lit):]
			continue
		}
		n := 0
		for n < len(s) && n < max(p.width, 2) && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n == 0 {
			return Date{}, false
		}
		v, _ := strconv.Atoi(s[:n])
		s = s[n:]
		switch p.field {
		case 'd':
			d.Day = v
		case 'm':
			d.Month = time.Month(v)
		case 'y':
			if n <= 2 {
				v += 2000
			}
			d.Year = v
		}
	}
	if s != "" || !d.Valid() {
		return Date{}, false
	}
	return d, true
}
