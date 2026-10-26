package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/configedit"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/text"
)

// The settings pages edit the configuration file. Every change loads the
// file, applies one configedit operation, validates the whole result and
// saves it only when it is valid and the file did not change meanwhile
// (the page carries the hash of the version it shows).

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /settings", s.settingsHome)
	mux.HandleFunc("GET /settings/{tab}", s.settingsTab)
	mux.HandleFunc("POST /settings/yaml", s.saveYAML)

	mux.HandleFunc("GET /settings/repos/form", s.repoForm)
	mux.HandleFunc("GET /settings/repos/parse", s.repoParse)
	mux.HandleFunc("POST /settings/repos", s.saveRepo)
	mux.HandleFunc("POST /settings/repos/delete", s.deleteRepo)

	mux.HandleFunc("GET /settings/projects/form", s.projectForm)
	mux.HandleFunc("POST /settings/projects", s.saveProject)
	mux.HandleFunc("POST /settings/projects/delete", s.deleteProject)

	mux.HandleFunc("GET /settings/activities/form", s.activityForm)
	mux.HandleFunc("POST /settings/activities", s.saveActivity)
	mux.HandleFunc("POST /settings/activities/delete", s.deleteActivity)

	mux.HandleFunc("POST /settings/general/{part}", s.saveGeneral)
	mux.HandleFunc("GET /settings/bm/from-link", s.bmFromLink)
	mux.HandleFunc("GET /settings/date-preview", s.datePreview)
	mux.HandleFunc("POST /settings/examples/delete", s.deleteExamples)
}

var settingsTabs = []struct{ ID, Label string }{
	{"setup", "Průvodce"}, {"repos", "Repozitáře"}, {"projects", "Projekty"},
	{"activities", "Činnosti"}, {"general", "Obecné"}, {"yaml", "YAML"},
}

// exampleConfig is ExampleConfig parsed once per server.
func (s *Server) exampleConfig() *config.Config {
	s.exampleOnce.Do(func() {
		if cfg, err := config.Parse(ExampleConfig); err == nil {
			s.example = cfg
		}
	})
	return s.example
}

func (s *Server) setupSteps(cfg *config.Config) []configedit.Step {
	example := s.exampleConfig()
	if example == nil || cfg == nil {
		return nil
	}
	return configedit.Setup(cfg, example)
}

func (s *Server) settingsHome(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	tab := "projects"
	if q.cfg == nil {
		tab = "yaml"
	} else if !configedit.SetupDone(s.setupSteps(q.cfg)) {
		tab = "setup"
	}
	http.Redirect(w, r, "/settings/"+tab, http.StatusSeeOther)
}

func (s *Server) settingsTab(w http.ResponseWriter, r *http.Request) {
	tab := r.PathValue("tab")
	if !slices.ContainsFunc(settingsTabs, func(t struct{ ID, Label string }) bool { return t.ID == tab }) {
		http.NotFound(w, r)
		return
	}
	q := s.req(w, r)
	d := s.buildSettings(q, tab)
	if tab == "yaml" {
		d.YAML = s.yamlView(q)
	}
	s.render(w, "settings-page", d)
}

// --- view models -------------------------------------------------------------------

type settingsData struct {
	chrome
	Tab        string
	Tabs       []tabLink
	Hash       string
	ConfigErr  error
	OOB        bool
	Steps      []configedit.Step
	Repos      []repoRow
	Projects   []projectCard
	Activities []activityRow
	General    generalView
	YAML       yamlView
}

type tabLink struct {
	ID, Label string
	Active    bool
	Warn      bool
}

type badge struct {
	Code, Name string
	Class      string
	Style      template.CSS
	Kind       string // activities: "good", "bad"
}

type repoRow struct {
	Pattern  string
	Wildcard bool
	Projects []badge
	Entries  int
	Confirm  string
}

type projectCard struct {
	badge
	IssueOptional bool
	Activities    []badge // empty = all activities
	Default       string
	Repos         []string
	Entries       int
	Warning       string
	Confirm       string
}

type activityRow struct {
	badge
	Productive bool
	Projects   []string
	Entries    int
	Confirm    string
}

type yamlView struct {
	Text     string
	Problems []string
	Message  string
}

