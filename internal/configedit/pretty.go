package configedit

import (
	"regexp"
	"strings"
	"unicode/utf8"
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

	blank := blankBefore(source)
	spaced := make([]string, 0, len(lines)+16)
	verbatim := blockScalars(lines)
	for i, l := range lines {
		if i == 0 || verbatim[i] || l == "" || spaced[len(spaced)-1] == "" {
			spaced = append(spaced, l)
			continue
		}
		prev := lines[i-1]
		switch {
		case blank[normalize(l)]:
			// An empty line the source had before this line (also inside a section).
			spaced = append(spaced, "")
		case !startsIndented(l) && !strings.HasPrefix(l, "---") &&
			strings.TrimSpace(prev) != "" && (startsIndented(prev) || !strings.HasPrefix(prev, "#")):
			// A top-level line after the previous section's content (or its
			// indented trailing comment) starts a new section.
			spaced = append(spaced, "")
		}
		spaced = append(spaced, l)
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

// normalize compares lines regardless of alignment spaces.
func normalize(l string) string { return strings.Join(strings.Fields(l), " ") }

// blankBefore lists the source lines that follow an empty line.
func blankBefore(source []byte) map[string]bool {
	lines := strings.Split(string(source), "\n")
	out := map[string]bool{}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i-1]) == "" && strings.TrimSpace(lines[i]) != "" {
			out[normalize(lines[i])] = true
		}
	}
	return out
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
