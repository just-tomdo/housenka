package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/configedit"
	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
)

// The Issues tab of the settings edits issues.yaml, the extension's own
// file, with the same guarantees as config.yaml: comments stay, the result
// is validated, a concurrent change is refused (the forms carry ihash) and
// the previous version stays in issues.yaml.bak. It also turns the
// extension on and off and keeps the GitLab token, which it never shows.

func (s *Server) issueSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /settings/issues/enable", s.enableIssues)
	mux.HandleFunc("POST /settings/issues/disable", s.disableIssues)
	mux.HandleFunc("POST /settings/issues/general", s.saveIssueGeneral)
	mux.HandleFunc("GET /settings/issues/status-form", s.statusForm)
	mux.HandleFunc("POST /settings/issues/status", s.saveStatus)
	mux.HandleFunc("POST /settings/issues/status/move", s.moveStatus)
	mux.HandleFunc("POST /settings/issues/status/delete", s.deleteStatus)
	mux.HandleFunc("POST /settings/issues/token", s.saveToken)
	mux.HandleFunc("POST /settings/issues/yaml", s.saveIssuesYAML)
}

type issueSettingsView struct {
	Path      string
	Enabled   bool
	ConfigErr string
	Hash      string // of issues.yaml, carried as ihash
	Host      string
	HostHint  string // the first gitlab_hosts, used when Host is empty
	Sync      int
	Stale     int
	Statuses  []statusRow
	Token     string // "file", "env" or ""
	Part      string // the form that failed
	Errors    []string
	Message   string // shown in the token or YAML card
	YAML      string
	Problems  []string
}

type statusRow struct {
	issueStatusView
	Name    string
	Color   string
	Watched bool
	Issues  int
	Confirm string
	First   bool
	Last    bool
}

func (s *Server) buildIssueSettings(cfg *config.Config) issueSettingsView {
	v := issueSettingsView{Path: s.iss.path}
	data, err := os.ReadFile(s.iss.path)
	if err != nil {
		return v
	}
	v.Enabled, v.Hash, v.YAML = true, configedit.Hash(data), string(data)
	if cfg != nil && len(cfg.GitLabHosts) > 0 {
		v.HostHint = cfg.GitLabHosts[0]
	}
	icfg, err := s.iss.config()
	if err != nil {
		v.ConfigErr = err.Error()
		return v
	}
	if icfg == nil {
		return v
	}
	v.Host, v.Sync, v.Stale = icfg.Host, icfg.SyncMinutes, icfg.StaleDays
	counts := map[string]int{}
	if st, err := s.iss.openStore(); err == nil {
		if f, err := st.Load(); err == nil {
			for _, is := range f.Issues {
				counts[is.Status]++
			}
		}
	}
	for i, st := range icfg.Statuses {
		row := statusRow{issueStatusView: newIssueStatusView(st, i), Name: st.Name, Color: st.Color, Watched: st.Watched(),
			Issues: counts[st.Code], First: i == 0, Last: i == len(icfg.Statuses)-1}
		row.Confirm = "Smazat stav „" + st.Label() + "“?"
		if row.Issues > 0 {
			row.Confirm += " Jeho issue (" + strconv.Itoa(row.Issues) + ") se vrátí mezi nová."
		}
		v.Statuses = append(v.Statuses, row)
	}
	home, _ := config.HomeDir()
	switch {
	case gitlab.TokenFromEnv():
		v.Token = "env"
	case home != "":
		if _, err := gitlab.Token(home); err == nil {
			v.Token = "file"
		}
	}
	return v
}

// changeIssues applies op to issues.yaml and saves it, like change does
// for config.yaml.
func (s *Server) changeIssues(r *http.Request, op func(d *configedit.Doc) error) []string {
	d, err := configedit.LoadIssues(s.iss.path)
	if err != nil {
		return []string{err.Error()}
	}
	if d.Hash() != r.FormValue("ihash") {
		return []string{configedit.ErrConflict.Error()}
	}
	if err := op(d); err != nil {
		return []string{err.Error()}
	}
	if err := d.Save(s.iss.path); err != nil {
		var p config.Problems
		if errors.As(err, &p) {
			return p
		}
		return []string{strings.TrimPrefix(err.Error(), "neplatný config:\n  - ")}
	}
	return nil
}

