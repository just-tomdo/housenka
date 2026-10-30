package configedit

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/config"
)

var codeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

func checkCode(code, what string) error {
	if code == "" {
		return fmt.Errorf("chybí kód %s", what)
	}
	if !codeRe.MatchString(code) {
		return fmt.Errorf("kód %s %q smí obsahovat jen písmena bez diakritiky, číslice, tečku, pomlčku a podtržítko", what, code)
	}
	return nil
}

func hasKey(m *yaml.Node, key string) bool {
	_, i := lookup(m, key)
	return i >= 0
}

// --- activities ------------------------------------------------------------

// Activity is the editable part of an activity.
type Activity struct {
	Name       string
	Short      string // label shown instead of the code, "" = the code
	Productive bool
}

// SetActivity creates (old == "") or updates the activity old, renaming it
// to code. A new code is renamed in every project too.
func (d *Doc) SetActivity(old, code string, a Activity) error {
	code = strings.TrimSpace(code)
	if err := checkCode(code, "činnosti"); err != nil {
		return err
	}
	acts := section(d.top(), "activities")
	if old != "" && !hasKey(acts, old) {
		return fmt.Errorf("činnost %s neexistuje", old)
	}
	if code != old && hasKey(acts, code) {
		return fmt.Errorf("činnost %s už existuje", code)
	}
	if old != "" && old != code {
		rename(acts, old, code)
		for _, p := range d.projectNodes() {
			replaceInList(mustLookup(p, "activities"), old, code)
			if v, _ := lookup(p, "default_activity"); v != nil && v.Value == old {
				v.Value = code
			}
		}
	}
	node, _ := lookup(acts, code)
	if node == nil || node.Kind != yaml.MappingNode {
		node = &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		set(acts, code, node)
	}
	set(node, "name", scalar(strings.TrimSpace(a.Name)))
	if short := strings.TrimSpace(a.Short); short == "" {
		del(node, "short")
	} else {
		setAfter(node, "short", "name", scalar(short))
	}
	set(node, "productive", boolean(a.Productive))
	return nil
}

// DeleteActivity removes an activity and its use in projects. It refuses
// when the activity is the only one allowed for a project, because an
// empty list would allow every activity.
func (d *Doc) DeleteActivity(code string) error {
	acts := section(d.top(), "activities")
	if !hasKey(acts, code) {
		return fmt.Errorf("činnost %s neexistuje", code)
	}
	var only []string
	projects := section(d.top(), "projects")
	for _, pc := range keys(projects) {
		p, _ := lookup(projects, pc)
		if l := strs(mustLookup(p, "activities")); len(l) == 1 && l[0] == code {
			only = append(only, pc)
		}
	}
	if len(only) > 0 {
		return fmt.Errorf("činnost %s je jediná povolená u projektu %s – nejdřív u něj povol jinou činnost", code, strings.Join(only, ", "))
	}
	for _, p := range d.projectNodes() {
		replaceInList(mustLookup(p, "activities"), code, "")
		if v, _ := lookup(p, "default_activity"); v != nil && v.Value == code {
			del(p, "default_activity")
		}
	}
	del(acts, code)
	return nil
}

// --- projects ----------------------------------------------------------------

// Project is the editable part of a project code.
type Project struct {
	Name            string
	Color           string   // color name, "#rrggbb" or "" (derived from the code)
	Activities      []string // allowed activities, empty = all
	DefaultActivity string
	IssueOptional   bool
	Repos           []string // repos patterns that offer the project
}

func (d *Doc) projectNodes() []*yaml.Node {
	projects, _ := lookup(d.top(), "projects")
	var out []*yaml.Node
	for _, k := range keys(projects) {
		if p, _ := lookup(projects, k); p != nil && p.Kind == yaml.MappingNode {
			out = append(out, p)
		}
	}
	return out
}

// mustLookup returns the value of key or nil.
func mustLookup(m *yaml.Node, key string) *yaml.Node {
	v, _ := lookup(m, key)
	return v
}

// SetProject creates (old == "") or updates the project old, renaming it to
// code (also in repos), and makes exactly the patterns in p.Repos offer
// it. It returns the repos patterns removed because no project was left.
func (d *Doc) SetProject(old, code string, p Project) (removed []string, err error) {
	code = strings.TrimSpace(code)
	if err := checkCode(code, "projektu"); err != nil {
		return nil, err
	}
	projects := section(d.top(), "projects")
	if old != "" && !hasKey(projects, old) {
		return nil, fmt.Errorf("projekt %s neexistuje", old)
	}
	if code != old && hasKey(projects, code) {
		return nil, fmt.Errorf("projekt %s už existuje", code)
	}
	repos := section(d.top(), "repos")
	for _, r := range p.Repos {
		if !hasKey(repos, r) {
			return nil, fmt.Errorf("pravidlo %s v repos neexistuje", r)
		}
	}
	if old != "" && old != code {
		rename(projects, old, code)
		for _, pat := range keys(repos) {
			replaceInList(mustLookup(repos, pat), old, code)
		}
	}
	node, _ := lookup(projects, code)
	if node == nil || node.Kind != yaml.MappingNode {
		node = &yaml.Node{Kind: yaml.MappingNode}
		set(projects, code, node)
	}
	set(node, "name", scalar(strings.TrimSpace(p.Name)))
	setOrDel(node, "color", p.Color != "", func() *yaml.Node { return scalar(p.Color) })
	setOrDel(node, "activities", len(p.Activities) > 0, func() *yaml.Node { return list(p.Activities) })
	setOrDel(node, "default_activity", p.DefaultActivity != "", func() *yaml.Node { return scalar(p.DefaultActivity) })
	setOrDel(node, "issue", p.IssueOptional, func() *yaml.Node { return scalar(string(config.IssueOptional)) })

	for _, pat := range keys(repos) {
		codes := strs(mustLookup(repos, pat))
		has, want := slices.Contains(codes, code), slices.Contains(p.Repos, pat)
		switch {
		case want && !has:
			set(repos, pat, list(append(codes, code)))
		case has && !want:
			if len(codes) == 1 {
				del(repos, pat)
				removed = append(removed, pat)
			} else {
				replaceInList(mustLookup(repos, pat), code, "")
			}
		}
	}
	return removed, nil
}

