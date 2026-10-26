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

// JoinArgs writes a command as one line, quoting arguments that are empty
// or contain spaces or quotes, so SplitArgs gives them back unchanged.
func JoinArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'\\") {
			a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// SplitArgs splits a command line into arguments like a shell does for
// spaces and quotes: "a b" and 'a b' are one argument, "" is an empty one,
// and \" or \\ escape inside double quotes.
func SplitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		case c == '"' || c == '\'':
			inArg = true
			j := i + 1
			for ; j < len(s) && s[j] != c; j++ {
				if c == '"' && s[j] == '\\' && j+1 < len(s) && (s[j+1] == '"' || s[j+1] == '\\') {
					j++
				}
				cur.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, fmt.Errorf("neukončené uvozovky %c", c)
			}
			i = j
		default:
			inArg = true
			cur.WriteByte(c)
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
