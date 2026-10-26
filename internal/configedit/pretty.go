package configedit

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// pretty restores the layout yaml.v3 drops, so the file stays pleasant to
// edit by hand: an empty line between top-level sections, trailing
// comments aligned within a block (at their original column when the line
// came from source), and rows of flow mappings
// (`DEV: { name: Vývoj, productive: true }`) aligned like a table.
func pretty(data, source []byte) []byte {
	crlf := strings.Contains(string(source), "\r\n")
	source = []byte(strings.ReplaceAll(string(source), "\r\n", "\n"))
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	lines = alignFlowMaps(lines, blockScalars(lines))
	lines = alignFlowSeqs(lines, blockScalars(lines), valueColumns(source))

	gaps := sourceGaps(source)
	spaced := make([]string, 0, len(lines)+16)
	verbatim := blockScalars(lines)
	prevLine := "" // the previous non-empty output line
	for i, l := range lines {
		if i == 0 || verbatim[i] || l == "" || spaced[len(spaced)-1] == "" {
			spaced = append(spaced, l)
			if l != "" {
				prevLine = l
			}
			continue
		}
		if gaps.before(prevLine, l, startsIndented(lines[i-1])) {
			spaced = append(spaced, "")
		}
		spaced = append(spaced, l)
		prevLine = l
	}
	end := "\n"
	if strings.HasSuffix(string(source), "\n\n") {
		end = "\n\n" // keep a trailing empty line
	}
	spaced = alignComments(spaced, blockScalars(spaced), commentColumns(source))
	// A line whose content did not change keeps its exact source layout.
	orig := map[string]string{}
	for _, l := range strings.Split(string(source), "\n") {
		if n := normalize(l); n != "" {
			if _, seen := orig[n]; !seen {
				orig[n] = strings.TrimRight(l, " \t")
			}
		}
	}
	// Blocks of sibling lines are restored only as a whole, so a changed or
	// added row still lines up with the rest of its table.
	verbatim = blockScalars(spaced)
	for i := 0; i < len(spaced); {
		j := i + 1
		for j < len(spaced) && spaced[j] != "" && indentOf(spaced[j]) == indentOf(spaced[i]) {
			j++
		}
		same := true
		for k := i; k < j && same; k++ {
			o, ok := orig[normalize(spaced[k])]
			same = ok && !verbatim[k] && indentOf(o) == indentOf(spaced[k])
		}
		if same {
			for k := i; k < j; k++ {
				if spaced[k] != "" {
					spaced[k] = orig[normalize(spaced[k])]
				}
			}
		}
		i = j
	}
	out := strings.Join(spaced, "\n") + end
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return []byte(out)
}

// normalize identifies a line by what it means, not how it is aligned:
// the YAML value of its code (so "a: { x: 1 }" equals "a:   {x: 1}", and
// "Foo,Bar" differs from "Foo, Bar") and the text of its comment.
func normalize(l string) string {
	if t := strings.TrimSpace(l); strings.HasPrefix(t, "#") {
		return t // a comment line is its text
	}
	code, comment := splitComment(strings.TrimSpace(l))
	code = strings.TrimSpace(code)
	comment = strings.TrimSpace(comment)
	if code == "" {
		return comment
	}
	var v any
	if err := yaml.Unmarshal([]byte(code), &v); err == nil {
		if out, err := yaml.Marshal(v); err == nil {
			return string(out) + "\x00" + comment
		}
	}
	return "\x01" + strings.Join(strings.Fields(code), " ") + "\x00" + comment
}

// gaps remembers where the source had empty lines, per occurrence: the
// n-th time a line (or a pair of neighbouring lines) comes, it gets what
// its n-th occurrence in the source had.
type gaps struct {
	pairs map[string][]bool // previous line + line → empty line between them
	lines map[string][]bool // line → empty line before it
}

func sourceGaps(source []byte) gaps {
	g := gaps{pairs: map[string][]bool{}, lines: map[string][]bool{}}
	prev, gap := "", false
	for _, l := range strings.Split(string(source), "\n") {
		if strings.TrimSpace(l) == "" {
			gap = prev != ""
			continue
		}
		key := normalize(prev) + "\x02" + normalize(l)
		g.pairs[key] = append(g.pairs[key], gap)
		g.lines[normalize(l)] = append(g.lines[normalize(l)], gap)
		prev, gap = l, false
	}
	return g
}

