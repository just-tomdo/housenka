package web

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/issues"
	"github.com/just-tomdo/housenka/internal/stats"
	"github.com/just-tomdo/housenka/internal/text"
)

// The issue overview is an optional extension: it exists while issues.yaml
// does. The page has three tabs (new issues, the board of the user's
// statuses, hidden issues) and the same keys as the TUI.

// issueSide is the server's part of the overview.
type issueSide struct {
	path, dir string // issues.yaml, the directory of issues.json
	file      issues.ConfigFile

	mu    sync.Mutex
	store *issues.Store

	syncing sync.Mutex // one sync at a time
	lastTry time.Time  // the last automatic or manual sync (guarded by mu)
}

func newIssueSide() *issueSide {
	path, _ := issues.ConfigPath()
	dir, _ := issues.DataDir()
	return &issueSide{path: path, dir: dir, file: issues.ConfigFile{Path: path}}
}

// config returns issues.yaml; nil and no error when the extension is off.
func (s *issueSide) config() (*issues.Config, error) { return s.file.Get() }

func (s *issueSide) openStore() (*issues.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		st, err := issues.OpenStore(s.dir)
		if err != nil {
			return nil, err
		}
		s.store = st
	}
	return s.store, nil
}

func (s *Server) issueRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /issues", s.issuesPage)
	mux.HandleFunc("GET /issues/view", s.issuesView)
	mux.HandleFunc("GET /issues/detail", s.issueDetail)
	mux.HandleFunc("POST /issues/status", s.issueStatus)
	mux.HandleFunc("POST /issues/move", s.issueMove)
	mux.HandleFunc("POST /issues/hidden", s.issueHidden)
	mux.HandleFunc("POST /issues/muted", s.issueMuted)
	mux.HandleFunc("POST /issues/notes", s.issueNotes)
	mux.HandleFunc("POST /issues/sync", s.issueSync)
}

// issuesState is what the page shows. It travels in the URL (tab, q, sort,
// lanes) and, for changing requests, in the hidden #viewstate form with
// the prefix v_.
type issuesState struct {
	Tab   string // "new", "board", "hidden"; "" = new when there are any, else board
	Q     string
	Sort  issues.Sort
	Lanes bool // the board grouped by repository
}

func readIssuesState(get func(string) string, prefix string) issuesState {
	v := issuesState{Q: strings.TrimSpace(get(prefix + "q")), Sort: issues.ParseSort(get(prefix + "sort")), Lanes: get(prefix+"lanes") != ""}
	switch t := get(prefix + "tab"); t {
	case "new", "board", "hidden":
		v.Tab = t
	}
	return v
}

// Query returns the URL query of the state.
func (v issuesState) Query() string {
	q := url.Values{}
	if v.Tab != "" {
		q.Set("tab", v.Tab)
	}
	if v.Q != "" {
		q.Set("q", v.Q)
	}
	if v.Sort != issues.SortManual {
		q.Set("sort", string(v.Sort))
	}
	if v.Lanes {
		q.Set("lanes", "1")
	}
	return q.Encode()
}

func (v issuesState) WithTab(tab string) issuesState { v.Tab = tab; return v }

func (v issuesState) WithoutQ() issuesState { v.Q = ""; return v }

func (v issuesState) WithoutSort() issuesState { v.Sort = issues.SortManual; return v }

func (v issuesState) ToggleLanes() issuesState { v.Lanes = !v.Lanes; return v }

// fromIssues reports whether a changing request was sent from the issue page.
func fromIssues(r *http.Request) bool {
	return r.FormValue("v_tab") != ""
}

// issueReq is a request of the issue page with the extension's parts.
type issueReq struct {
	*req
	icfg  *issues.Config
	store *issues.Store
}

// issueReq answers 404 when the extension is off and an error toast when
// issues.yaml is broken (page requests show the error instead).
func (s *Server) issueReq(w http.ResponseWriter, r *http.Request, page bool) (*issueReq, bool) {
	q := s.req(w, r)
	icfg, err := s.iss.config()
	if icfg == nil && err == nil {
		http.NotFound(w, r)
		return nil, false
	}
	iq := &issueReq{req: q, icfg: icfg}
	if err != nil {
		if !page {
			s.fail(w, err)
			return nil, false
		}
		return iq, true
	}
	if iq.store, err = s.iss.openStore(); err != nil {
		s.fail(w, err)
		return nil, false
	}
	return iq, true
}

