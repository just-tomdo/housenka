package gitlaburl

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// RepoRef is a GitLab repository (or a whole group) entered by the user.
type RepoRef struct {
	Host     string // lower-cased host, "" when only a path was entered
	Path     string // project path ("clients/acme-web") or group path ("interni") when Wildcard
	Wildcard bool   // the input was "group/*": every repository of the group
}

// Pattern returns the repos key: the path, or "path/*" for a whole group.
func (r RepoRef) Pattern() string {
	if r.Wildcard {
		return r.Path + "/*"
	}
	return r.Path
}

// Patterns returns the repos keys that cover the repository, most specific
// first: the exact path and every enclosing group as "group/*".
func (r RepoRef) Patterns() []string {
	segments := strings.Split(r.Path, "/")
	var out []string
	if !r.Wildcard {
		out = append(out, r.Path)
	}
	last := len(segments) - 1
	if r.Wildcard {
		last = len(segments)
	}
	for i := last; i >= 1; i-- {
		out = append(out, strings.Join(segments[:i], "/")+"/*")
	}
	return out
}

// ParseRepo accepts a repository the way people copy it: a web link
// (https://gitlab.example.com/clients/acme-web, also with /-/issues/… or
// .git), an SSH clone address (git@gitlab.example.com:clients/acme-web.git),
// a plain path (clients/acme-web) or a whole group (interni/*). Copied
// without https://, a first segment that is one of knownHosts is the host;
// any other first segment is a group, even with dots in its name.
func ParseRepo(raw string, knownHosts []string) (RepoRef, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return RepoRef{}, errors.New("zadej odkaz na repozitář nebo cestu skupina/projekt")
	}
	var ref RepoRef
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil {
			return RepoRef{}, fmt.Errorf("neplatná URL: %v", err)
		}
		if u.Host == "" {
			return RepoRef{}, errors.New("v odkazu chybí doména")
		}
		ref.Host, s = strings.ToLower(u.Hostname()), u.Path
	case strings.HasPrefix(s, "git@"):
		host, path, ok := strings.Cut(strings.TrimPrefix(s, "git@"), ":")
		if !ok || host == "" {
			return RepoRef{}, errors.New("nerozpoznaná SSH adresa (očekáváno git@doména:skupina/projekt.git)")
		}
		ref.Host, s = strings.ToLower(host), path
	default:
		first, rest, _ := strings.Cut(strings.TrimPrefix(s, "/"), "/")
		if slices.ContainsFunc(knownHosts, func(h string) bool { return strings.EqualFold(h, first) }) {
			ref.Host, s = strings.ToLower(first), rest
		}
	}
	s, _, _ = strings.Cut(s, "/-/")
	s, _, _ = strings.Cut(s, "?")
	s, _, _ = strings.Cut(s, "#")
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	if p, ok := strings.CutSuffix(s, "/*"); ok {
		s, ref.Wildcard = p, true
	}
	segments := strings.Split(s, "/")
	if s == "" || hasEmpty(segments) || strings.Contains(s, "*") || strings.ContainsAny(s, " \t") {
		return RepoRef{}, fmt.Errorf("neplatná cesta %q – zadej např. skupina/projekt nebo skupina/*", strings.TrimSpace(raw))
	}
	if !ref.Wildcard && len(segments) < 2 {
		return RepoRef{}, fmt.Errorf("%q je jen skupina – zadej i projekt (skupina/projekt), nebo celou skupinu jako %s/*", s, s)
	}
	ref.Path = s
	return ref, nil
}
