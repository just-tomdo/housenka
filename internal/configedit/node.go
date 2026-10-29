package configedit

import (
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/hours"
)

// top returns the top-level mapping.
func (d *Doc) top() *yaml.Node { return d.root.Content[0] }

// lookup returns the value of key in mapping m and the index of its key node.
func lookup(m *yaml.Node, key string) (*yaml.Node, int) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1], i
		}
	}
	return nil, -1
}

// set replaces the value of key in m, keeping the key's comments, or
// appends the key at the end of m.
func set(m *yaml.Node, key string, val *yaml.Node) {
	if old, i := lookup(m, key); i >= 0 {
		val.LineComment = orStr(val.LineComment, old.LineComment)
		val.HeadComment = orStr(val.HeadComment, old.HeadComment)
		val.FootComment = orStr(val.FootComment, old.FootComment)
		m.Content[i+1] = val
		return
	}
	m.Content = append(m.Content, scalar(key), val)
}

// setAfter is set, but a new key goes right after the key after (or last).
func setAfter(m *yaml.Node, key, after string, val *yaml.Node) {
	_, i := lookup(m, after)
	if _, j := lookup(m, key); j >= 0 || i < 0 {
		set(m, key, val)
		return
	}
	m.Content = slices.Insert(m.Content, i+2, scalar(key), val)
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// del removes key from m without losing comments around it: its head
// comment moves to the next key, and a trailing comment block (which
// yaml.v3 keeps on the key as a foot comment, like a commented-out example
// at the end of a section) moves to the previous key.
func del(m *yaml.Node, key string) {
	_, i := lookup(m, key)
	if i < 0 {
		return
	}
	k, v := m.Content[i], m.Content[i+1]
	if hc := k.HeadComment; hc != "" {
		if i+2 < len(m.Content) {
			m.Content[i+2].HeadComment = joinComments(hc, m.Content[i+2].HeadComment)
		} else if i > 0 {
			m.Content[i-2].FootComment = joinComments(m.Content[i-2].FootComment, hc)
		}
	}
	if fc := joinComments(k.FootComment, v.FootComment); fc != "" {
		switch {
		case i > 0:
			m.Content[i-2].FootComment = joinComments(m.Content[i-2].FootComment, fc)
		case i+2 < len(m.Content):
			m.Content[i+2].HeadComment = joinComments(fc, m.Content[i+2].HeadComment)
		default:
			m.FootComment = joinComments(m.FootComment, fc)
		}
	}
	m.Content = slices.Delete(m.Content, i, i+2)
}

func joinComments(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "\n" + b
}

// rename changes key old to new in place.
func rename(m *yaml.Node, old, new string) {
	if _, i := lookup(m, old); i >= 0 {
		m.Content[i].Value = new
	}
}

// section returns the mapping under key in m, creating it when missing.
func section(m *yaml.Node, key string) *yaml.Node {
	if v, _ := lookup(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	set(m, key, v)
	return v
}

// keys returns the keys of mapping m in order.
func keys(m *yaml.Node) []string {
	var out []string
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, m.Content[i].Value)
	}
	return out
}

// strs returns the scalar items of a sequence node.
func strs(n *yaml.Node) []string {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, c := range n.Content {
		out = append(out, c.Value)
	}
	return out
}

func scalar(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if strings.HasPrefix(s, "#") {
		n.Style = yaml.DoubleQuotedStyle // like the docs: color: "#e06c75"
	}
	return n
}

func boolean(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}

func integer(n int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
}

// hoursNode writes quarter-hours as people do: 8, 7.5, 7.25.
func hoursNode(q int) *yaml.Node {
	if q%hours.PerHour == 0 {
		return integer(q / hours.PerHour)
	}
	v := strings.TrimRight(hours.FormatSep(q, "."), "0")
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: v}
}

// list is a flow sequence: [A, B].
func list(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, it := range items {
		n.Content = append(n.Content, scalar(it))
	}
	return n
}

// replaceInList renames or removes (new == "") item old in sequence n.
func replaceInList(n *yaml.Node, old, new string) {
	if n == nil || n.Kind != yaml.SequenceNode {
		return
	}
	n.Content = slices.DeleteFunc(n.Content, func(c *yaml.Node) bool {
		if c.Value != old {
			return false
		}
		c.Value = new
		return new == ""
	})
}
