package web

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/entryedit"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/stats"
)

// viewState is what the page shows: a period around a date, and a filter.
// It travels in the URL (kind, date, q) and, for changing requests, in the
// hidden #viewstate form (v_kind, v_date, v_q).
type viewState struct {
	Kind period.Kind
	Date civil.Date
	Q    string
}

var kindNames = map[period.Kind]string{period.Day: "day", period.Week: "week", period.Month: "month"}

func parseKind(s string) (period.Kind, bool) {
	for k, name := range kindNames {
		if name == s {
			return k, true
		}
	}
	return period.Day, false
}

// readViewState reads the view from the given parameter names; missing or
// invalid values fall back to today's day.
func readViewState(get func(string) string, prefix string, today civil.Date) viewState {
	v := viewState{Kind: period.Day, Date: today, Q: strings.TrimSpace(get(prefix + "q"))}
	if k, ok := parseKind(get(prefix + "kind")); ok {
		v.Kind = k
	}
	if d, err := civil.Parse(get(prefix + "date")); err == nil {
		v.Date = d
	}
	return v
}

// Query returns the URL query of the view, without the filter when empty.
func (v viewState) Query() string {
	q := url.Values{"kind": {kindNames[v.Kind]}, "date": {v.Date.String()}}
	if v.Q != "" {
		q.Set("q", v.Q)
	}
	return q.Encode()
}

func (v viewState) with(k period.Kind, d civil.Date) viewState {
	v.Kind, v.Date = k, d
	return v
}

func (v viewState) KindName() string { return kindNames[v.Kind] }

func (v viewState) Period() period.Period { return period.Of(v.Kind, v.Date) }

// --- main view ---------------------------------------------------------------

type mainView struct {
	View        viewState
	Today       civil.Date
	PeriodLabel string
	Current     string // "dnes" / "tento týden" / "tento měsíc"
	IsCurrent   bool
	TowardsPast bool // today lies before the shown period
	Tabs        []tab
	Prev, Next  viewState
	ToToday     viewState
	NewDate     civil.Date // date of a new entry: the shown day, today, or the first day
	Header      headerView
	Groups      []dayGroup
	Count       int // entries after the filter
	Unsent      int
	LoadErr     error
	ConfigErr   error
	Caterpillar bool
}

type tab struct {
	Label  string
	Key    string
	View   viewState
	Active bool
}

type headerView struct {
	stats.Totals
	Goal        int
	Over        bool // a day over the daily warning
	Warning     int
	Reached     bool
	ShowDays    bool
	WorkedDays  int
	Avg         string
	ProdPct     int
	Bar         bar
	WeekDays    []dayChip
	GoalReached bool
}

// bar holds widths in percent of max(goal, total).
type bar struct {
	Productive, Other float64
}

// Filled is the width of both parts together.
func (b bar) Filled() float64 { return b.Productive + b.Other }

type dayChip struct {
	Label string
	Day   stats.Day
	Today bool
	Bar   bar
	View  viewState
}

type dayGroup struct {
	Show    bool // day header row (week and month)
	Day     stats.Day
	Label   string
	Today   bool
	Bar     bar
	ProdPct int
	Entries []entryView
	NewView viewState
}

type entryView struct {
	entry.Entry
	State        string // "sent", "pending", "invalid"
	Group, Repo  string
	IssueLabel   string
	IssueLink    string // the link without the #note fragment
	BadLink      bool
	ProjectName  string
	ProjectClass string
	ProjectStyle template.CSS
	ActivityName string
	ActivityKind string // "good", "bad", "err"
	Errors       []string
	BMURL        string
	BMErr        string
	DateLabel    string
	Today        civil.Date
}

func barOf(t stats.Totals, goal int) bar {
	scale := float64(max(goal, t.Total, 1))
	return bar{
		Productive: float64(t.Productive) * 100 / scale,
		Other:      float64(t.Unproductive+t.Unknown) * 100 / scale,
	}
}

