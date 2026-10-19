// Package rules decides which combinations of issue, project code and
// activity are allowed, and validates entries against the configuration.
package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
)

// Rules answers questions about allowed combinations. Create it with New.
type Rules struct {
	cfg       *config.Config
	hosts     []string
	exact     map[string]repoRule // lower-cased project path -> rule
	wildcards []wildcard          // sorted by prefix length, longest first
}

type repoRule struct {
	pattern string // as written in the config
	codes   []string
}

type wildcard struct {
	prefix string // lower-cased, with trailing "/"
	repoRule
}

// New builds Rules from a validated configuration.
func New(cfg *config.Config) *Rules {
	r := &Rules{cfg: cfg, exact: map[string]repoRule{}}
	for _, h := range cfg.GitLabHosts {
		r.hosts = append(r.hosts, strings.ToLower(h))
	}
	for pattern, codes := range cfg.Repos {
		p := strings.ToLower(pattern)
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			r.wildcards = append(r.wildcards, wildcard{prefix, repoRule{pattern, codes}})
		} else {
			r.exact[p] = repoRule{pattern, codes}
		}
	}
	slices.SortFunc(r.wildcards, func(a, b wildcard) int { return len(b.prefix) - len(a.prefix) })
	return r
}

// Config returns the underlying configuration.
func (r *Rules) Config() *config.Config { return r.cfg }

// ParseIssue parses a GitLab link and checks that it points to a configured host.
func (r *Rules) ParseIssue(raw string) (gitlaburl.Ref, error) {
	ref, err := gitlaburl.Parse(raw)
	if err != nil {
		return ref, err
	}
	if !slices.Contains(r.hosts, ref.Host) {
		return ref, fmt.Errorf("doména %s není v configu (gitlab_hosts: %s)", ref.Host, strings.Join(r.cfg.GitLabHosts, ", "))
	}
	return ref, nil
}

// ProjectsForRepo returns project codes allowed for a GitLab project path.
// An exact path match wins over the longest matching "group/*" pattern.
func (r *Rules) ProjectsForRepo(projectPath string) ([]string, error) {
	if _, codes, ok := r.MatchRepo(projectPath); ok {
		return codes, nil
	}
	return nil, fmt.Errorf("repozitář %s nemá v configu přiřazené kódy projektů – doplň ho do sekce repos, např.:\n  %s: [KOD01]", projectPath, projectPath)
}

// MatchRepo returns the repos pattern (as written in the config) that
// applies to a project path, and its project codes.
func (r *Rules) MatchRepo(projectPath string) (pattern string, codes []string, ok bool) {
	p := strings.ToLower(projectPath)
	if m, ok := r.exact[p]; ok {
		return m.pattern, m.codes, true
	}
	for _, w := range r.wildcards {
		if strings.HasPrefix(p, w.prefix) {
			return w.pattern, w.codes, true
		}
	}
	return "", nil, false
}

// ProjectsWithoutIssue returns project codes that may be reported without an issue link.
func (r *Rules) ProjectsWithoutIssue() []string {
	var codes []string
	for _, code := range r.cfg.ProjectCodes() {
		if r.cfg.Projects[code].IssueOptional() {
			codes = append(codes, code)
		}
	}
	return codes
}

// AllowedProjects returns project codes allowed for an issue link. An empty
// link yields projects with issue: optional. The parsed link is returned
// when issueURL is not empty and valid.
func (r *Rules) AllowedProjects(issueURL string) ([]string, *gitlaburl.Ref, error) {
	if strings.TrimSpace(issueURL) == "" {
		codes := r.ProjectsWithoutIssue()
		if len(codes) == 0 {
			return nil, nil, fmt.Errorf("odkaz na issue je povinný (žádný projekt nemá issue: optional)")
		}
		return codes, nil, nil
	}
	ref, err := r.ParseIssue(issueURL)
	if err != nil {
		return nil, nil, err
	}
	codes, err := r.ProjectsForRepo(ref.ProjectPath)
	if err != nil {
		return nil, &ref, err
	}
	return codes, &ref, nil
}

// AllowedActivities returns activity codes allowed for a project: its own
// list in configured order, or all activities when the project has none.
func (r *Rules) AllowedActivities(projectCode string) []string {
	p, ok := r.cfg.Projects[projectCode]
	if !ok {
		return nil
	}
	if len(p.Activities) > 0 {
		return p.Activities
	}
	return r.cfg.ActivityCodes()
}

// DefaultActivity returns the activity to preselect for a project: the
// configured default, or the only allowed activity, or "".
func (r *Rules) DefaultActivity(projectCode string) string {
	p, ok := r.cfg.Projects[projectCode]
	if !ok {
		return ""
	}
	if p.DefaultActivity != "" {
		return p.DefaultActivity
	}
	if allowed := r.AllowedActivities(projectCode); len(allowed) == 1 {
		return allowed[0]
	}
	return ""
}