func (s *Server) projectBadge(cfg *config.Config, code string) badge {
	cls, style := projectColor(cfg, code)
	return badge{Code: code, Name: cfg.Projects[code].Name, Class: cls, Style: style}
}

func activityBadge(cfg *config.Config, code string) badge {
	a := cfg.Activities[code]
	kind := "good"
	if !a.IsProductive() {
		kind = "bad"
	}
	return badge{Code: code, Name: a.Name, Kind: kind}
}

// buildSettings prepares the settings page for the current configuration.
func (s *Server) buildSettings(q *req, tab string) settingsData {
	d := settingsData{chrome: s.chrome(q, "settings"), Tab: tab, ConfigErr: q.cfgErr}
	if data, err := os.ReadFile(s.cfgPath); err == nil {
		d.Hash = configedit.Hash(data)
	}
	cfg := q.cfg
	d.Steps = s.setupSteps(cfg)
	for _, t := range settingsTabs {
		l := tabLink{ID: t.ID, Label: t.Label, Active: t.ID == tab}
		if t.ID == "setup" {
			l.Warn = d.Setup
		}
		if cfg == nil && t.ID != "yaml" {
			continue
		}
		d.Tabs = append(d.Tabs, l)
	}
	if cfg == nil {
		return d
	}

	all, _ := s.store.All()
	usage := configedit.CountUsage(all)
	repoEntries := map[string]int{}
	for _, e := range all {
		if ref, err := gitlaburl.Parse(e.IssueURL); err == nil {
			if pat, _, ok := q.rules.MatchRepo(ref.ProjectPath); ok {
				repoEntries[pat]++
			}
		}
	}

	for _, pat := range sortedRepoKeys(cfg) {
		row := repoRow{Pattern: pat, Wildcard: strings.HasSuffix(pat, "/*"), Entries: repoEntries[pat]}
		for _, code := range cfg.Repos[pat] {
			row.Projects = append(row.Projects, s.projectBadge(cfg, code))
		}
		row.Confirm = fmt.Sprintf("Smazat pravidlo %s? Odkazy z tohoto repozitáře pak nepůjdou vykázat, dokud ho nepřidáš znovu.", pat)
		d.Repos = append(d.Repos, row)
	}

	warnings := cfg.Warnings()
	for _, code := range cfg.ProjectCodes() {
		p := cfg.Projects[code]
		c := projectCard{badge: s.projectBadge(cfg, code), IssueOptional: p.IssueOptional(), Default: p.DefaultActivity,
			Repos: configedit.ReposOf(cfg, code), Entries: usage.Projects[code]}
		for _, a := range p.Activities {
			c.Activities = append(c.Activities, activityBadge(cfg, a))
		}
		for _, w := range warnings {
			if strings.HasPrefix(w, "projects."+code+":") {
				c.Warning = strings.TrimSpace(strings.TrimPrefix(w, "projects."+code+":"))
			}
		}
		c.Confirm = deleteProjectQuestion(cfg, code, c.Entries)
		d.Projects = append(d.Projects, c)
	}

	for _, code := range cfg.ActivityCodes() {
		a := cfg.Activities[code]
		row := activityRow{badge: activityBadge(cfg, code), Productive: a.IsProductive(),
			Projects: configedit.ProjectsOf(cfg, code), Entries: usage.Activities[code]}
		row.Confirm = fmt.Sprintf("Smazat činnost %s (%s)?", code, a.Name)
		if len(row.Projects) > 0 {
			row.Confirm += " Zmizí z povolených činností projektů " + strings.Join(row.Projects, ", ") + "."
		}
		if row.Entries > 0 {
			row.Confirm += fmt.Sprintf(" %s s ní zůstane, ale bude neplatných.", text.Plural(row.Entries, "záznam", "záznamy", "záznamů"))
		}
		d.Activities = append(d.Activities, row)
	}

	d.General = newGeneralView(cfg, q.today)
	d.General.WebThemes = s.webThemes()
	return d
}