// --- view models -------------------------------------------------------------------

type issuesPageData struct {
	chrome
	Main issuesMain
}

type issuesMain struct {
	OOB      bool
	State    issuesState
	Err      string
	Sync     syncView
	Tabs     []issuesTab
	Sorts    []sortOption
	Statuses []issueStatusView
	Cards    []cardView // the new or hidden tab
	Columns  []columnView
	Lanes    []laneView
	Shown    int
	Empty    string
	NewCount int // for the badge in the top bar
}

type syncView struct {
	When string // "dnes 9:30"
	Err  string
}

type issuesTab struct {
	Label  string
	Key    string
	Count  int
	Active bool
	State  issuesState
}

type sortOption struct {
	Value, Label string
	Selected     bool
}

type issueStatusView struct {
	Code, Label string
	Class       string
	Style       template.CSS
	Key         string // modal key 1–9
	Current     bool
}

type columnView struct {
	issueStatusView
	Cards []cardView
}

type laneView struct {
	Path  string
	Class string
	Style template.CSS
	Cols  []columnView
}

type cardView struct {
	issues.Card
	RepoName     string
	RepoClass    string
	RepoStyle    template.CSS
	Status       issueStatusView
	AssignedAge  string
	AssignedDate string
	DueLabel     string
	DueKind      string
	NotesFirst   string
	Marks        []issues.Mark
	Prev, Next   string            // neighbour statuses for H and L
	First        string            // the first status (new issues: L puts them there)
	Statuses     []issueStatusView // new issues: where they can go
	Above, Below string            // the cards shown next to it, for J/K (manual order only)
}

// colorOf returns the badge class (named color) or inline style (#rrggbb)
// of a color spec; an empty spec gets the color derived from key.
func colorOf(key, spec string) (string, template.CSS) {
	if strings.HasPrefix(spec, "#") && len(spec) == 7 && !strings.ContainsAny(spec, ";:(){}\"'<>") {
		return "", template.CSS("--pc: " + spec)
	}
	if slices.Contains(config.ColorNames, spec) {
		return "pc-" + spec, ""
	}
	return "pc-" + config.DerivedColorName(key), ""
}

func newIssueStatusView(st issues.Status, i int) issueStatusView {
	cls, style := colorOf(st.Code, st.Color)
	v := issueStatusView{Code: st.Code, Label: st.Label(), Class: cls, Style: style}
	if i < 9 {
		v.Key = strconv.Itoa(i + 1)
	}
	return v
}

func (s *Server) newCardView(q *issueReq, c issues.Card) cardView {
	v := cardView{Card: c, RepoName: c.Issue.Repo(), AssignedAge: issues.AgeLabel(c.Assigned), Marks: c.Marks()}
	v.RepoClass, v.RepoStyle = colorOf(c.Repo.ColorKey, c.Repo.ColorSpec)
	layout := q.cfg.UI.DateLayout()
	v.AssignedDate = layout.Format(civil.Of(c.AssignedAt.Local()))
	if c.Due != nil {
		v.DueLabel, v.DueKind = layout.Format(*c.Due), issues.DueKind(*c.Due, q.today)
	}
	v.NotesFirst, _, _ = strings.Cut(c.Notes, "\n")
	if c.Column >= 0 {
		v.Status = newIssueStatusView(q.icfg.Statuses[c.Column], c.Column)
		v.Prev, v.Next = q.icfg.Around(c.Column)
	}
	v.First = q.icfg.Statuses[0].Code
	if c.Column < 0 && !c.Hidden {
		for i, st := range q.icfg.Statuses {
			v.Statuses = append(v.Statuses, newIssueStatusView(st, i))
		}
	}
	return v
}

