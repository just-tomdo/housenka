package configedit

import (
	"maps"
	"slices"

	"github.com/just-tomdo/housenka/internal/config"
)

// Step is one part of the first setup.
type Step struct {
	ID    string // "gitlab", "bm", "activities", "projects", "repos"
	Title string
	Done  bool // the part differs from the example configuration
}

// Setup compares cfg with the example configuration and tells which parts
// still hold the example values.
func Setup(cfg, example *config.Config) []Step {
	return []Step{
		{"gitlab", "Doména GitLabu", !slices.Equal(cfg.GitLabHosts, example.GitLabHosts)},
		{"bm", "Adresa a pole Bílého motýla", cfg.BM.BaseURL != example.BM.BaseURL},
		{"activities", "Činnosti", !maps.EqualFunc(cfg.Activities, example.Activities, func(a, b config.Activity) bool {
			return a.Name == b.Name && a.IsProductive() == b.IsProductive()
		})},
		{"projects", "Kódy projektů", !maps.EqualFunc(cfg.Projects, example.Projects, func(a, b config.Project) bool {
			return a.Name == b.Name && slices.Equal(a.Activities, b.Activities) && a.DefaultActivity == b.DefaultActivity &&
				a.Issue == b.Issue && a.Color == b.Color
		})},
		{"repos", "Repozitáře", !maps.EqualFunc(cfg.Repos, example.Repos, slices.Equal[[]string])},
	}
}

// SetupDone reports whether every step is done.
func SetupDone(steps []Step) bool {
	return !slices.ContainsFunc(steps, func(s Step) bool { return !s.Done })
}