// take returns the next remembered value for key (the last one again
// when the source had fewer occurrences).
func take(m map[string][]bool, key string) (bool, bool) {
	q, ok := m[key]
	if !ok || len(q) == 0 {
		return false, false
	}
	if len(q) > 1 {
		m[key] = q[1:]
	}
	return q[0], true
}

// before decides whether an empty line goes before l, which follows prev:
// as in the source when the two lines were neighbours there; for a line
// whose neighbour changed, as before that line in the source; and for a
// new top-level line after a section's content, a section break.
func (g gaps) before(prev, l string, prevIndented bool) bool {
	if gap, ok := take(g.pairs, normalize(prev)+"\x02"+normalize(l)); ok {
		take(g.lines, normalize(l))
		return gap
	}
	if gap, ok := take(g.lines, normalize(l)); ok {
		return gap
	}
	return !startsIndented(l) && !strings.HasPrefix(l, "---") &&
		(prevIndented || !strings.HasPrefix(strings.TrimSpace(prev), "#"))
}

var flowSeqRe = regexp.MustCompile(`^(\s*)([^\s#\[\]][^\[\]]*?):(\s+)(\[.*\])$`)

// valueColumns maps "key:" rows with a flow sequence in the source to the
// column of the "[".
func valueColumns(source []byte) map[string]int {
	cols := map[string]int{}
	for _, l := range strings.Split(string(source), "\n") {
		code, _ := splitComment(l)
		if m := flowSeqRe.FindStringSubmatch(code); m != nil {
			cols[m[1]+m[2]] = width(m[1]) + width(m[2]) + 1 + width(m[3])
		}
	}
	return cols
}

// alignFlowSeqs keeps "key: [A, B]" rows aligned when they were aligned in
// the source (like a table of repos), also for rows added to the block.
func alignFlowSeqs(lines []string, verbatim []bool, known map[string]int) []string {
	type row struct {
		indent, key, value, comment string
	}
	for i := 0; i < len(lines); {
		var rows []row
		j := i
		for j < len(lines) && !verbatim[j] {
			code, c := splitComment(lines[j])
			m := flowSeqRe.FindStringSubmatch(code)
			if m == nil || len(rows) > 0 && m[1] != rows[0].indent {
				break
			}
			rows = append(rows, row{m[1], m[2], m[4], c})
			j++
		}
		col, votes := 0, map[int]int{}
		for _, r := range rows {
			if kc, ok := known[r.indent+r.key]; ok && kc > width(r.indent+r.key)+2 {
				votes[kc]++
			}
		}
		for c, n := range votes {
			if n > votes[col] || n == votes[col] && c < col {
				col = c
			}
		}
		if col > 0 {
			for k, r := range rows {
				l := pad(r.indent+r.key+":", col-1) + " " + r.value
				if r.comment != "" {
					l += " " + r.comment
				}
				lines[i+k] = l
			}
		}
		i = max(j, i+1)
	}
	return lines
}

// commentColumns maps trailing comments of the source to their column.
func commentColumns(source []byte) map[string]int {
	cols := map[string]int{}
	for _, l := range strings.Split(string(source), "\n") {
		if code, c := splitComment(l); c != "" && strings.TrimSpace(code) != "" {
			if _, seen := cols[c]; !seen {
				cols[c] = width(l) - width(c)
			}
		}
	}
	return cols
}

func startsIndented(l string) bool { return l[0] == ' ' || l[0] == '\t' }

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

var blockScalarRe = regexp.MustCompile(`:\s+[|>][-+0-9]*\s*(#.*)?$`)

// blockScalars marks the content lines of literal and folded scalars,
// which must not be touched.
func blockScalars(lines []string) []bool {
	v := make([]bool, len(lines))
	for i := 0; i < len(lines); i++ {
		if !blockScalarRe.MatchString(lines[i]) {
			continue
		}
		ind := indentOf(lines[i])
		for j := i + 1; j < len(lines) && (strings.TrimSpace(lines[j]) == "" || indentOf(lines[j]) > ind); j++ {
			v[j] = true
			i = j
		}
	}
	return v
}