func (s *Server) buildIssuesMain(q *issueReq, v issuesState) issuesMain {
	m := issuesMain{State: v}
	if q.icfg == nil {
		_, err := s.iss.config()
		m.Err = "Soubor issues.yaml obsahuje chyby, přehled issues teď nejde zobrazit (oprav ho v Nastavení → Issues): " + text.OneLine(err.Error())
		return m
	}
	f, err := q.store.Load()
	if err != nil {
		m.Err = "Chyba při načítání issue: " + text.OneLine(err.Error())
		return m
	}
	entries, err := s.store.All()
	if err != nil {
		m.Err = "Chyba při načítání záznamů: " + text.OneLine(err.Error())
	}
	o := issues.BuildOverview(f, q.icfg, q.rules, entries, q.today, v.Q, v.Sort, false)
	all, b := o.All, o.Shown
	if v.Tab == "" {
		v.Tab = issues.DefaultTab(all)
		m.State = v
	}
	m.Sync = syncView{When: issues.SyncedLabel(f, q.today), Err: f.SyncError}
	m.NewCount = len(all.New)
	for _, t := range issues.Tabs {
		m.Tabs = append(m.Tabs, issuesTab{Label: t.Label, Key: t.Key, Count: t.Count(all), State: v.WithTab(t.ID), Active: t.ID == v.Tab})
	}
	for _, so := range issues.Sorts {
		m.Sorts = append(m.Sorts, sortOption{Value: string(so), Label: so.Label(), Selected: so == v.Sort})
	}
	for i, st := range q.icfg.Statuses {
		m.Statuses = append(m.Statuses, newIssueStatusView(st, i))
	}

	views := func(cs []issues.Card) []cardView {
		out := make([]cardView, 0, len(cs))
		for _, c := range cs {
			out = append(out, s.newCardView(q, c))
		}
		return out
	}
	columns := func(b issues.Board) []columnView {
		var out []columnView
		for i, col := range b.Columns {
			cv := columnView{issueStatusView: newIssueStatusView(col.Status, i), Cards: views(col.Cards)}
			if v.Sort == issues.SortManual {
				// J/K put the card next to the card shown above or below it
				// (a lane shows the cards of one repository only).
				for j := range cv.Cards {
					cv.Cards[j].Above, _, _ = issues.Shift(col.Cards, cv.Cards[j].Key, -1, false)
					cv.Cards[j].Below, _, _ = issues.Shift(col.Cards, cv.Cards[j].Key, 1, false)
				}
			}
			out = append(out, cv)
		}
		return out
	}
	switch v.Tab {
	case "new":
		m.Cards, m.Shown = views(b.New), len(b.New)
	case "hidden":
		m.Cards, m.Shown = views(b.Hidden), len(b.Hidden)
	default:
		var boardCards []issues.Card
		for _, col := range b.Columns {
			boardCards = append(boardCards, col.Cards...)
		}
		m.Shown = len(boardCards)
		if v.Lanes {
			for _, lane := range issues.Lanes(boardCards) {
				lv := laneView{Path: lane.ProjectPath, Cols: columns(issues.Arrange(lane.Cards, q.icfg))}
				r := issues.Repo(lane.Cards[0].Issue, q.rules)
				lv.Class, lv.Style = colorOf(r.ColorKey, r.ColorSpec)
				m.Lanes = append(m.Lanes, lv)
			}
		} else {
			m.Columns = columns(b)
		}
	}
	m.Empty = issues.EmptyText(v.Tab, v.Q)
	return m
}

// --- handlers -------------------------------------------------------------------------

func (s *Server) issuesPage(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, true)
	if !ok {
		return
	}
	d := issuesPageData{chrome: s.chrome(q.req, "issues"), Main: s.buildIssuesMain(q, readIssuesState(r.URL.Query().Get, ""))}
	s.render(w, "issues-page", d)
}

func (s *Server) issuesView(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, true)
	if !ok {
		return
	}
	m := s.buildIssuesMain(q, readIssuesState(r.URL.Query().Get, ""))
	w.Header().Set("HX-Replace-Url", "/issues?"+m.State.Query())
	s.render(w, "issues-main", m)
}

// issuesChanged answers a change: the page is rebuilt (out of band, so the
// modal closes) with a toast.
func (s *Server) issuesChanged(w http.ResponseWriter, r *http.Request, t toastData) {
	q, ok := s.issueReq(w, r, true)
	if !ok {
		return
	}
	m := s.buildIssuesMain(q, readIssuesState(r.FormValue, "v_"))
	m.OOB = true
	nav := chrome{Section: "issues", Issues: true, NewIssues: m.NewCount, NavOOB: true}
	s.render(w, "issues-update", struct {
		Main  issuesMain
		Nav   chrome
		Toast toastData
	}{m, nav, t})
}

