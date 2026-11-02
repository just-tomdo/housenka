package tui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/issues"
	"github.com/just-tomdo/housenka/internal/stats"
)

// issuesView is the optional overview of GitLab issues assigned to the user
// (on while issues.yaml exists): new issues, the board of the user's
// statuses and hidden issues. T switches between it and the entries.
type issuesView struct {
	file  issues.ConfigFile
	dir   string // of issues.json
	store *issues.Store

	cfg     *issues.Config
	cfgErr  error
	loadErr error
	data    issues.File
	view    issues.Overview

	tab       string // "new", "board", "hidden"
	sort      issues.Sort
	byRepo    bool // cards of each status grouped by repository
	filter    textinput.Model
	filtering bool
	col       int    // the selected column on the board
	sel       string // the key of the selected issue
	pending   string // first key of gg / gx
	syncing   bool
	detail    *issueDetail
}

// issueDetail is the modal of one issue: status, notes and reported work.
type issueDetail struct {
	key     string
	offset  int
	editing bool
	notes   textarea.Model
	base    time.Time // NotesAt the notes were read with
	work    stats.IssueSummary
}

func newIssuesView() *issuesView {
	path, _ := issues.ConfigPath()
	dir, _ := issues.DataDir()
	f := textinput.New()
	f.Prompt = "/"
	f.Placeholder = "r:repo p:KÓD l:štítek is:stale"
	return &issuesView{file: issues.ConfigFile{Path: path}, dir: dir, filter: f}
}

// issuesEnabled reports whether issues.yaml exists (valid or not).
func (m *Model) issuesEnabled() bool {
	cfg, err := m.iss.file.Get()
	return cfg != nil || err != nil
}

// newIssueCount returns the issues waiting to be sorted, for the entries' status line.
func (m *Model) newIssueCount() int {
	cfg, _ := m.iss.file.Get()
	if cfg == nil {
		return 0
	}
	st, err := m.issueStore()
	if err != nil {
		return 0
	}
	f, err := st.Load()
	if err != nil {
		return 0
	}
	return len(f.New(cfg))
}

func (m *Model) issueStore() (*issues.Store, error) {
	v := m.iss
	if v.store == nil {
		st, err := issues.OpenStore(v.dir)
		if err != nil {
			return nil, err
		}
		v.store = st
	}
	return v.store, nil
}

// openIssues shows the overview and reads GitLab when the last sync is older than sync_minutes.
func (m *Model) openIssues() tea.Cmd {
	if !m.issuesEnabled() {
		return m.notify("Přehled issues je vypnutý – zapneš ho příkazem housenka issues init (nebo v Nastavení webu)", true)
	}
	m.issuesOpen = true
	m.loadIssues()
	v := m.iss
	if v.cfg != nil && time.Since(v.data.SyncedAt) >= time.Duration(v.cfg.SyncMinutes)*time.Minute {
		return m.syncIssues()
	}
	return nil
}

// loadIssues re-reads issues.yaml, issues.json and the entries (all cached
// until the files change) and keeps the selection.
func (m *Model) loadIssues() {
	v := m.iss
	v.cfg, v.cfgErr = v.file.Get()
	v.loadErr = nil
	if v.cfg == nil {
		v.view = issues.Overview{}
		return
	}
	st, err := m.issueStore()
	if err == nil {
		v.data, err = st.Load()
	}
	entries, eerr := m.store.All()
	if v.loadErr = errors.Join(err, eerr); err != nil {
		return
	}
	v.view = issues.BuildOverview(v.data, v.cfg, m.rules, entries, m.today, v.filter.Value(), v.sort, v.byRepo)
	if v.tab == "" {
		v.tab = "board"
		if len(v.view.All.New) > 0 {
			v.tab = "new"
		}
	}
	v.col = min(max(v.col, 0), len(v.cfg.Statuses)-1)
	if v.tab == "board" && v.sel != "" {
		for i, col := range v.view.Shown.Columns {
			if slices.ContainsFunc(col.Cards, func(c issues.Card) bool { return c.Key == v.sel }) {
				v.col = i
			}
		}
	}
	if list := m.issueList(); !slices.ContainsFunc(list, func(c issues.Card) bool { return c.Key == v.sel }) {
		v.sel = ""
		if len(list) > 0 {
			v.sel = list[0].Key
		}
	}
	if d := v.detail; d != nil && !d.editing {
		if _, ok := m.issueByKey(d.key); !ok {
			v.detail = nil
		}
	}
}