func deleteProjectQuestion(cfg *config.Config, code string, entries int) string {
	q := fmt.Sprintf("Smazat projekt %s (%s)?", code, cfg.Projects[code].Name)
	if repos := configedit.ReposOf(cfg, code); len(repos) > 0 {
		q += " Zmizí z pravidel " + strings.Join(repos, ", ") + "."
		if only := configedit.OnlyRepoOf(cfg, code); len(only) > 0 {
			q += " Pravidla " + strings.Join(only, ", ") + " se odstraní celá."
		}
	}
	if entries > 0 {
		q += fmt.Sprintf(" %s s tímto kódem zůstane, ale bude neplatných.", text.Plural(entries, "záznam", "záznamy", "záznamů"))
	}
	return q
}

func sortedRepoKeys(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Repos))
	for k := range cfg.Repos {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out
}

// --- saving -----------------------------------------------------------------------

// change loads the configuration, applies op and saves the result. It
// returns the problems to show: validation errors, a conflict or op's error.
func (s *Server) change(r *http.Request, op func(d *configedit.Doc) error) []string {
	d, err := configedit.Load(s.cfgPath)
	if err != nil {
		return []string{err.Error()}
	}
	if d.Hash() != r.FormValue("hash") {
		return []string{configedit.ErrConflict.Error()}
	}
	if err := op(d); err != nil {
		return []string{err.Error()}
	}
	if err := d.Save(s.cfgPath); err != nil {
		var p config.Problems
		if errors.As(err, &p) {
			return p
		}
		return []string{strings.TrimPrefix(err.Error(), "neplatný config:\n  - ")}
	}
	return nil
}

// saved answers a successful change: it closes the modal and swaps in the
// tab with the new configuration and a toast.
func (s *Server) saved(w http.ResponseWriter, r *http.Request, tab, msg string) {
	q := s.req(w, r) // re-reads the saved configuration
	d := s.buildSettings(q, tab)
	d.OOB = true
	if tab == "yaml" {
		d.YAML = s.yamlView(q)
		d.YAML.Message = msg
	}
	s.render(w, "settings-update", struct {
		Settings settingsData
		Toast    toastData
	}{d, toastData{Text: msg}})
}

