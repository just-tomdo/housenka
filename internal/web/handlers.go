package web

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/entryedit"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/stats"
)

// req bundles what most handlers need for one request.
type req struct {
	w      http.ResponseWriter
	r      *http.Request
	cfg    *config.Config
	rules  *rules.Rules
	cfgErr error
	today  civil.Date
}

func (s *Server) req(w http.ResponseWriter, r *http.Request) *req {
	cfg, ru, err := s.config()
	return &req{w: w, r: r, cfg: cfg, rules: ru, cfgErr: err, today: s.clock()}
}

// viewFromQuery reads the view from the URL (kind, date, q).
func (q *req) viewFromQuery() viewState {
	return readViewState(q.r.URL.Query().Get, "", q.today)
}

// viewFromState reads the view a changing request was sent from (v_kind, v_date, v_q).
func (q *req) viewFromState() viewState {
	return readViewState(q.r.FormValue, "v_", q.today)
}

const themeCookie = "housenka_theme"

type pageData struct {
	Theme  string
	Themes []themeOption
	CSRF   string
	Main   mainData
}

type themeOption struct {
	ID, Name string
	Selected bool
}

type mainData struct {
	mainView
	OOB bool
}

type toastData struct {
	Text      string
	Err       bool
	Link      string
	LinkLabel string
}

// theme returns the viewer's theme: the cookie set by the theme picker,
// else web.theme from the config, else auto.
func (s *Server) theme(q *req) string {
	if c, err := q.r.Cookie(themeCookie); err == nil && s.themes.valid(c.Value) {
		return c.Value
	}
	if s.themes.valid(q.cfg.Web.Theme) {
		return q.cfg.Web.Theme
	}
	return themeAuto
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	v := q.viewFromQuery()
	d := pageData{Theme: s.theme(q), CSRF: csrfToken(w, r)}
	d.Themes = append(d.Themes, themeOption{ID: themeAuto, Name: "Podle systému", Selected: d.Theme == themeAuto})
	for _, t := range s.themes.list {
		d.Themes = append(d.Themes, themeOption{ID: t.ID, Name: t.Name, Selected: d.Theme == t.ID})
	}
	d.Main = mainData{mainView: s.buildMain(q.cfg, q.rules, q.cfgErr, v, q.today)}
	s.render(w, "page", d)
}

// view renders the main area (header and list) for navigation and the filter.
func (s *Server) view(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	v := q.viewFromQuery()
	if r.Header.Get("HX-Trigger") == "filter" {
		w.Header().Set("HX-Replace-Url", "/?"+v.Query())
	}
	s.render(w, "main", mainData{mainView: s.buildMain(q.cfg, q.rules, q.cfgErr, v, q.today)})
}

// find returns the entry with the given ID.
func (s *Server) find(id string) (entry.Entry, error) {
	all, err := s.store.All()
	if err != nil {
		return entry.Entry{}, err
	}
	for _, e := range all {
		if e.ID == id {
			return e, nil
		}
	}
	return entry.Entry{}, errors.New("záznam už neexistuje – načti stránku znovu")
}

// newEntry opens the form for a new entry on ?date=, or for a copy of ?copy=.
func (s *Server) newEntry(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	date, err := civil.Parse(r.URL.Query().Get("date"))
	if err != nil {
		date = q.today
	}
	e, title := entry.Entry{Date: date}, "Nový záznam"
	if id := r.URL.Query().Get("copy"); id != "" {
		orig, err := s.find(id)
		if err != nil {
			s.fail(w, err)
			return
		}
		e, title = entryedit.CopyOf(orig, date), "Kopie záznamu"
	}
	f := buildForm(q.cfg, q.rules, inputOf(e, q.cfg.UI.DateLayout()), q.today, nil, nil)
	f.Title, f.Action = title, "/entries"
	s.render(w, "form", f)
}

