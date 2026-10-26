package configedit

import (
	"slices"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
)

// Usage counts entries per project code and activity.
type Usage struct {
	Projects   map[string]int
	Activities map[string]int
}

// CountUsage counts how many entries use each code.
func CountUsage(entries []entry.Entry) Usage {
	u := Usage{Projects: map[string]int{}, Activities: map[string]int{}}
	for _, e := range entries {
		u.Projects[e.ProjectCode]++
		u.Activities[e.Activity]++
	}
	return u
}

// ReposOf returns the repos patterns that offer a project.
func ReposOf(cfg *config.Config, code string) []string {
	var out []string
	for _, pat := range sortedKeys(cfg.Repos) {
		if slices.Contains(cfg.Repos[pat], code) {
			out = append(out, pat)
		}
	}
	return out
}

// ProjectsOf returns the projects that explicitly allow an activity.
func ProjectsOf(cfg *config.Config, activity string) []string {
	var out []string
	for _, code := range cfg.ProjectCodes() {
		if slices.Contains(cfg.Projects[code].Activities, activity) {
			out = append(out, code)
		}
	}
	return out
}

// OnlyRepoOf returns the repos patterns that would lose their last project
// when code is removed from them.
func OnlyRepoOf(cfg *config.Config, code string) []string {
	var out []string
	for _, pat := range ReposOf(cfg, code) {
		if len(cfg.Repos[pat]) == 1 {
			out = append(out, pat)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