// splitComment splits a line into code and trailing comment (with "#"),
// ignoring "#" inside quotes.
func splitComment(l string) (code, comment string) {
	var quote byte
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && i > 0 && (l[i-1] == ' ' || l[i-1] == '\t'):
			return strings.TrimRight(l[:i], " \t"), l[i:]
		}
	}
	return l, ""
}

func width(s string) int { return utf8.RuneCountInString(s) }

func pad(s string, w int) string { return s + strings.Repeat(" ", max(w-width(s), 0)) }

// maxCommentColumn: lines longer than this do not push the comment column
// of their block further right.
const maxCommentColumn = 40

// alignComments aligns trailing comments of consecutive lines. A comment
// known from the source keeps its column; others align with the block.
func alignComments(lines []string, verbatim []bool, known map[string]int) []string {
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && !verbatim[j] && strings.TrimSpace(lines[j]) != "" &&
			!strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
			if _, c := splitComment(lines[j]); c == "" {
				break
			}
			j++
		}
		// The block's column: the most common known one, else past the
		// longest line (ignoring very long ones).
		col, votes := 0, map[int]int{}
		for k := i; k < j; k++ {
			code, c := splitComment(lines[k])
			if kc, ok := known[c]; ok && kc >= width(code)+2 {
				votes[kc]++
			}
		}
		for c, n := range votes {
			if n > votes[col] || n == votes[col] && c < col {
				col = c
			}
		}
		if col == 0 {
			for k := i; k < j; k++ {
				if code, _ := splitComment(lines[k]); width(code) <= maxCommentColumn {
					col = max(col, width(code)+2)
				}
			}
		}
		for k := i; k < j; k++ {
			code, c := splitComment(lines[k])
			target := col
			if kc, ok := known[c]; ok && kc >= width(code)+2 {
				target = kc
			}
			lines[k] = pad(code, max(target, width(code)+2)) + c
		}
		i = max(j, i+1)
	}
	return lines
}

var flowMapRe = regexp.MustCompile(`^(\s*)([^\s#{}][^{}]*?):\s*\{(.*)\}$`)

type flowRow struct {
	indent, key string
	items       []string
	comment     string
}

func alignFlowMaps(lines []string, verbatim []bool) []string {
	parse := func(l string) (flowRow, bool) {
		code, c := splitComment(l)
		m := flowMapRe.FindStringSubmatch(code)
		if m == nil {
			return flowRow{}, false
		}
		return flowRow{m[1], m[2], splitItems(m[3]), c}, true
	}
	for i := 0; i < len(lines); {
		var rows []flowRow
		j := i
		for j < len(lines) && !verbatim[j] {
			r, ok := parse(lines[j])
			if !ok || len(rows) > 0 && r.indent != rows[0].indent {
				break
			}
			rows = append(rows, r)
			j++
		}
		if len(rows) > 0 {
			keyW := 0
			var colW []int
			for _, r := range rows {
				keyW = max(keyW, width(r.key)+1)
				for k, it := range r.items {
					if k == len(r.items)-1 {
						continue
					}
					if k >= len(colW) {
						colW = append(colW, 0)
					}
					colW[k] = max(colW[k], width(it)+1)
				}
			}
			for k, r := range rows {
				var b strings.Builder
				for n, it := range r.items {
					if n < len(r.items)-1 {
						b.WriteString(pad(it+",", colW[n]+1))
					} else {
						b.WriteString(it)
					}
				}
				lines[i+k] = r.indent + pad(r.key+":", keyW+1) + "{ " + b.String() + " }"
				if len(r.items) == 0 {
					lines[i+k] = r.indent + r.key + ": {}"
				}
				if r.comment != "" {
					lines[i+k] += "  " + r.comment
				}
			}
		}
		i = max(j, i+1)
	}
	return lines
}

// splitItems splits the inside of a flow mapping at top-level commas.
func splitItems(s string) []string {
	var items []string
	var quote byte
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			items = append(items, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		items = append(items, last)
	}
	return items
}