// issueList returns the cards the cursor moves through: the selected
// column on the board, else the tab's list.
func (m *Model) issueList() []issues.Card {
	v := m.iss
	switch v.tab {
	case "new":
		return v.view.Shown.New
	case "hidden":
		return v.view.Shown.Hidden
	}
	if v.col < len(v.view.Shown.Columns) {
		return v.view.Shown.Columns[v.col].Cards
	}
	return nil
}

func (m *Model) selectedIssue() (issues.Card, int, bool) {
	list := m.issueList()
	i := slices.IndexFunc(list, func(c issues.Card) bool { return c.Key == m.iss.sel })
	if i < 0 {
		return issues.Card{}, -1, false
	}
	return list[i], i, true
}

func (m *Model) issueByKey(key string) (issues.Card, bool) {
	b := m.iss.view.All
	all := append(slices.Clone(b.New), b.Hidden...)
	for _, col := range b.Columns {
		all = append(all, col.Cards...)
	}
	i := slices.IndexFunc(all, func(c issues.Card) bool { return c.Key == key })
	if i < 0 {
		return issues.Card{}, false
	}
	return all[i], true
}

// --- keys ---------------------------------------------------------------------------

type issuesKeyMap struct {
	Move, Column, Status, Order, Tabs, Detail, Notes, Report, Open, Hide key.Binding
	Filter, Sort, ByRepo, Sync, Back, Help, Quit                         key.Binding
}

var issuesKeys = issuesKeyMap{
	Move:   key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "výběr")),
	Column: key.NewBinding(key.WithKeys("h", "l"), key.WithHelp("h/l", "sloupec")),
	Status: key.NewBinding(key.WithKeys("H", "L"), key.WithHelp("H/L", "předchozí/další stav")),
	Order:  key.NewBinding(key.WithKeys("J", "K"), key.WithHelp("J/K", "pořadí")),
	Tabs:   key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1/2/3", "nová/nástěnka/skrytá")),
	Detail: key.NewBinding(key.WithKeys("enter", "s"), key.WithHelp("enter/s", "detail, stav")),
	Notes:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "poznámky")),
	Report: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "vykázat")),
	Open:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o/gx", "otevřít v GitLabu")),
	Hide:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "skrýt/vrátit")),
	Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filtr")),
	Sort:   key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "řazení")),
	ByRepo: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "podle repozitáře")),
	Sync:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "načíst z GitLabu")),
	Back:   key.NewBinding(key.WithKeys("T", "esc"), key.WithHelp("T/esc", "výkazy")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "nápověda")),
	Quit:   key.NewBinding(key.WithKeys("q"), key.WithHelp("q/:q", "konec")),
}

func (k issuesKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Move, k.Column, k.Status, k.Detail, k.Report, k.Open, k.Hide, k.Back, k.Help}
}

func (k issuesKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Move, k.Column, k.Tabs, k.Filter, k.Sort, k.ByRepo},
		{k.Status, k.Order, k.Detail, k.Notes, k.Report},
		{k.Open, k.Hide, k.Sync, k.Back, k.Help, k.Quit},
	}
}

type issueDetailKeyMap struct {
	Status, Notes, Report, Open, Hide, Scroll, Close, Save, Cancel key.Binding
	editing                                                        bool
}

var issueDetailKeys = issueDetailKeyMap{
	Status: key.NewBinding(key.WithKeys("1"), key.WithHelp("1–9", "stav")),
	Notes:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "upravit poznámky")),
	Report: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "vykázat")),
	Open:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "otevřít v GitLabu")),
	Hide:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "skrýt/vrátit")),
	Scroll: key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "posun")),
	Close:  key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "zavřít")),
	Save:   key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "uložit poznámky")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "zrušit úpravy")),
}

func (k issueDetailKeyMap) ShortHelp() []key.Binding {
	if k.editing {
		return []key.Binding{k.Save, k.Cancel}
	}
	return []key.Binding{k.Status, k.Notes, k.Report, k.Open, k.Hide, k.Scroll, k.Close}
}