func (s *Server) editEntry(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	e, err := s.find(r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	f := buildForm(q.cfg, q.rules, inputOf(e, q.cfg.UI.DateLayout()), q.today, nil, nil)
	f.Title, f.Action, f.Sent = "Úprava záznamu", "/entries/"+e.ID, e.Sent
	s.render(w, "form", f)
}

func formInput(r *http.Request) entryedit.Input {
	return entryedit.Input{
		Date: r.FormValue("date"), Issue: r.FormValue("issue"),
		Project: r.FormValue("project"), Activity: r.FormValue("activity"),
		Hours: r.FormValue("hours"), Description: r.FormValue("description"),
	}
}

// refreshForm recomputes the choices and hints while the form is filled in.
func (s *Server) refreshForm(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	s.render(w, "form-live", buildForm(q.cfg, q.rules, formInput(r), q.today, nil, nil))
}

func (s *Server) saveEntry(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	var base entry.Entry
	if id := r.PathValue("id"); id != "" {
		var err error
		if base, err = s.find(id); err != nil {
			s.fail(w, err)
			return
		}
	}
	in := formInput(r)
	e, errs, warns := entryedit.Parse(q.rules, base, in, q.today)
	if len(errs) > 0 {
		f := buildForm(q.cfg, q.rules, in, q.today, errs, warns)
		f.Title, f.Action, f.Sent = r.FormValue("title"), r.URL.Path, base.Sent
		s.render(w, "form", f)
		return
	}
	e, resent := entryedit.PrepareSave(base, e)
	saved, err := s.store.Save(e)
	if err != nil {
		s.fail(w, err)
		return
	}
	v := q.viewFromState()
	if !v.Period().Contains(saved.Date) {
		v.Date = saved.Date
	}
	t := toastData{Text: "Uloženo ✓"}
	if resent {
		t = toastData{Text: "Uloženo – záznam byl už odeslaný, uprav ho i v BM", Err: true}
	}
	if u, err := entryedit.BMURL(q.cfg, q.rules.Validate(saved, q.today), saved); err == nil {
		t.Link, t.LinkLabel = u, "Otevřít v BM ↗"
	}
	s.update(w, q, v, t, saved.ID)
}

func (s *Server) toggleSent(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	e, err := s.find(r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	e.Sent = !e.Sent
	if _, err := s.store.Save(e); err != nil {
		s.fail(w, err)
		return
	}
	t := toastData{Text: "Označeno jako odeslané do BM"}
	if !e.Sent {
		t.Text = "Označeno jako neodeslané"
	}
	s.update(w, q, q.viewFromState(), t, e.ID)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	s.update(w, q, q.viewFromState(), toastData{Text: "Záznam smazán"}, "")
}

// update answers a change: it closes the modal and swaps in the main area
// and a toast. The browser selects the entry selectID.
func (s *Server) update(w http.ResponseWriter, q *req, v viewState, t toastData, selectID string) {
	if selectID != "" {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"housenka:select": %q}`, selectID))
	}
	w.Header().Set("HX-Push-Url", "/?"+v.Query())
	s.render(w, "update", struct {
		Main  mainData
		Toast toastData
	}{mainData{s.buildMain(q.cfg, q.rules, q.cfgErr, v, q.today), true}, t})
}

func (s *Server) issueSummary(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	link := r.URL.Query().Get("url")
	if strings.TrimSpace(link) == "" {
		s.fail(w, errors.New("záznam nemá odkaz na issue"))
		return
	}
	all, err := s.store.All()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "issue", buildIssue(q.cfg, q.rules, stats.BuildIssueSummary(link, all, q.cfg), q.today))
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	q := s.req(w, r)
	v := q.viewFromQuery()
	p := period.Of(period.Week, v.Date)
	entries, err := s.store.Range(p.Start, p.End())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "status", buildStatus(q.cfg, stats.BuildStatus(p, entries, q.cfg), v))
}

func (s *Server) help(w http.ResponseWriter, _ *http.Request) {
	s.render(w, "help", nil)
}

// fail shows an error toast and leaves the rest of the page as it is.
func (s *Server) fail(w http.ResponseWriter, err error) {
	w.Header().Set("HX-Reswap", "none")
	s.render(w, "toast", struct{ Toast toastData }{toastData{Text: oneLine(err.Error()), Err: true}})
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var b bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&b, name, data); err != nil {
		log.Printf("šablona %s: %v", name, err)
		http.Error(w, "chyba při vykreslení stránky: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}