func formList(r *http.Request, key string) []string {
	r.ParseForm()
	var out []string
	for _, v := range r.Form[key] {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' || r == '\r' }) {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// --- YAML -------------------------------------------------------------------------

func (s *Server) yamlView(q *req) yamlView {
	data, err := os.ReadFile(s.cfgPath)
	if err != nil {
		return yamlView{Problems: []string{err.Error()}}
	}
	v := yamlView{Text: string(data)}
	if _, err := config.Parse(data); err != nil {
		v.Problems = problems(err)
	}
	return v
}

func problems(err error) []string {
	var p config.Problems
	if errors.As(err, &p) {
		return p
	}
	return []string{err.Error()}
}

func (s *Server) saveYAML(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	// Browsers send text areas with CRLF; keep the line endings of the file.
	body := strings.ReplaceAll(r.FormValue("yaml"), "\r\n", "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if cur, err := os.ReadFile(s.cfgPath); err == nil && strings.Contains(string(cur), "\r\n") {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}
	// The text stays based on the version the page was loaded with: a save
	// after Check or after an error must still notice changes made
	// elsewhere meanwhile.
	based := r.FormValue("hash")
	d := s.buildSettings(q, "yaml")
	d.Hash = based
	d.YAML = yamlView{Text: body}
	if _, err := config.Parse([]byte(body)); err != nil {
		d.YAML.Problems = problems(err)
		s.render(w, "settings-content", d)
		return
	}
	if r.FormValue("action") != "save" {
		d.YAML.Message = "Config je v pořádku. Ulož ho tlačítkem Uložit."
		s.render(w, "settings-content", d)
		return
	}
	if err := configedit.WriteFile(s.cfgPath, []byte(body), based); err != nil {
		d.YAML.Problems = []string{err.Error()}
		if errors.Is(err, configedit.ErrConflict) {
			d.YAML.Problems = append(d.YAML.Problems, "Tvůj text zůstal v poli níže – zkopíruj si úpravy a načti stránku znovu, ať upravuješ aktuální verzi.")
		}
		s.render(w, "settings-content", d)
		return
	}
	q = s.req(w, r)
	d = s.buildSettings(q, "yaml")
	d.YAML = s.yamlView(q)
	d.YAML.Message = "Uloženo. Předchozí verze je v " + s.cfgPath + ".bak."
	s.render(w, "settings-content", d)
}

// --- repos --------------------------------------------------------------------------

type repoFormView struct {
	Old      string
	Input    string
	Analysis repoAnalysis
	Projects []checkOption
	NewCode  string
	NewName  string
	Errors   []string
}

type checkOption struct {
	badge
	Value   string
	Checked bool
	Note    string
}

type repoAnalysis struct {
	Err       string
	Host      string
	HostKnown bool
	AddHost   bool
	Patterns  []patternOption
	Covered   string // existing rule that already applies
	Hint      string
}

type patternOption struct {
	Pattern string
	Label   string
	Checked bool
	Exists  bool
}

// domainLike matches a first path segment that may be a GitLab host typed
// without https:// (only a hint: groups may have dots in their names too).
var domainLike = regexp.MustCompile(`^(?i)[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,}$`)

// analyzeRepo explains what the typed repository means for the config.
func analyzeRepo(cfg *config.Config, r *rules.Rules, input, chosen, old string) repoAnalysis {
	var a repoAnalysis
	if strings.TrimSpace(input) == "" {
		return a
	}
	ref, err := gitlaburl.ParseRepo(input, cfg.GitLabHosts)
	if err != nil {
		a.Err = err.Error()
		return a
	}
	if first, _, _ := strings.Cut(ref.Path, "/"); ref.Host == "" && domainLike.MatchString(first) {
		a.Hint = "Je-li " + first + " doména GitLabu, vlož celý odkaz i s https:// – doména se přidá mezi GitLab domény."
	}
	if ref.Host != "" {
		a.Host = ref.Host
		a.HostKnown = slices.ContainsFunc(cfg.GitLabHosts, func(h string) bool { return strings.EqualFold(h, ref.Host) })
		a.AddHost = !a.HostKnown
	}
	pats := ref.Patterns()
	if chosen == "" || !slices.Contains(pats, chosen) {
		chosen = pats[0]
	}
	for i, p := range pats {
		label := "jen tento repozitář"
		if strings.HasSuffix(p, "/*") {
			label = "všechny repozitáře ve skupině " + strings.TrimSuffix(p, "/*")
		}
		if i == 0 && ref.Wildcard {
			label = "celá skupina " + strings.TrimSuffix(p, "/*")
		}
		exists := false
		for k := range cfg.Repos {
			exists = exists || strings.EqualFold(k, p) && k != old
		}
		a.Patterns = append(a.Patterns, patternOption{Pattern: p, Label: label, Checked: p == chosen, Exists: exists})
	}
	if !ref.Wildcard {
		if pat, codes, ok := r.MatchRepo(ref.Path); ok && pat != old {
			a.Covered = fmt.Sprintf("Tento repozitář už pokrývá pravidlo %s → %s.", pat, strings.Join(codes, ", "))
		}
	}
	return a
}

func (s *Server) buildRepoForm(cfg *config.Config, r *rules.Rules, f repoFormView, chosen string, selected []string) repoFormView {
	f.Analysis = analyzeRepo(cfg, r, f.Input, chosen, f.Old)
	for _, code := range cfg.ProjectCodes() {
		f.Projects = append(f.Projects, checkOption{badge: s.projectBadge(cfg, code), Value: code, Checked: slices.Contains(selected, code)})
	}
	return f
}

func (s *Server) repoForm(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	f := repoFormView{Old: r.URL.Query().Get("pattern")}
	var selected []string
	if f.Old != "" {
		codes, ok := q.cfg.Repos[f.Old]
		if !ok {
			s.fail(w, fmt.Errorf("pravidlo %s neexistuje – načti stránku znovu", f.Old))
			return
		}
		f.Input, selected = f.Old, codes
	}
	s.render(w, "repo-form", s.buildRepoForm(q.cfg, q.rules, f, f.Old, selected))
}

func (s *Server) repoParse(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	s.render(w, "repo-analysis", analyzeRepo(q.cfg, q.rules, r.FormValue("repo"), r.FormValue("pattern"), r.FormValue("old")))
}

func (s *Server) saveRepo(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	f := repoFormView{Old: r.FormValue("old"), Input: r.FormValue("repo"),
		NewCode: strings.TrimSpace(r.FormValue("new_code")), NewName: strings.TrimSpace(r.FormValue("new_name"))}
	codes := formList(r, "projects")
	pattern := r.FormValue("pattern")
	a := analyzeRepo(q.cfg, q.rules, f.Input, pattern, f.Old)
	switch {
	case a.Err != "":
		f.Errors = []string{a.Err}
	case len(a.Patterns) == 0:
		f.Errors = []string{"zadej odkaz na repozitář nebo cestu skupina/projekt"}
	case f.NewCode == "" && f.NewName != "":
		f.Errors = []string{"u nového projektu chybí kód"}
	}
	if f.Errors == nil {
		if !slices.ContainsFunc(a.Patterns, func(p patternOption) bool { return p.Pattern == pattern }) {
			pattern = a.Patterns[0].Pattern
		}
		if f.NewCode != "" {
			codes = append(codes, f.NewCode)
		}
		f.Errors = s.change(r, func(d *configedit.Doc) error {
			if a.Host != "" && !a.HostKnown && r.FormValue("add_host") != "" {
				d.AddGitLabHost(a.Host)
			}
			if f.NewCode != "" {
				name := f.NewName
				if name == "" {
					name = f.NewCode
				}
				if _, err := d.SetProject("", f.NewCode, configedit.Project{Name: name}); err != nil {
					return err
				}
			}
			return d.SetRepo(f.Old, pattern, codes)
		})
	}
	if f.Errors != nil {
		s.render(w, "repo-form", s.buildRepoForm(q.cfg, q.rules, f, pattern, codes))
		return
	}
	msg := "Repozitář " + pattern + " uložen"
	if a.Host != "" && !a.HostKnown && r.FormValue("add_host") != "" {
		msg += ", doména " + a.Host + " přidána"
	}
	s.saved(w, r, "repos", msg)
}

func (s *Server) deleteRepo(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if errs := s.change(r, func(d *configedit.Doc) error { return d.DeleteRepo(pattern) }); errs != nil {
		s.fail(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	s.saved(w, r, "repos", "Pravidlo "+pattern+" smazáno")
}

// --- projects --------------------------------------------------------------------------

type projectFormView struct {
	Old           string
	Code          string
	Name          string
	Color         string // color name, "custom" or "" (automatic)
	Custom        string // #rrggbb when Color is "custom"
	Swatches      []swatch
	Activities    []checkOption
	Default       string
	Defaults      []badge
	IssueOptional bool
	Repos         []checkOption
	Entries       int
	Errors        []string
	Derived       string // the automatic color name
}

type swatch struct {
	Name, Label string
	Checked     bool
}

var colorLabels = map[string]string{
	"red": "červená", "orange": "oranžová", "yellow": "žlutá", "green": "zelená", "teal": "tyrkysová",
	"cyan": "azurová", "blue": "modrá", "purple": "fialová", "pink": "růžová",
}

func (s *Server) buildProjectForm(cfg *config.Config, f projectFormView, acts, repos []string) projectFormView {
	if f.Code != "" {
		f.Derived = config.DerivedColorName(f.Code)
	}
	for _, n := range config.ColorNames {
		f.Swatches = append(f.Swatches, swatch{Name: n, Label: colorLabels[n], Checked: f.Color == n})
	}
	for _, code := range cfg.ActivityCodes() {
		b := activityBadge(cfg, code)
		f.Activities = append(f.Activities, checkOption{badge: b, Value: code, Checked: slices.Contains(acts, code)})
		f.Defaults = append(f.Defaults, b)
	}
	for _, pat := range sortedRepoKeys(cfg) {
		o := checkOption{Value: pat, Checked: slices.Contains(repos, pat)}
		o.Code = pat
		if others := slices.DeleteFunc(slices.Clone(cfg.Repos[pat]), func(c string) bool { return c == f.Old }); len(others) > 0 {
			o.Note = strings.Join(others, ", ")
		}
		f.Repos = append(f.Repos, o)
	}
	return f
}

func (s *Server) projectForm(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	f := projectFormView{Old: r.URL.Query().Get("code")}
	var acts, repos []string
	if f.Old != "" {
		p, ok := q.cfg.Projects[f.Old]
		if !ok {
			s.fail(w, fmt.Errorf("projekt %s neexistuje – načti stránku znovu", f.Old))
			return
		}
		f.Code, f.Name, f.Default, f.IssueOptional = f.Old, p.Name, p.DefaultActivity, p.IssueOptional()
		switch {
		case strings.HasPrefix(p.Color, "#"):
			f.Color, f.Custom = "custom", p.Color
		default:
			f.Color = p.Color
		}
		acts, repos = p.Activities, configedit.ReposOf(q.cfg, f.Old)
		all, _ := s.store.All()
		f.Entries = configedit.CountUsage(all).Projects[f.Old]
	} else if pat := r.URL.Query().Get("repo"); pat != "" {
		repos = []string{pat}
	}
	s.render(w, "project-form", s.buildProjectForm(q.cfg, f, acts, repos))
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	f := projectFormView{Old: r.FormValue("old"), Code: strings.TrimSpace(r.FormValue("code")),
		Name: strings.TrimSpace(r.FormValue("name")), Color: r.FormValue("color"), Custom: r.FormValue("custom"),
		Default: r.FormValue("default_activity"), IssueOptional: r.FormValue("issue_optional") != ""}
	acts, repos := formList(r, "activities"), formList(r, "repos")
	color := f.Color
	if color == "custom" {
		color = f.Custom
	}
	if len(acts) > 0 && f.Default != "" && !slices.Contains(acts, f.Default) {
		f.Errors = []string{"výchozí činnost " + f.Default + " není mezi povolenými činnostmi"}
	}
	var removed []string
	if f.Errors == nil {
		f.Errors = s.change(r, func(d *configedit.Doc) error {
			var err error
			removed, err = d.SetProject(f.Old, f.Code, configedit.Project{Name: f.Name, Color: color, Activities: acts,
				DefaultActivity: f.Default, IssueOptional: f.IssueOptional, Repos: repos})
			return err
		})
	}
	if f.Errors != nil {
		if f.Old != "" {
			all, _ := s.store.All()
			f.Entries = configedit.CountUsage(all).Projects[f.Old]
		}
		s.render(w, "project-form", s.buildProjectForm(q.cfg, f, acts, repos))
		return
	}
	msg := "Projekt " + f.Code + " uložen"
	if f.Old != "" && f.Old != f.Code {
		msg = "Projekt " + f.Old + " přejmenován na " + f.Code
	}
	if len(removed) > 0 {
		msg += "; pravidla bez projektu odstraněna: " + strings.Join(removed, ", ")
	}
	s.saved(w, r, "projects", msg)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	var removed []string
	errs := s.change(r, func(d *configedit.Doc) error {
		var err error
		removed, err = d.DeleteProject(code)
		return err
	})
	if errs != nil {
		s.fail(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	msg := "Projekt " + code + " smazán"
	if len(removed) > 0 {
		msg += "; odstraněna pravidla " + strings.Join(removed, ", ")
	}
	s.saved(w, r, "projects", msg)
}

// --- activities ----------------------------------------------------------------------------

type activityFormView struct {
	Old        string
	Code       string
	Name       string
	Productive bool
	Entries    int
	Projects   []string
	Errors     []string
}

func (s *Server) activityForm(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	f := activityFormView{Old: r.URL.Query().Get("code"), Productive: true}
	if f.Old != "" {
		a, ok := q.cfg.Activities[f.Old]
		if !ok {
			s.fail(w, fmt.Errorf("činnost %s neexistuje – načti stránku znovu", f.Old))
			return
		}
		f.Code, f.Name, f.Productive = f.Old, a.Name, a.IsProductive()
		all, _ := s.store.All()
		f.Entries = configedit.CountUsage(all).Activities[f.Old]
		f.Projects = configedit.ProjectsOf(q.cfg, f.Old)
	}
	s.render(w, "activity-form", f)
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	f := activityFormView{Old: r.FormValue("old"), Code: strings.TrimSpace(r.FormValue("code")),
		Name: strings.TrimSpace(r.FormValue("name")), Productive: r.FormValue("productive") == "true"}
	f.Errors = s.change(r, func(d *configedit.Doc) error {
		return d.SetActivity(f.Old, f.Code, configedit.Activity{Name: f.Name, Productive: f.Productive})
	})
	if f.Errors != nil {
		s.render(w, "activity-form", f)
		return
	}
	msg := "Činnost " + f.Code + " uložena"
	if f.Old != "" && f.Old != f.Code {
		msg = "Činnost " + f.Old + " přejmenována na " + f.Code
	}
	s.saved(w, r, "activities", msg)
}

func (s *Server) deleteActivity(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if errs := s.change(r, func(d *configedit.Doc) error { return d.DeleteActivity(code) }); errs != nil {
		s.fail(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	s.saved(w, r, "activities", "Činnost "+code+" smazána")
}

// deleteExamples removes the example repos, projects and activities that
// are unchanged and not used by entries or by anything the user added.
func (s *Server) deleteExamples(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	ex := s.exampleConfig()
	if ex == nil {
		s.fail(w, errors.New("vzorový config není k dispozici"))
		return
	}
	all, err := s.store.All()
	if err != nil {
		s.fail(w, err)
		return
	}
	l := configedit.ExampleLeftovers(q.cfg, ex, configedit.CountUsage(all))
	if l.Empty() {
		s.fail(w, errors.New("Žádné nepoužité vzorové položky – co zbylo, používají tvoje projekty nebo záznamy"))
		return
	}
	if errs := s.change(r, func(d *configedit.Doc) error { return d.DeleteLeftovers(l) }); errs != nil {
		s.fail(w, errors.New("Vzorové položky nejde odstranit: "+strings.Join(errs, "; ")+
			" – nejdřív přidej vlastní činnosti a projekty"))
		return
	}
	var parts []string
	for _, p := range [][]string{l.Repos, l.Projects, l.Activities} {
		parts = append(parts, p...)
	}
	s.saved(w, r, "setup", "Odstraněny vzorové položky: "+strings.Join(parts, ", "))
}

// --- general ---------------------------------------------------------------------------------

type generalView struct {
	Hosts       string
	BaseURL     string
	Fields      config.BMFields
	OpenCommand string
	Goal        string
	Warning     string
	Max         string
	UI          config.UI
	TUIThemes   []string
	DatePreview string
	Web         config.Web
	WebHosts    string
	WebThemes   []themeOption
	Part        string   // the part the errors belong to
	Errors      []string // of Part
}

func (s *Server) webThemes() []themeOption {
	out := []themeOption{{ID: themeAuto, Name: "Podle systému"}}
	for _, t := range s.themes.list {
		out = append(out, themeOption{ID: t.ID, Name: t.Name})
	}
	return out
}

func newGeneralView(cfg *config.Config, today civil.Date) generalView {
	return generalView{
		Hosts: strings.Join(cfg.GitLabHosts, "\n"), BaseURL: cfg.BM.BaseURL, Fields: cfg.BM.Fields,
		OpenCommand: text.JoinArgs(cfg.BM.OpenCommand),
		Goal:        hoursText(int(cfg.Time.DailyGoal)), Warning: hoursText(int(cfg.Time.DailyWarning)), Max: hoursText(int(cfg.Time.MaxEntry)),
		UI: cfg.UI, TUIThemes: config.Themes, DatePreview: cfg.UI.DateLayout().Format(today),
		Web: cfg.Web, WebHosts: strings.Join(cfg.Web.Hostnames, "\n"),
	}
}

func hoursText(q int) string {
	if q%hours.PerHour == 0 {
		return strconv.Itoa(q / hours.PerHour)
	}
	return hours.Format(q)
}

func (s *Server) saveGeneral(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	part := r.PathValue("part")
	var op func(d *configedit.Doc) error
	var bad []string
	parseHours := func(label, v string) config.Hours {
		n, err := hours.Parse(v)
		if err != nil {
			bad = append(bad, label+": "+err.Error())
		}
		return config.Hours(n)
	}
	msg := "Uloženo"
	switch part {
	case "gitlab":
		hosts := lines(r.FormValue("hosts"))
		op = func(d *configedit.Doc) error { d.SetGitLabHosts(hosts); return nil }
		msg = "Domény GitLabu uloženy"
	case "bm":
		f := config.BMFields{Project: r.FormValue("f_project"), Issue: r.FormValue("f_issue"), Hours: r.FormValue("f_hours"),
			Activity: r.FormValue("f_activity"), Date: r.FormValue("f_date"), Description: r.FormValue("f_description")}
		base := strings.TrimSpace(r.FormValue("base_url"))
		open, err := text.SplitArgs(r.FormValue("open_command"))
		if err != nil {
			bad = append(bad, "příkaz prohlížeče: "+err.Error())
		}
		op = func(d *configedit.Doc) error { d.SetBM(base, f, open); return nil }
		msg = "Nastavení Bílého motýla uloženo"
	case "time":
		t := config.Time{DailyGoal: parseHours("denní cíl", r.FormValue("goal")),
			DailyWarning: parseHours("varování", r.FormValue("warning")), MaxEntry: parseHours("maximum záznamu", r.FormValue("max"))}
		op = func(d *configedit.Doc) error { d.SetTime(t); return nil }
		msg = "Časové limity uloženy"
	case "ui":
		u := config.UI{Theme: r.FormValue("theme"), NerdFont: r.FormValue("nerd_font") != "", Caterpillar: r.FormValue("caterpillar") != "",
			DateFormat: strings.TrimSpace(r.FormValue("date_format")), Background: r.FormValue("background") != ""}
		op = func(d *configedit.Doc) error { d.SetUI(u); return nil }
		msg = "Vzhled TUI uložen"
	case "web":
		port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
		if err != nil {
			bad = append(bad, "port musí být číslo")
		}
		wc := config.Web{Port: port, Hostnames: lines(r.FormValue("hostnames")), OpenBrowser: r.FormValue("open_browser") != "",
			Theme: r.FormValue("web_theme")}
		op = func(d *configedit.Doc) error { d.SetWeb(wc); return nil }
		msg = "Nastavení webu uloženo – port a názvy hostitele platí po restartu (housenka web stop a start)"
	default:
		http.NotFound(w, r)
		return
	}
	if bad == nil {
		bad = s.change(r, op)
	}
	if bad != nil {
		d := s.buildSettings(q, "general")
		d.General = generalFromForm(d.General, r, part)
		d.General.WebThemes = s.webThemes()
		d.General.Part, d.General.Errors = part, bad
		s.render(w, "settings-content", d)
		return
	}
	s.saved(w, r, "general", msg)
}

// generalFromForm keeps what was typed into the part that failed to save.
func generalFromForm(g generalView, r *http.Request, part string) generalView {
	switch part {
	case "gitlab":
		g.Hosts = r.FormValue("hosts")
	case "bm":
		g.BaseURL, g.OpenCommand = r.FormValue("base_url"), r.FormValue("open_command")
		g.Fields = config.BMFields{Project: r.FormValue("f_project"), Issue: r.FormValue("f_issue"), Hours: r.FormValue("f_hours"),
			Activity: r.FormValue("f_activity"), Date: r.FormValue("f_date"), Description: r.FormValue("f_description")}
	case "time":
		g.Goal, g.Warning, g.Max = r.FormValue("goal"), r.FormValue("warning"), r.FormValue("max")
	case "ui":
		g.UI = config.UI{Theme: r.FormValue("theme"), NerdFont: r.FormValue("nerd_font") != "", Caterpillar: r.FormValue("caterpillar") != "",
			DateFormat: r.FormValue("date_format"), Background: r.FormValue("background") != ""}
	case "web":
		g.WebHosts = r.FormValue("hostnames")
		g.Web.OpenBrowser, g.Web.Theme = r.FormValue("open_browser") != "", r.FormValue("web_theme")
		if p, err := strconv.Atoi(r.FormValue("port")); err == nil {
			g.Web.Port = p
		}
	}
	return g
}

// bmFromLink fills the BM address and field names from a link of the BM editor.
func (s *Server) bmFromLink(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	// Start from what is typed in the form, not from the saved config.
	g := generalFromForm(newGeneralView(q.cfg, q.today), r, "bm")
	base, f, err := configedit.BMFromLink(r.FormValue("link"))
	if err != nil {
		g.Part, g.Errors = "bm", []string{err.Error()}
	} else {
		g.BaseURL, g.Fields = base, f
		g.Part = "bm-filled"
	}
	s.render(w, "bm-fields", g)
}

func (s *Server) datePreview(w http.ResponseWriter, r *http.Request) {
	l, err := civil.ParseLayout(r.FormValue("date_format"))
	text := ""
	if err != nil {
		text = "✗ " + err.Error()
	} else {
		text = "dnes: " + l.Format(s.clock())
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(template.HTMLEscapeString(text)))
}