var currentLabel = map[period.Kind]string{period.Day: "dnes", period.Week: "tento týden", period.Month: "tento měsíc"}

var tabLabels = map[period.Kind]string{period.Day: "Den", period.Week: "Týden", period.Month: "Měsíc"}

// buildMain loads the period of v and prepares everything the header and
// the list show.
func (s *Server) buildMain(cfg *config.Config, r *rules.Rules, cfgErr error, v viewState, today civil.Date) mainView {
	layout := cfg.UI.DateLayout()
	per := v.Period()
	m := mainView{
		View: v, Today: today, PeriodLabel: per.Label(layout), Current: currentLabel[v.Kind],
		IsCurrent: per.Contains(today), TowardsPast: per.Start.After(today),
		Prev: v.with(v.Kind, per.Prev().Start), Next: v.with(v.Kind, per.Next().Start),
		ToToday: v.with(v.Kind, today), ConfigErr: cfgErr, Caterpillar: cfg.UI.Caterpillar,
		NewDate: per.Start,
	}
	if per.Contains(today) {
		m.NewDate = today
	}
	for i, k := range []period.Kind{period.Day, period.Week, period.Month} {
		d := v.Date // switching the period keeps today when it is shown
		if per.Contains(today) {
			d = today
		}
		m.Tabs = append(m.Tabs, tab{Label: tabLabels[k], Key: fmt.Sprint(i + 1), View: v.with(k, d), Active: k == v.Kind})
	}

	all, err := s.store.Range(per.Start, per.End())
	m.LoadErr = err
	sum := stats.Summarize(per, all, cfg)
	goal := entryedit.PeriodGoal(cfg, per)
	h := headerView{
		Totals: sum.Totals, Goal: goal, Warning: int(cfg.Time.DailyWarning),
		Over:     v.Kind == period.Day && sum.Total > int(cfg.Time.DailyWarning),
		ShowDays: v.Kind != period.Day, WorkedDays: sum.WorkedDays,
		Avg:     strings.Replace(fmt.Sprintf("%.2f", sum.AvgHoursPerWorkedDay()), ".", ",", 1),
		ProdPct: sum.ProductivePercent(), Bar: barOf(sum.Totals, goal),
		Reached: goal > 0 && sum.Total >= goal,
	}
	h.GoalReached = v.Kind == period.Day && sum.Total >= int(cfg.Time.DailyGoal)
	if v.Kind == period.Week {
		for _, d := range sum.Days {
			wd := d.Date.Weekday()
			if (wd == time.Saturday || wd == time.Sunday) && d.Total == 0 {
				continue
			}
			h.WeekDays = append(h.WeekDays, dayChip{
				Label: period.WeekdayShort(wd), Day: d, Today: d.Date == today,
				Bar: barOf(d.Totals, int(cfg.Time.DailyGoal)), View: v.with(period.Day, d.Date),
			})
		}
	}
	m.Header = h

	days := map[civil.Date]stats.Day{}
	for _, d := range sum.Days {
		days[d.Date] = d
	}
	for _, e := range all {
		if !e.Sent {
			m.Unsent++
		}
		if !entryedit.Matches(e, v.Q) {
			continue
		}
		m.Count++
		if n := len(m.Groups); n == 0 || m.Groups[n-1].Day.Date != e.Date {
			d := days[e.Date]
			m.Groups = append(m.Groups, dayGroup{
				Show: v.Kind != period.Day, Day: d, Today: d.Date == today,
				Label: period.WeekdayShort(d.Date.Weekday()) + " " + layout.Format(d.Date),
				Bar:   barOf(d.Totals, int(cfg.Time.DailyGoal)), ProdPct: d.ProductivePercent(),
				NewView: v.with(period.Day, d.Date),
			})
		}
		g := &m.Groups[len(m.Groups)-1]
		g.Entries = append(g.Entries, newEntryView(cfg, r, e, today))
	}
	return m
}

