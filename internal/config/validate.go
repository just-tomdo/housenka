package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/civil"
)

// Problems is a list of configuration errors, reported all at once.
type Problems []string

func (p Problems) Error() string {
	return "neplatný config:\n  - " + strings.Join(p, "\n  - ")
}

// Validate checks the configuration for consistency. It returns Problems
// listing every issue found, or nil.
func (c *Config) Validate() error {
	var p Problems
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if len(c.GitLabHosts) == 0 {
		add("gitlab_hosts: chybí alespoň jedna doména GitLabu")
	}
	for i, h := range c.GitLabHosts {
		if h == "" || strings.ContainsAny(h, "/: ") {
			add("gitlab_hosts[%d]: %q není doména (zadej jen např. gitlab.example.com)", i, h)
		}
	}

	if c.BM.BaseURL == "" {
		add("bm.base_url: chybí adresa editoru výkazu v Bílém motýlu")
	} else if u, err := url.Parse(c.BM.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		add("bm.base_url: %q není platná URL", c.BM.BaseURL)
	}
	seenField := map[string]string{}
	for _, kv := range c.BM.Fields.Ordered() {
		switch name := strings.TrimSpace(kv[1]); {
		case name == "":
			add("bm.fields.%s: chybí název pole formuláře", kv[0])
		case seenField[name] != "":
			add("bm.fields.%s: název %q už používá bm.fields.%s", kv[0], name, seenField[name])
		default:
			seenField[name] = kv[0]
		}
	}

	if !slices.Contains(Themes, c.UI.Theme) {
		add("ui.theme: neznámé téma %q (možnosti: %s)", c.UI.Theme, strings.Join(Themes, ", "))
	}

	if _, err := civil.ParseLayout(c.UI.DateFormat); err != nil {
		add("ui.date_format: %q – %v", c.UI.DateFormat, err)
	}

	for key, v := range map[string]Hours{
		"time.daily_goal_hours":    c.Time.DailyGoal,
		"time.max_entry_hours":     c.Time.MaxEntry,
		"time.daily_warning_hours": c.Time.DailyWarning,
	} {
		if v <= 0 {
			add("%s: musí být větší než 0", key)
		}
	}

	if len(c.Activities) == 0 {
		add("activities: chybí alespoň jedna činnost")
	}
	for _, code := range c.ActivityCodes() {
		a := c.Activities[code]
		if strings.TrimSpace(code) == "" {
			add("activities: prázdný kód činnosti")
		}
		if strings.TrimSpace(a.Name) == "" {
			add("activities.%s.name: chybí název", code)
		}
		if a.Productive == nil {
			add("activities.%s.productive: chybí (true = produktivní, false = neproduktivní)", code)
		}
	}

	if len(c.Projects) == 0 {
		add("projects: chybí alespoň jeden kód projektu")
	}
	for _, code := range c.ProjectCodes() {
		pr := c.Projects[code]
		if strings.TrimSpace(code) == "" {
			add("projects: prázdný kód projektu")
		}
		if strings.TrimSpace(pr.Name) == "" {
			add("projects.%s.name: chybí název", code)
		}
		seen := map[string]bool{}
		for _, a := range pr.Activities {
			if _, ok := c.Activities[a]; !ok {
				add("projects.%s.activities: činnost %q není v sekci activities", code, a)
			}
			if seen[a] {
				add("projects.%s.activities: činnost %q je uvedená vícekrát", code, a)
			}
			seen[a] = true
		}
		if d := pr.DefaultActivity; d != "" {
			if _, ok := c.Activities[d]; !ok {
				add("projects.%s.default_activity: činnost %q není v sekci activities", code, d)
			} else if len(pr.Activities) > 0 && !seen[d] {
				add("projects.%s.default_activity: činnost %q není mezi povolenými činnostmi projektu", code, d)
			}
		}
		switch pr.Issue {
		case "", IssueRequired, IssueOptional:
		default:
			add("projects.%s.issue: neplatná hodnota %q (required nebo optional)", code, pr.Issue)
		}
	}

	normalized := map[string]string{}
	for _, pattern := range sortedKeys(c.Repos) {
		if err := checkRepoPattern(pattern); err != "" {
			add("repos.%s: %s", pattern, err)
		}
		norm := strings.ToLower(pattern)
		if other, ok := normalized[norm]; ok {
			add("repos.%s: duplicitní s %q (na velikosti písmen nezáleží)", pattern, other)
		}
		normalized[norm] = pattern

		codes := c.Repos[pattern]
		if len(codes) == 0 {
			add("repos.%s: chybí alespoň jeden kód projektu", pattern)
		}
		seen := map[string]bool{}
		for _, code := range codes {
			if _, ok := c.Projects[code]; !ok {
				add("repos.%s: kód projektu %q není v sekci projects", pattern, code)
			}
			if seen[code] {
				add("repos.%s: kód projektu %q je uvedený vícekrát", pattern, code)
			}
			seen[code] = true
		}
	}

	if len(p) == 0 {
		return nil
	}
	return p
}

// Warnings returns non-fatal findings about a valid configuration.
func (c *Config) Warnings() []string {
	var w []string
	used := map[string]bool{}
	for _, codes := range c.Repos {
		for _, code := range codes {
			used[code] = true
		}
	}
	for _, code := range c.ProjectCodes() {
		if !used[code] && !c.Projects[code].IssueOptional() {
			w = append(w, fmt.Sprintf("projects.%s: kód nejde vybrat – není u žádného repozitáře v repos a nemá issue: optional", code))
		}
	}
	return w
}

// checkRepoPattern validates a repos key: either an exact project path
// ("clients/beta-app") or a group wildcard ("interni/*"). It returns an error message or "".
func checkRepoPattern(pattern string) string {
	path, wildcard := strings.CutSuffix(pattern, "/*")
	if path == "" {
		return "prázdná cesta"
	}
	if strings.Contains(path, "*") {
		return "hvězdička je povolená jen na konci jako „/*“ (celá skupina)"
	}
	segments := strings.Split(path, "/")
	if slices.Contains(segments, "") {
		return "cesta nesmí začínat/končit lomítkem ani obsahovat „//“"
	}
	if !wildcard && len(segments) < 2 {
		return "přesná cesta musí obsahovat skupinu i repozitář (např. clients/beta-app); pro celou skupinu použij „skupina/*“"
	}
	return ""
}