func (k issueDetailKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (m *Model) handleIssuesKey(km tea.KeyPressMsg) tea.Cmd {
	v := m.iss
	if v.detail != nil {
		return m.handleIssueDetailKey(km)
	}
	if v.filtering {
		switch km.String() {
		case "esc":
			v.filter.SetValue("")
			fallthrough
		case "enter":
			v.filtering = false
			v.filter.Blur()
			return nil
		}
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(km)
		return cmd
	}
	k := km.String()
	if v.pending != "" {
		seq := v.pending + k
		v.pending = ""
		switch seq {
		case "gg":
			if list := m.issueList(); len(list) > 0 {
				v.sel = list[0].Key
			}
		case "gx":
			return m.openIssueURL()
		}
		return nil
	}
	if v.cfg == nil && k != "T" && k != "esc" && k != "q" && k != ":" {
		return nil // broken issues.yaml: only leaving works
	}
	card, i, ok := m.selectedIssue()
	list := m.issueList()
	switch k {
	case "g":
		v.pending = k
	case ":":
		m.mode, m.command = modeCommand, ""
	case "q":
		return tea.Quit
	case "T":
		m.issuesOpen = false
	case "esc":
		switch {
		case v.filter.Value() != "":
			v.filter.SetValue("")
		case m.help.ShowAll:
			m.help.ShowAll = false
		default:
			m.issuesOpen = false
		}
	case "?":
		m.help.ShowAll = !m.help.ShowAll
	case "1", "2", "3":
		v.tab = map[string]string{"1": "new", "2": "board", "3": "hidden"}[k]
		v.sel = ""
	case "j", "down":
		if ok && i+1 < len(list) {
			v.sel = list[i+1].Key
		}
	case "k", "up":
		if ok && i > 0 {
			v.sel = list[i-1].Key
		}
	case "G":
		if len(list) > 0 {
			v.sel = list[len(list)-1].Key
		}
	case "h", "left", "l", "right":
		delta := 1
		if k == "h" || k == "left" {
			delta = -1
		}
		if v.tab != "board" {
			if ok && i+delta >= 0 && i+delta < len(list) {
				v.sel = list[i+delta].Key
			}
			return nil
		}
		m.moveIssueColumn(delta, max(i, 0))
	case "H", "L":
		if !ok {
			return nil
		}
		return m.shiftIssueStatus(card, k == "L")
	case "J", "K":
		if !ok || v.tab != "board" {
			return nil
		}
		if v.sort != issues.SortManual {
			return m.notify("Pořadí jde měnit jen ve vlastním řazení (O)", true)
		}
		delta := 1
		if k == "K" {
			delta = -1
		}
		if next, after, ok := issues.Shift(list, card.Key, delta, v.byRepo); ok {
			if err := v.store.MoveNextTo(card.Key, next, after); err != nil {
				return m.fail(err)
			}
		}
	case "enter", "s":
		if ok {
			return m.openIssueDetail(card.Key, false)
		}
	case "n":
		if ok {
			return m.openIssueDetail(card.Key, true)
		}
	case "v":
		if ok {
			return m.reportOnIssue(card)
		}
	case "o":
		return m.openIssueURL()
	case "x":
		if ok {
			return m.toggleIssueHidden(card)
		}
	case "/":
		v.filtering = true
		return v.filter.Focus()
	case "O":
		v.sort = v.sort.Next()
		return m.notify("Řazení: "+v.sort.Label(), false)
	case "p":
		v.byRepo = !v.byRepo
	case "r":
		return m.syncIssues()
	}
	return nil
}

// moveIssueColumn selects the card at about the same row in the next
// column with cards.
func (m *Model) moveIssueColumn(delta, row int) {
	v := m.iss
	cols := v.view.Shown.Columns
	for c := v.col + delta; c >= 0 && c < len(cols); c += delta {
		if cards := cols[c].Cards; len(cards) > 0 {
			v.col, v.sel = c, cards[min(row, len(cards)-1)].Key
			return
		}
	}
	if c := v.col + delta; c >= 0 && c < len(cols) {
		v.col, v.sel = c, "" // an empty column: show it anyway
	}
}

// shiftIssueStatus moves an issue to the next or previous status; a new
// issue goes to the first one.
func (m *Model) shiftIssueStatus(card issues.Card, next bool) tea.Cmd {
	v := m.iss
	to := -1
	switch {
	case card.Hidden:
		return nil
	case card.Column < 0:
		if next {
			to = 0
		}
	case next:
		to = card.Column + 1
	default:
		to = card.Column - 1
	}
	if to < 0 || to >= len(v.cfg.Statuses) {
		return nil
	}
	return m.setIssueStatus(card, to)
}

func (m *Model) setIssueStatus(card issues.Card, to int) tea.Cmd {
	v := m.iss
	st := v.cfg.Statuses[to]
	if err := v.store.SetStatus(card.Key, st.Code, ""); err != nil {
		return m.fail(err)
	}
	if v.tab == "board" {
		v.col = to
	}
	return m.notify(card.Label()+" → "+st.Label(), false)
}

func (m *Model) toggleIssueHidden(card issues.Card) tea.Cmd {
	if err := m.iss.store.SetHidden(card.Key, !card.Hidden); err != nil {
		return m.fail(err)
	}
	if card.Hidden {
		return m.notify(card.Label()+" je zpět", false)
	}
	return m.notify(card.Label()+" skryto (3 = skrytá)", false)
}

func (m *Model) openIssueURL() tea.Cmd {
	key := m.iss.sel
	if d := m.iss.detail; d != nil {
		key = d.key
	}
	card, ok := m.issueByKey(key)
	if !ok {
		return nil
	}
	u := card.URL
	return func() tea.Msg { return openedMsg{"Issue otevřeno v prohlížeči", m.opener.Open(u)} }
}

// reportOnIssue opens the entry form for today with the issue filled in;
// saving returns to the overview.
func (m *Model) reportOnIssue(card issues.Card) tea.Cmd {
	m.iss.detail = nil
	return m.openForm(entry.Entry{Date: m.today, IssueURL: card.URL}, "Vykázat "+card.Label())
}

func (m *Model) openIssueDetail(key string, editNotes bool) tea.Cmd {
	card, ok := m.issueByKey(key)
	if !ok {
		return nil
	}
	all, err := m.store.All()
	if err != nil {
		return m.fail(err)
	}
	d := &issueDetail{key: key, notes: newDescArea(m.st, card.Notes), base: card.NotesAt, work: stats.BuildIssueSummary(card.URL, all, m.cfg)}
	d.notes.Placeholder = "co je potřeba, na co čekám, kontakty…"
	d.notes.SetHeight(6)
	d.notes.SetWidth(m.modalWidth() - 8)
	m.iss.detail = d
	if editNotes {
		d.editing = true
		return d.notes.Focus()
	}
	return nil
}

func (m *Model) handleIssueDetailKey(km tea.KeyPressMsg) tea.Cmd {
	v := m.iss
	d := v.detail
	card, ok := m.issueByKey(d.key)
	if !ok {
		v.detail = nil
		return nil
	}
	k := km.String()
	if d.editing {
		switch k {
		case "ctrl+s":
			stored, err := v.store.SetNotes(d.key, d.notes.Value(), d.base)
			if errors.Is(err, issues.ErrConflict) {
				// Keep what was typed on top of the newer version.
				m.loadIssues()
				if fresh, ok := m.issueByKey(d.key); ok {
					d.base = fresh.NotesAt
				}
				return m.notify("Poznámky se mezitím změnily v jiném okně – další ctrl+s je přepíše", true)
			}
			if err != nil {
				return m.fail(err)
			}
			d.base, d.editing = stored.NotesAt, false
			d.notes.Blur()
			return m.notify("Poznámky uloženy", false)
		case "esc":
			d.editing = false
			d.notes.Blur()
			d.notes.SetValue(card.Notes)
			return nil
		}
		var cmd tea.Cmd
		d.notes, cmd = d.notes.Update(km)
		return cmd
	}
	switch k {
	case "esc", "q", "enter", "s":
		v.detail = nil
	case "j", "down":
		d.offset++
	case "k", "up":
		d.offset = max(d.offset-1, 0)
	case "n":
		d.editing = true
		return d.notes.Focus()
	case "v":
		return m.reportOnIssue(card)
	case "o":
		return m.openIssueURL()
	case "x":
		v.detail = nil
		return m.toggleIssueHidden(card)
	default:
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(v.cfg.Statuses) {
			v.detail = nil
			return m.setIssueStatus(card, n-1)
		}
	}
	return nil
}

// --- sync -------------------------------------------------------------------------

type issuesSyncedMsg struct {
	res issues.SyncResult
	err error
}

// syncIssues reads GitLab in the background.
func (m *Model) syncIssues() tea.Cmd {
	v := m.iss
	if v.syncing || v.cfg == nil {
		return nil
	}
	st, err := m.issueStore()
	if err != nil {
		return m.fail(err)
	}
	src, err := issues.Connect(v.cfg, m.cfg)
	if err != nil {
		return m.fail(issues.RecordSyncError(st, err))
	}
	v.syncing = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), issues.SyncTimeout)
		defer cancel()
		res, err := issues.Sync(ctx, src, st)
		return issuesSyncedMsg{res, err}
	}
}

func (m *Model) issuesSynced(msg issuesSyncedMsg) tea.Cmd {
	m.iss.syncing = false
	m.loadIssues()
	if msg.err != nil {
		return m.fail(errors.New("GitLab: " + msg.err.Error()))
	}
	return m.notify("Načteno z GitLabu: "+msg.res.String(), false)
}