// issueSettingsFailed shows the tab again with the errors in the card part.
func (s *Server) issueSettingsFailed(w http.ResponseWriter, r *http.Request, part string, errs []string) {
	q := s.req(w, r)
	d := s.buildSettings(q, "issues")
	d.Issues.Part, d.Issues.Errors = part, errs
	s.render(w, "settings-content", d)
}

func (s *Server) enableIssues(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(s.iss.path); err == nil {
		s.saved(w, r, "issues", "Přehled issues už je zapnutý")
		return
	}
	// A file switched off before comes back with its statuses.
	if err := os.Rename(s.iss.path+".off", s.iss.path); err != nil {
		if len(issues.ExampleConfig) == 0 {
			s.fail(w, errors.New("vzorový issues.yaml není k dispozici"))
			return
		}
		if err := atomicfile.Write(s.iss.path, issues.ExampleConfig, 0o644); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.saved(w, r, "issues", "Přehled issues zapnutý – nastav token GitLabu")
}

func (s *Server) disableIssues(w http.ResponseWriter, r *http.Request) {
	if err := os.Rename(s.iss.path, s.iss.path+".off"); err != nil {
		s.fail(w, err)
		return
	}
	s.saved(w, r, "issues", "Přehled issues vypnutý – nastavení i data zůstávají, Zapnout je vrátí")
}

func (s *Server) saveIssueGeneral(w http.ResponseWriter, r *http.Request) {
	var bad []string
	num := func(label, v string) int {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			bad = append(bad, label+" musí být celé číslo")
		}
		return n
	}
	host := r.FormValue("host")
	sync, stale := num("interval načítání", r.FormValue("sync_minutes")), num("počet dní", r.FormValue("stale_days"))
	if bad == nil {
		bad = s.changeIssues(r, func(d *configedit.Doc) error { d.SetIssueSettings(host, sync, stale); return nil })
	}
	if bad != nil {
		s.issueSettingsFailed(w, r, "general", bad)
		return
	}
	s.saved(w, r, "issues", "Nastavení přehledu issues uloženo")
}

type statusFormView struct {
	Hash     string // of issues.yaml
	Old      string
	Code     string
	Name     string
	Color    string // color name, "custom" or "" (automatic)
	Custom   string
	Watch    bool
	Swatches []swatch
	Derived  string
	Errors   []string
}

func (s *Server) buildStatusForm(f statusFormView) statusFormView {
	if data, err := os.ReadFile(s.iss.path); err == nil && f.Hash == "" {
		f.Hash = configedit.Hash(data)
	}
	if f.Code != "" {
		f.Derived = config.DerivedColorName(f.Code)
	}
	for _, n := range config.ColorNames {
		f.Swatches = append(f.Swatches, swatch{Name: n, Label: colorLabels[n], Checked: f.Color == n})
	}
	return f
}

func (s *Server) statusForm(w http.ResponseWriter, r *http.Request) {
	f := statusFormView{Old: r.URL.Query().Get("code"), Watch: true}
	if f.Old != "" {
		icfg, err := s.iss.config()
		if err != nil || icfg == nil {
			s.fail(w, errors.New("issues.yaml nejde načíst – načti stránku znovu"))
			return
		}
		st, ok := icfg.Status(f.Old)
		if !ok {
			s.fail(w, errors.New("stav "+f.Old+" neexistuje – načti stránku znovu"))
			return
		}
		f.Code, f.Name, f.Watch, f.Color = st.Code, st.Name, st.Watched(), st.Color
		if strings.HasPrefix(st.Color, "#") {
			f.Color, f.Custom = "custom", st.Color
		}
	}
	s.render(w, "status-form", s.buildStatusForm(f))
}

func (s *Server) saveStatus(w http.ResponseWriter, r *http.Request) {
	f := statusFormView{Old: r.FormValue("old"), Code: strings.TrimSpace(r.FormValue("code")), Name: r.FormValue("name"),
		Color: r.FormValue("color"), Custom: r.FormValue("custom"), Watch: r.FormValue("watch") != ""}
	st := issues.Status{Code: f.Code, Name: strings.TrimSpace(f.Name), Color: f.Color}
	if f.Color == "custom" {
		st.Color = f.Custom
	}
	if !f.Watch {
		off := false
		st.Watch = &off
	}
	if errs := s.changeIssues(r, func(d *configedit.Doc) error { return d.SetStatus(f.Old, st) }); errs != nil {
		f.Errors = errs
		s.render(w, "status-form", s.buildStatusForm(f))
		return
	}
	msg := "Stav " + st.Label() + " uložen"
	if f.Old != "" && f.Old != f.Code {
		if err := s.issueStoreUpdate(func(fl *issues.File) { fl.RenameStatus(f.Old, f.Code) }); err != nil {
			msg += ", ale issue se nepodařilo přeřadit: " + err.Error()
		}
	}
	s.saved(w, r, "issues", msg)
}