func setOrDel(m *yaml.Node, key string, keep bool, val func() *yaml.Node) {
	if keep {
		set(m, key, val())
	} else {
		del(m, key)
	}
}

// DeleteProject removes a project and its use in repos. It returns the
// repos patterns removed because no project was left for them.
func (d *Doc) DeleteProject(code string) (removed []string, err error) {
	projects := section(d.top(), "projects")
	if !hasKey(projects, code) {
		return nil, fmt.Errorf("projekt %s neexistuje", code)
	}
	repos, _ := lookup(d.top(), "repos")
	for _, pat := range keys(repos) {
		codes := strs(mustLookup(repos, pat))
		if !slices.Contains(codes, code) {
			continue
		}
		if len(codes) == 1 {
			del(repos, pat)
			removed = append(removed, pat)
		} else {
			replaceInList(mustLookup(repos, pat), code, "")
		}
	}
	del(projects, code)
	return removed, nil
}

// --- repos ---------------------------------------------------------------------

// SetRepo creates (old == "") or updates the repos rule old, renaming it to
// pattern, with the given project codes.
func (d *Doc) SetRepo(old, pattern string, codes []string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fmt.Errorf("chybí cesta repozitáře")
	}
	if len(codes) == 0 {
		return fmt.Errorf("vyber aspoň jeden projekt")
	}
	repos := section(d.top(), "repos")
	if old != "" && !hasKey(repos, old) {
		return fmt.Errorf("pravidlo %s neexistuje", old)
	}
	for _, k := range keys(repos) {
		if k != old && strings.EqualFold(k, pattern) {
			return fmt.Errorf("pravidlo %s už existuje – uprav ho v seznamu repozitářů", k)
		}
	}
	if old != "" && old != pattern {
		rename(repos, old, pattern)
	}
	set(repos, pattern, list(codes))
	return nil
}

// DeleteRepo removes a repos rule.
func (d *Doc) DeleteRepo(pattern string) error {
	repos := section(d.top(), "repos")
	if !hasKey(repos, pattern) {
		return fmt.Errorf("pravidlo %s neexistuje", pattern)
	}
	del(repos, pattern)
	return nil
}

// --- general settings ------------------------------------------------------------

// AddGitLabHost adds host to gitlab_hosts unless it is there already.
func (d *Doc) AddGitLabHost(host string) {
	hosts := strs(mustLookup(d.top(), "gitlab_hosts"))
	if slices.ContainsFunc(hosts, func(h string) bool { return strings.EqualFold(h, host) }) {
		return
	}
	d.SetGitLabHosts(append(hosts, strings.ToLower(host)))
}

// SetGitLabHosts replaces gitlab_hosts.
func (d *Doc) SetGitLabHosts(hosts []string) {
	set(d.top(), "gitlab_hosts", list(hosts))
}

// SetBM sets the BM editor address, form field names and browser command.
func (d *Doc) SetBM(baseURL string, f config.BMFields, openCommand []string) {
	bm := section(d.top(), "bm")
	set(bm, "base_url", scalar(baseURL))
	fields := section(bm, "fields")
	for _, kv := range f.Ordered() {
		set(fields, kv[0], scalar(kv[1]))
	}
	setOrDel(bm, "open_command", len(openCommand) > 0, func() *yaml.Node { return list(openCommand) })
}

// SetTime sets the time limits (quarter-hours).
func (d *Doc) SetTime(t config.Time) {
	sec := section(d.top(), "time")
	set(sec, "daily_goal_hours", hoursNode(int(t.DailyGoal)))
	set(sec, "daily_warning_hours", hoursNode(int(t.DailyWarning)))
	set(sec, "max_entry_hours", hoursNode(int(t.MaxEntry)))
}

// SetUI sets the look of the TUI.
func (d *Doc) SetUI(u config.UI) {
	sec := section(d.top(), "ui")
	set(sec, "theme", scalar(u.Theme))
	set(sec, "nerd_font", boolean(u.NerdFont))
	set(sec, "caterpillar", boolean(u.Caterpillar))
	set(sec, "date_format", scalar(u.DateFormat))
	set(sec, "background", boolean(u.Background))
}

// SetWeb sets the web interface settings.
func (d *Doc) SetWeb(w config.Web) {
	sec := section(d.top(), "web")
	set(sec, "port", integer(w.Port))
	set(sec, "hostnames", list(w.Hostnames))
	set(sec, "open_browser", boolean(w.OpenBrowser))
	set(sec, "theme", scalar(w.Theme))
}
