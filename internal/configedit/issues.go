package configedit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/issues"
)

// Operations on issues.yaml (a Doc from ParseIssues or LoadIssues).

// IssuesConfig decodes and validates the edited issues.yaml.
func (d *Doc) IssuesConfig() (*issues.Config, error) {
	data, err := d.Bytes()
	if err != nil {
		return nil, err
	}
	return issues.ParseConfig(data)
}

// SetIssueSettings sets the general keys; an empty host is removed (the
// first of gitlab_hosts is used then).
func (d *Doc) SetIssueSettings(host string, syncMinutes, staleDays int) {
	m := d.top()
	if host = strings.ToLower(strings.TrimSpace(host)); host == "" {
		del(m, "host")
	} else {
		set(m, "host", scalar(host))
	}
	set(m, "sync_minutes", integer(syncMinutes))
	set(m, "stale_days", integer(staleDays))
}

// statusNodes returns the statuses sequence, creating it when missing.
func (d *Doc) statusNodes() *yaml.Node {
	if v, _ := lookup(d.top(), "statuses"); v != nil && v.Kind == yaml.SequenceNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.SequenceNode}
	set(d.top(), "statuses", v)
	return v
}

func statusIndex(seq *yaml.Node, code string) int {
	return slices.IndexFunc(seq.Content, func(n *yaml.Node) bool {
		v, _ := lookup(n, "code")
		return v != nil && v.Value == code
	})
}

// SetStatus creates (old == "", at the end) or updates the status old,
// renaming its code. The caller moves issues of a renamed status
// (issues.File.RenameStatus).
func (d *Doc) SetStatus(old string, s issues.Status) error {
	s.Code = strings.TrimSpace(s.Code)
	if s.Code == "" {
		return errors.New("chybí kód stavu")
	}
	seq := d.statusNodes()
	i := statusIndex(seq, old)
	if old != "" && i < 0 {
		return fmt.Errorf("stav %s neexistuje", old)
	}
	if s.Code != old && statusIndex(seq, s.Code) >= 0 {
		return fmt.Errorf("stav %s už existuje", s.Code)
	}
	if s.Color != "" && !config.ValidColor(s.Color) {
		return errors.New(config.ColorProblem(s.Color))
	}
	var node *yaml.Node
	if i < 0 {
		node = &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		seq.Content = append(seq.Content, node)
	} else {
		node = seq.Content[i]
	}
	set(node, "code", scalar(s.Code))
	if name := strings.TrimSpace(s.Name); name == "" {
		del(node, "name")
	} else {
		setAfter(node, "name", "code", scalar(name))
	}
	if s.Color == "" {
		del(node, "color")
	} else {
		set(node, "color", scalar(s.Color))
	}
	if s.Watched() {
		del(node, "watch")
	} else {
		set(node, "watch", boolean(false))
	}
	return nil
}

// DeleteStatus removes a status. The caller returns its issues among the
// new ones (issues.File.DropStatus).
func (d *Doc) DeleteStatus(code string) error {
	seq := d.statusNodes()
	i := statusIndex(seq, code)
	if i < 0 {
		return fmt.Errorf("stav %s neexistuje", code)
	}
	if len(seq.Content) == 1 {
		return errors.New("nástěnka potřebuje aspoň jeden stav")
	}
	if hc := seq.Content[i].HeadComment; hc != "" && i+1 < len(seq.Content) {
		seq.Content[i+1].HeadComment = joinComments(hc, seq.Content[i+1].HeadComment)
	}
	seq.Content = slices.Delete(seq.Content, i, i+1)
	return nil
}

// MoveStatus shifts a status (a board column) by delta places.
func (d *Doc) MoveStatus(code string, delta int) error {
	seq := d.statusNodes()
	i := statusIndex(seq, code)
	if i < 0 {
		return fmt.Errorf("stav %s neexistuje", code)
	}
	j := min(max(i+delta, 0), len(seq.Content)-1)
	if i == j {
		return nil
	}
	n := seq.Content[i]
	seq.Content = slices.Insert(slices.Delete(seq.Content, i, i+1), j, n)
	return nil
}
