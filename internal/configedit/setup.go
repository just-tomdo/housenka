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
		{"activities", "Činnosti", !maps.EqualFunc(cfg.Activities, example.Activities, sameActivity)},
		{"projects", "Kódy projektů", !maps.EqualFunc(cfg.Projects, example.Projects, sameProject)},
		{"repos", "Repozitáře", !maps.EqualFunc(cfg.Repos, example.Repos, slices.Equal[[]string])},
	}
}

// SetupDone reports whether every step is done.
func SetupDone(steps []Step) bool {
	return !slices.ContainsFunc(steps, func(s Step) bool { return !s.Done })
}

func sameActivity(a, b config.Activity) bool {
	return a.Name == b.Name && a.Short == b.Short && a.IsProductive() == b.IsProductive()
}

func sameProject(a, b config.Project) bool {
	return a.Name == b.Name && slices.Equal(a.Activities, b.Activities) && a.DefaultActivity == b.DefaultActivity &&
		a.IssueOptional() == b.IssueOptional() && a.Color == b.Color
}

// Leftovers are the example items that can go: still exactly as in the
// example, without entries, and not used by anything the user added.
type Leftovers struct {
	Repos, Projects, Activities []string
}

// Empty reports whether there is nothing to remove.
func (l Leftovers) Empty() bool {
	return len(l.Repos)+len(l.Projects)+len(l.Activities) == 0
}

// ExampleLeftovers finds the unchanged and unused example items of cfg.
func ExampleLeftovers(cfg, example *config.Config, u Usage) Leftovers {
	var l Leftovers
	gone := map[string]bool{} // projects being removed
	for _, code := range cfg.ProjectCodes() {
		ex, ok := example.Projects[code]
		if !ok || !sameProject(cfg.Projects[code], ex) || u.Projects[code] > 0 {
			continue
		}
		// A project the user put into an own repos rule stays.
		used := false
		for pat, codes := range cfg.Repos {
			exCodes, isExample := example.Repos[pat]
			if slices.Contains(codes, code) && (!isExample || !slices.Equal(codes, exCodes)) {
				used = true
			}
		}
		if !used {
			gone[code] = true
			l.Projects = append(l.Projects, code)
		}
	}
	for _, pat := range sortedKeys(cfg.Repos) {
		codes := cfg.Repos[pat]
		if ex, ok := example.Repos[pat]; ok && slices.Equal(codes, ex) &&
			!slices.ContainsFunc(codes, func(c string) bool { return !gone[c] }) {
			l.Repos = append(l.Repos, pat)
		}
	}
	for _, code := range cfg.ActivityCodes() {
		ex, ok := example.Activities[code]
		if !ok || !sameActivity(cfg.Activities[code], ex) || u.Activities[code] > 0 {
			continue
		}
		used := false
		for pc, p := range cfg.Projects {
			if !gone[pc] && (slices.Contains(p.Activities, code) || p.DefaultActivity == code) {
				used = true
			}
		}
		if !used {
			l.Activities = append(l.Activities, code)
		}
	}
	return l
}

// DeleteLeftovers removes the items found by ExampleLeftovers.
func (d *Doc) DeleteLeftovers(l Leftovers) error {
	for _, pat := range l.Repos {
		if err := d.DeleteRepo(pat); err != nil {
			return err
		}
	}
	for _, code := range l.Projects {
		if _, err := d.DeleteProject(code); err != nil {
			return err
		}
	}
	for _, code := range l.Activities {
		if err := d.DeleteActivity(code); err != nil {
			return err
		}
	}
	return nil
}