type issueDetailView struct {
	cardView
	Statuses  []issueStatusView
	Version   string
	Notice    string
	Notes     string
	FocusNote bool
	FirstSeen string
	RepoLink  string // the repository in GitLab, for adding it to housenka
	Work      issueView
	Worked    bool
}

func (s *Server) buildIssueDetail(q *issueReq, key string) (issueDetailView, error) {
	f, err := q.store.Load()
	if err != nil {
		return issueDetailView{}, err
	}
	var found *issues.Issue
	for i := range f.Issues {
		if f.Issues[i].Key == key {
			found = &f.Issues[i]
		}
	}
	if found == nil {
		return issueDetailView{}, issues.ErrNotFound
	}
	entries, err := s.store.All()
	if err != nil {
		return issueDetailView{}, err
	}
	card := issues.Cards(issues.File{Issues: []issues.Issue{*found}}, q.icfg, q.rules, issues.WorkByIssue(entries), q.today)[0]
	d := issueDetailView{cardView: s.newCardView(q, card), Notes: found.Notes, Version: found.NotesAt.Format(time.RFC3339Nano)}
	d.FirstSeen = q.cfg.UI.DateLayout().Format(civil.Of(found.FirstSeen.Local()))
	for i, st := range q.icfg.Statuses {
		sv := newIssueStatusView(st, i)
		sv.Current = i == card.Column
		d.Statuses = append(d.Statuses, sv)
	}
	if !card.Repo.Known() {
		d.RepoLink = "https://" + found.Host + "/" + found.ProjectPath
	}
	sum := stats.BuildIssueSummary(found.URL, entries, q.cfg)
	d.Work, d.Worked = buildIssue(q.cfg, q.rules, sum, q.today), len(sum.Entries) > 0
	return d, nil
}