func newEntryView(cfg *config.Config, r *rules.Rules, e entry.Entry, today civil.Date) entryView {
	res := r.Validate(e, today)
	ev := entryView{Entry: e, State: "pending", Repo: "bez issue", Today: today}
	ev.DateLabel = period.WeekdayShort(e.Date.Weekday()) + " " + cfg.UI.DateLayout().Format(e.Date)
	switch {
	case !res.Valid():
		ev.State = "invalid"
	case e.Sent:
		ev.State = "sent"
	}
	if ref, err := gitlaburl.Parse(e.IssueURL); err == nil {
		ev.Group, ev.Repo, ev.IssueLabel = ref.Group, ref.Repo, issuePrefix(ref)+fmt.Sprint(ref.IID)
		ev.IssueLink, _, _ = strings.Cut(e.IssueURL, "#")
	} else if e.IssueURL != "" {
		ev.Repo, ev.BadLink = "neplatný odkaz", true
	}
	ev.ProjectClass, ev.ProjectStyle = projectColor(cfg, e.ProjectCode)
	if p, ok := cfg.Projects[e.ProjectCode]; ok {
		ev.ProjectName = p.Name
	}
	ev.ActivityKind = "err"
	if a, ok := cfg.Activities[e.Activity]; ok {
		ev.ActivityName = a.Name
		ev.ActivityKind = "good"
		if !a.IsProductive() {
			ev.ActivityKind = "bad"
		}
	}
	for _, f := range res.Errors() {
		ev.Errors = append(ev.Errors, oneLine(f.Message))
	}
	if u, err := entryedit.BMURL(cfg, res, e); err == nil {
		ev.BMURL = u
	} else {
		ev.BMErr = oneLine(err.Error())
	}
	return ev
}

func issuePrefix(ref gitlaburl.Ref) string {
	if ref.Kind == gitlaburl.MergeRequest {
		return "!"
	}
	return "#"
}