func (s *Server) issueStoreUpdate(change func(f *issues.File)) error {
	st, err := s.iss.openStore()
	if err != nil {
		return err
	}
	return st.Update(func(f *issues.File, _ time.Time) error { change(f); return nil })
}

func (s *Server) moveStatus(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	delta, _ := strconv.Atoi(r.FormValue("delta"))
	if errs := s.changeIssues(r, func(d *configedit.Doc) error { return d.MoveStatus(code, delta) }); errs != nil {
		s.fail(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	s.saved(w, r, "issues", "Pořadí stavů uloženo")
}

func (s *Server) deleteStatus(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if errs := s.changeIssues(r, func(d *configedit.Doc) error { return d.DeleteStatus(code) }); errs != nil {
		s.fail(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	msg := "Stav " + code + " smazán"
	if err := s.issueStoreUpdate(func(f *issues.File) { f.DropStatus(code) }); err != nil {
		msg += ", ale jeho issue se nepodařilo vrátit mezi nová: " + err.Error()
	}
	s.saved(w, r, "issues", msg)
}

// saveToken checks the token with GitLab and saves it; an empty one with
// remove set deletes the saved token.
func (s *Server) saveToken(w http.ResponseWriter, r *http.Request) {
	home, err := config.HomeDir()
	if err != nil {
		s.fail(w, err)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	if r.FormValue("remove") != "" {
		if err := gitlab.SaveToken(home, ""); err != nil {
			s.fail(w, err)
			return
		}
		s.saved(w, r, "issues", "Token GitLabu smazán")
		return
	}
	if token == "" {
		s.issueSettingsFailed(w, r, "token", []string{"vlož token"})
		return
	}
	icfg, _ := s.iss.config()
	cfg, _, _ := s.config()
	host := ""
	if icfg != nil {
		host = icfg.GitLabHost(cfg)
	}
	msg := "Token uložen"
	if host != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		u, err := issues.Dial(host, token).CurrentUser(ctx)
		switch {
		case errors.Is(err, gitlab.ErrUnauthorized) || errors.Is(err, gitlab.ErrForbidden):
			s.issueSettingsFailed(w, r, "token", []string{err.Error()})
			return
		case err != nil:
			msg = "Token uložen, ale GitLab se nepodařilo ověřit: " + err.Error()
		default:
			msg = "Token uložen – patří uživateli @" + u.Username
		}
	}
	if err := gitlab.SaveToken(home, token); err != nil {
		s.issueSettingsFailed(w, r, "token", []string{err.Error()})
		return
	}
	s.saved(w, r, "issues", msg)
}

func (s *Server) saveIssuesYAML(w http.ResponseWriter, r *http.Request) {
	text := strings.ReplaceAll(r.FormValue("yaml"), "\r\n", "\n")
	if _, err := issues.ParseConfig([]byte(text)); err != nil {
		var p config.Problems
		if !errors.As(err, &p) {
			p = config.Problems{err.Error()}
		}
		q := s.req(w, r)
		d := s.buildSettings(q, "issues")
		d.Issues.YAML, d.Issues.Problems, d.Issues.Part = text, p, "yaml"
		s.render(w, "settings-content", d)
		return
	}
	if r.FormValue("action") == "check" {
		q := s.req(w, r)
		d := s.buildSettings(q, "issues")
		d.Issues.YAML, d.Issues.Part, d.Issues.Message = text, "yaml", "issues.yaml je v pořádku (zatím neuloženo)"
		s.render(w, "settings-content", d)
		return
	}
	if err := configedit.WriteFile(s.iss.path, []byte(text), r.FormValue("ihash")); err != nil {
		s.issueSettingsFailed(w, r, "yaml", []string{err.Error()})
		return
	}
	s.saved(w, r, "issues", "issues.yaml uložen, předchozí verze je v "+s.iss.path+".bak")
}
