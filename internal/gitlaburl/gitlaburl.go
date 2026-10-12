// Package gitlaburl parses links to GitLab issues, work items and merge requests.
package gitlaburl

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Kind is the type of the linked GitLab object.
type Kind string

const (
	Issue        Kind = "issues"
	WorkItem     Kind = "work_items"
	MergeRequest Kind = "merge_requests"
)

// Ref is a parsed GitLab link.
type Ref struct {
	Raw         string // the link as entered (trimmed), including any #fragment
	Host        string // lower-cased host, e.g. "gitlab.example.com"
	ProjectPath string // full project path, e.g. "clients/acme-web"
	Group       string // namespace part, e.g. "clients" (may contain subgroups)
	Repo        string // last path segment, e.g. "acme-web"
	Kind        Kind
	IID         int // project-scoped number, e.g. 636
}

// Short returns a compact human-readable form: "clients / acme-web / #636".
func (r Ref) Short() string {
	prefix := "#"
	if r.Kind == MergeRequest {
		prefix = "!"
	}
	return fmt.Sprintf("%s / %s / %s%d", r.Group, r.Repo, prefix, r.IID)
}

// Parse parses a GitLab link such as
// https://gitlab.example.com/clients/acme-web/-/work_items/636#note_1.
func Parse(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, errors.New("odkaz není vyplněný")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Ref{}, fmt.Errorf("neplatná URL: %v", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return Ref{}, errors.New("odkaz musí začínat https://")
	}
	if u.Host == "" {
		return Ref{}, errors.New("v odkazu chybí doména")
	}

	path := strings.Trim(u.Path, "/")
	projectPath, rest, ok := strings.Cut(path, "/-/")
	if !ok {
		return Ref{}, errors.New("odkaz nevede na issue (chybí část „/-/issues/…“ nebo „/-/work_items/…“)")
	}
	segments := strings.Split(projectPath, "/")
	if len(segments) < 2 || hasEmpty(segments) {
		return Ref{}, fmt.Errorf("neplatná cesta projektu %q", projectPath)
	}

	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return Ref{}, fmt.Errorf("nerozpoznaná část odkazu %q (očekáváno např. issues/123)", rest)
	}
	kind := Kind(parts[0])
	switch kind {
	case Issue, WorkItem, MergeRequest:
	default:
		return Ref{}, fmt.Errorf("nepodporovaný typ odkazu %q (podporováno: issues, work_items, merge_requests)", parts[0])
	}
	iid, err := strconv.Atoi(parts[1])
	if err != nil || iid <= 0 || strings.TrimLeft(parts[1], "0123456789") != "" {
		return Ref{}, fmt.Errorf("neplatné číslo issue %q", parts[1])
	}

	return Ref{
		Raw:         raw,
		Host:        strings.ToLower(u.Hostname()),
		ProjectPath: projectPath,
		Group:       strings.Join(segments[:len(segments)-1], "/"),
		Repo:        segments[len(segments)-1],
		Kind:        kind,
		IID:         iid,
	}, nil
}

func hasEmpty(ss []string) bool {
	for _, s := range ss {
		if s == "" {
			return true
		}
	}
	return false
}