// projectColor returns the badge class (named color) or inline style
// (#rrggbb) of a project.
func projectColor(cfg *config.Config, code string) (string, template.CSS) {
	spec := cfg.Projects[code].Color
	if strings.HasPrefix(spec, "#") && len(spec) == 7 && !strings.ContainsAny(spec, ";:(){}\"'<>") {
		return "", template.CSS("--pc: " + spec)
	}
	for _, n := range config.ColorNames {
		if n == spec {
			return "pc-" + n, ""
		}
	}
	return "pc-" + config.DerivedColorName(code), ""
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- form ----------------------------------------------------------------------

type formView struct {
	Title    string
	Action   string
	In       entryedit.Input
	Errs     map[string]string
	Warns    map[string]string
	Choices  choicesView
	Hints    hintsView
	Today    civil.Date
	DateISO  string // the typed date for the date picker, "" when invalid
	Example  string // date placeholder
	Sent     bool
	Resubmit bool // editing a sent entry
}

type option struct {
	Code, Name string
	Class      string
	Style      template.CSS
	Kind       string // activities: "good" / "bad"
	Selected   bool
}

type choicesView struct {
	Projects     []option
	Activities   []option
	ProjectHint  string
	ActivityHint hint
	NoProjects   string
	Errs, Warns  map[string]string
}

type hint struct {
	Text  string
	Class string // "", "good", "bad", "err", "warn"
}

// hintAt is a hint placed under a field (element ID), optionally swapped out of band.
type hintAt struct {
	hint
	ID  string
	OOB bool
}

func (h hint) WithID(id string) hintAt { return hintAt{hint: h, ID: id} }

func (h hintAt) AsOOB() hintAt {
	h.OOB = true
	return h
}

type hintsView struct {
	Date, Issue, Hours hint
}

func fieldMessages(m entryedit.Messages) map[string]string {
	out := map[string]string{}
	for f, msg := range m {
		out[string(f)] = oneLine(msg)
	}
	return out
}

// buildForm prepares the entry form. errs and warns come from a failed
// submit; live hints are shown for fields without a message.
func buildForm(cfg *config.Config, r *rules.Rules, in entryedit.Input, today civil.Date, errs, warns entryedit.Messages) formView {
	layout := cfg.UI.DateLayout()
	c := entryedit.Options(r, in.Issue, in.Project, in.Activity)
	in.Project, in.Activity = c.Project, c.Activity
	f := formView{
		In: in, Errs: fieldMessages(errs), Warns: fieldMessages(warns), Today: today,
		Example: layout.Format(today) + " · dnes · včera · -1",
	}
	if d, err := civil.ParseHuman(in.Date, today, layout); err == nil {
		f.DateISO = d.String()
	}
	f.Choices = buildChoices(cfg, c, f.Errs, f.Warns)
	f.Hints = liveHints(cfg, r, in, c, today)
	return f
}

// Hint returns the message under a field: an error, a warning, or the live hint.
func (f formView) Hint(field string) hint {
	return pickHint(f.Errs, f.Warns, field, map[string]hint{
		"date": f.Hints.Date, "issue": f.Hints.Issue, "hours": f.Hints.Hours,
	}[field])
}

// Hint returns the message under the project or activity choice.
func (c choicesView) Hint(field string) hint {
	live := hint{Text: c.ProjectHint}
	if field == "activity" {
		live = c.ActivityHint
	}
	return pickHint(c.Errs, c.Warns, field, live)
}

func pickHint(errs, warns map[string]string, field string, live hint) hint {
	if msg, ok := errs[field]; ok {
		return hint{"✗ " + msg, "err"}
	}
	if msg, ok := warns[field]; ok {
		return hint{"! " + msg, "warn"}
	}
	return live
}

func buildChoices(cfg *config.Config, c entryedit.Choices, errs, warns map[string]string) choicesView {
	v := choicesView{Errs: errs, Warns: warns}
	for _, code := range c.Projects {
		cls, style := projectColor(cfg, code)
		v.Projects = append(v.Projects, option{Code: code, Name: cfg.Projects[code].Name, Class: cls, Style: style, Selected: code == c.Project})
	}
	if len(c.Projects) == 0 {
		v.NoProjects = "nejdřív vyplň odkaz na issue"
	}
	if p, ok := cfg.Projects[c.Project]; ok {
		v.ProjectHint = p.Name
	}
	for _, code := range c.Activities {
		a := cfg.Activities[code]
		kind := "good"
		if !a.IsProductive() {
			kind = "bad"
		}
		v.Activities = append(v.Activities, option{Code: code, Name: a.Name, Kind: kind, Selected: code == c.Activity})
	}
	if a, ok := cfg.Activities[c.Activity]; ok {
		v.ActivityHint = hint{a.Name + " · produktivní", "good"}
		if !a.IsProductive() {
			v.ActivityHint = hint{a.Name + " · neproduktivní", "bad"}
		}
	}
	return v
}

// liveHints describe the typed date, issue link and hours like the TUI does.
func liveHints(cfg *config.Config, r *rules.Rules, in entryedit.Input, c entryedit.Choices, today civil.Date) hintsView {
	layout := cfg.UI.DateLayout()
	var h hintsView
	if d, err := civil.ParseHuman(in.Date, today, layout); err == nil {
		h.Date = hint{Text: period.WeekdayShort(d.Weekday()) + " " + layout.Format(d)}
	} else if strings.TrimSpace(in.Date) != "" {
		h.Date = hint{err.Error(), "err"}
	}
	switch {
	case c.IssueRef != nil && c.IssueErr == nil:
		h.Issue = hint{"✓ " + c.IssueRef.Short(), "good"}
	case c.IssueErr != nil && strings.TrimSpace(in.Issue) != "":
		h.Issue = hint{oneLine(c.IssueErr.Error()), "err"}
	case c.IssueErr == nil:
		h.Issue = hint{Text: "bez issue – jen projekty s issue: optional"}
	default:
		h.Issue = hint{oneLine(c.IssueErr.Error()), "err"}
	}
	if q, err := hours.Parse(in.Hours); err == nil && q > 0 {
		h.Hours = hint{Text: fmt.Sprintf("= %d h %02d min", q/4, q%4*15)}
		if q > int(cfg.Time.MaxEntry) {
			h.Hours = hint{fmt.Sprintf("víc než %s h na jeden záznam", hours.Format(int(cfg.Time.MaxEntry))), "err"}
		}
	} else if err != nil && strings.TrimSpace(in.Hours) != "" {
		h.Hours = hint{err.Error(), "err"}
	}
	return h
}

func inputOf(e entry.Entry, layout civil.Layout) entryedit.Input {
	in := entryedit.Input{Issue: e.IssueURL, Project: e.ProjectCode, Activity: e.Activity, Description: e.Description}
	if e.Date.Valid() {
		in.Date = layout.Format(e.Date)
	}
	if e.Quarters > 0 {
		in.Hours = hours.Format(e.Quarters)
	}
	return in
}

// --- issue summary and status ------------------------------------------------------

type issueView struct {
	stats.IssueSummary
	Span       string
	Activities []shareView
	Projects   []shareView
	Entries    []entryView
}

type shareView struct {
	stats.IssueShare
	Pct   int
	Class string
	Style template.CSS
	Kind  string
}

func buildIssue(cfg *config.Config, r *rules.Rules, s stats.IssueSummary, today civil.Date) issueView {
	layout := cfg.UI.DateLayout()
	v := issueView{IssueSummary: s}
	if s.Days > 0 {
		v.Span = layout.Format(s.First)
		if s.Last != s.First {
			v.Span = layout.FormatRange(s.First, s.Last)
		}
	}
	pct := func(q int) int {
		if s.Total == 0 {
			return 0
		}
		return q * 100 / s.Total
	}
	for _, a := range s.Activities {
		kind := "good"
		switch {
		case !a.Known:
			kind = "err"
		case !a.Productive:
			kind = "bad"
		}
		v.Activities = append(v.Activities, shareView{IssueShare: a, Pct: pct(a.Total), Kind: kind})
	}
	for _, p := range s.Projects {
		cls, style := projectColor(cfg, p.Code)
		v.Projects = append(v.Projects, shareView{IssueShare: p, Pct: pct(p.Total), Class: cls, Style: style})
	}
	for _, e := range s.Entries {
		v.Entries = append(v.Entries, newEntryView(cfg, r, e, today))
	}
	return v
}

type statusView struct {
	stats.Status
	Label      string
	View       viewState
	Prev, Next viewState
	Text, MD   string
	Projects   []statusProject
}

type statusProject struct {
	stats.StatusProject
	Class string
	Style template.CSS
}

func buildStatus(cfg *config.Config, st stats.Status, v viewState) statusView {
	sv := statusView{Status: st, Label: st.Period.Label(st.Layout), View: v, Text: st.Text(), MD: st.Markdown()}
	sv.Prev = v.with(period.Week, st.Period.Prev().Start)
	sv.Next = v.with(period.Week, st.Period.Next().Start)
	for _, p := range st.Projects {
		cls, style := projectColor(cfg, p.Code)
		sv.Projects = append(sv.Projects, statusProject{p, cls, style})
	}
	return sv
}

// --- template functions ----------------------------------------------------------

var funcs = template.FuncMap{
	"hours": hours.Format,
	"pct":   func(f float64) string { return fmt.Sprintf("%.2f%%", f) },
	"lines": func(s string) []string { return strings.Split(s, "\n") },
	"first": func(s string) string { l, _, _ := strings.Cut(s, "\n"); return l },
	"rest": func(s string) string {
		_, r, _ := strings.Cut(s, "\n")
		return strings.TrimSpace(r)
	},
	"plural": czPlural,
	"sub":    func(a, b int) int { return a - b },
}

// czPlural returns "1 výkaz", "3 výkazy", "5 výkazů".
func czPlural(n int, one, few, many string) string {
	switch {
	case n == 1:
		return "1 " + one
	case n >= 2 && n <= 4:
		return fmt.Sprintf("%d %s", n, few)
	}
	return fmt.Sprintf("%d %s", n, many)
}
