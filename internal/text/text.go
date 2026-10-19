// Package text holds small text helpers shared by the command line, the
// TUI and the web interface.
package text

import (
	"fmt"
	"strings"
)

// Plural returns n with the Czech form of a noun: 1 → one, 2–4 → few,
// otherwise many ("1 výkaz", "3 výkazy", "5 výkazů").
func Plural(n int, one, few, many string) string {
	form := many
	switch {
	case n == 1:
		form = one
	case n >= 2 && n <= 4:
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}

// OneLine joins a multi-line message into one line.
func OneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