func (s *Server) issueDetail(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	d, err := s.buildIssueDetail(q, r.URL.Query().Get("key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	d.FocusNote = r.URL.Query().Get("notes") != ""
	s.render(w, "issue-detail", d)
}

func (s *Server) issueStatus(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	key, status := r.FormValue("key"), r.FormValue("status")
	st, known := q.icfg.Status(status)
	if !known {
		s.fail(w, errors.New("neznámý stav "+status+" – načti stránku znovu"))
		return
	}
	f, err := q.store.Load()
	if err != nil {
		s.fail(w, err)
		return
	}
	same := slices.ContainsFunc(f.Issues, func(is issues.Issue) bool { return is.Key == key && is.Status == status && !is.Hidden })
	reorder := same && (r.FormValue("before") != "" || r.FormValue("end") != "") // a drop within the column
	switch {
	case same && !reorder: // the current status chosen again
		s.issuesChanged(w, r, toastData{})
		return
	case reorder && !manualOrder(r):
		s.fail(w, errReorder) // it would scramble the manual order hidden behind the sort
		return
	}
	if err := q.store.SetStatus(key, status, r.FormValue("before")); err != nil {
		s.fail(w, err)
		return
	}
	t := toastData{Text: issueLabel(key) + " → " + st.Label()}
	if same {
		t = toastData{}
	}
	s.issuesChanged(w, r, t)
}

var errReorder = errors.New("pořadí jde měnit jen ve vlastním řazení (O)")

// manualOrder reports whether the page that sent a change shows the manual order.
func manualOrder(r *http.Request) bool {
	return readIssuesState(r.FormValue, "v_").Sort == issues.SortManual
}

// issueMove puts the issue next to the card shown above (after=0) or below
// it (after=1).
func (s *Server) issueMove(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	if !manualOrder(r) {
		s.fail(w, errReorder)
		return
	}
	if err := q.store.MoveNextTo(r.FormValue("key"), r.FormValue("next"), r.FormValue("after") == "1"); err != nil {
		s.fail(w, err)
		return
	}
	s.issuesChanged(w, r, toastData{})
}

func (s *Server) issueHidden(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	key, hidden := r.FormValue("key"), r.FormValue("hidden") == "1"
	if err := q.store.SetHidden(key, hidden); err != nil {
		s.fail(w, err)
		return
	}
	t := toastData{Text: issueLabel(key) + " skryto"}
	if !hidden {
		t.Text = issueLabel(key) + " je zpět"
	}
	s.issuesChanged(w, r, t)
}

func (s *Server) issueMuted(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	key, muted := r.FormValue("key"), r.FormValue("muted") == "1"
	if err := q.store.SetMuted(key, muted); err != nil {
		s.fail(w, err)
		return
	}
	t := toastData{Text: issueLabel(key) + " odloženo – zašedlé na konci sloupce"}
	if !muted {
		t.Text = issueLabel(key) + " už není odložené"
	}
	s.issuesChanged(w, r, t)
}

func (s *Server) issueNotes(w http.ResponseWriter, r *http.Request) {
	q, ok := s.issueReq(w, r, false)
	if !ok {
		return
	}
	key, notes := r.FormValue("key"), r.FormValue("notes")
	base, _ := time.Parse(time.RFC3339Nano, r.FormValue("version"))
	_, err := q.store.SetNotes(key, notes, base)
	if errors.Is(err, issues.ErrConflict) {
		// Keep what was typed on top of the newer version, like the entry form.
		d, derr := s.buildIssueDetail(q, key)
		if derr != nil {
			s.fail(w, derr)
			return
		}
		d.Notes, d.FocusNote = notes, true
		d.Notice = "Poznámky se mezitím změnily v jiném okně. Pole drží tvůj text – Uložit ho zapíše přes novější verzi, Zrušit ji ponechá."
		s.render(w, "issue-detail", d)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.issuesChanged(w, r, toastData{Text: "Poznámky k " + issueLabel(key) + " uloženy"})
}

func (s *Server) issueSync(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.issueReq(w, r, false); !ok {
		return
	}
	res, err := s.runSync(r.Context())
	if err != nil {
		s.issuesChanged(w, r, toastData{Text: "GitLab: " + text.OneLine(err.Error()), Err: true})
		return
	}
	s.issuesChanged(w, r, toastData{Text: "Načteno z GitLabu: " + res.String()})
}

// issueLabel shortens a key for messages: "shop-web#12".
func issueLabel(key string) string {
	return key[strings.LastIndex(key, "/")+1:]
}

// --- sync -------------------------------------------------------------------------------

// runSync reads GitLab once.
func (s *Server) runSync(ctx context.Context) (issues.SyncResult, error) {
	if !s.iss.syncing.TryLock() {
		return issues.SyncResult{}, issues.ErrSyncRunning
	}
	defer s.iss.syncing.Unlock()
	s.iss.mu.Lock()
	s.iss.lastTry = time.Now()
	s.iss.mu.Unlock()

	icfg, err := s.iss.config()
	if err != nil {
		return issues.SyncResult{}, err
	}
	if icfg == nil {
		return issues.SyncResult{}, errors.New("přehled issues je vypnutý")
	}
	st, err := s.iss.openStore()
	if err != nil {
		return issues.SyncResult{}, err
	}
	main, _, _ := s.config()
	src, err := issues.Connect(icfg, main)
	if err != nil {
		return issues.SyncResult{}, issues.RecordSyncError(st, err)
	}
	ctx, cancel := context.WithTimeout(ctx, issues.SyncTimeout)
	defer cancel()
	return issues.Sync(ctx, src, st)
}

// syncCheckEvery is how often the server checks whether a sync is due.
var syncCheckEvery = time.Minute

// keepIssuesSynced reads GitLab every sync_minutes while the overview is on.
func (s *Server) keepIssuesSynced(ctx context.Context) {
	t := time.NewTicker(syncCheckEvery)
	defer t.Stop()
	for {
		s.syncIfDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) syncIfDue(ctx context.Context) {
	icfg, _ := s.iss.config()
	if icfg == nil {
		return
	}
	st, err := s.iss.openStore()
	if err != nil {
		return
	}
	f, err := st.Load()
	if err != nil {
		return
	}
	s.iss.mu.Lock()
	last := s.iss.lastTry
	s.iss.mu.Unlock()
	if issues.SyncDue(f, icfg, last, time.Now()) {
		s.runSync(ctx)
	}
}
