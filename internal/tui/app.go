// Package tui is the full-screen terminal interface of Housenka.
package tui

import (
	"errors"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/entryedit"
	"github.com/just-tomdo/housenka/internal/opener"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/stats"
	"github.com/just-tomdo/housenka/internal/store"
	"github.com/just-tomdo/housenka/internal/tui/theme"
)

// Store is the persistence the UI needs.
type Store interface {
	Range(from, to civil.Date) ([]entry.Entry, error)
	All() ([]entry.Entry, error)
	Save(e entry.Entry) (entry.Entry, error)
	Delete(id string) error
}

// Opener opens URLs and copies text.
type Opener interface {
	Open(url string) error
	Copy(text string) error
}

type mode int

const (
	modeNormal mode = iota
	modeForm
	modeConfirmDelete
	modeConfirmOpen
	modeFilter
	modeCommand
	modeStatus
	modeIssue
)

// row is one line of the list: a day header or an entry.
type row struct {
	header bool
	day    stats.Day
	entry  int // index into Model.entries
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg    *config.Config
	rules  *rules.Rules
	store  Store
	opener Opener
	st     styles
	icons  theme.Icons
	keys   keyMap
	help   help.Model
	layout civil.Layout      // date format
	clock  func() civil.Date // current date; re-read on every key so midnight is handled
	today  civil.Date

	per     period.Period
	all     []entry.Entry // entries of the period
	entries []entry.Entry // after filter
	results map[string]rules.Result
	summary stats.Summary
	rows    []row
	cursor  int
	offset  int

	width, height int
	mode          mode
	form          *form
	filter        textinput.Model
	command       string
	pending       string // first key of a vim sequence (g, d, y)
	yanked        *entry.Entry
	lastSaved     entry.Entry
	status        *statusView
	issue         *issueView

	toast    string
	toastErr bool
	toastSeq int
	loadErr  error
}

// New creates the model showing today's entries. clock returns the current date.
func New(cfg *config.Config, s Store, o Opener, clock func() civil.Date) *Model {
	today := clock()
	colors := map[string]string{}
	for code, p := range cfg.Projects {
		colors[code] = p.Color
	}
	st := newStyles(theme.Get(cfg.UI.Theme), colors)
	h := help.New()
	h.Styles = st.Help
	f := textinput.New()
	f.Prompt = "/"
	m := &Model{
		cfg: cfg, rules: rules.New(cfg), store: s, opener: o,
		st: st, icons: theme.GetIcons(cfg.UI.NerdFont), keys: newKeyMap(), help: h,
		layout: cfg.UI.DateLayout(), clock: clock, today: today, per: period.Of(period.Day, today), filter: f,
	}
	m.reload("")
	return m
}

// Run starts the TUI.
func Run(cfg *config.Config) error {
	dir, err := config.DataDir()
	if err != nil {
		return err
	}
	s, err := store.Open(dir)
	if err != nil {
		return err
	}
	o := opener.New(opener.Detect(), cfg.BM.OpenCommand)
	_, err = tea.NewProgram(New(cfg, s, o, civil.Today)).Run()
	return err
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// --- messages ---------------------------------------------------------------

type toastExpiredMsg struct{ seq int }

type openedMsg struct {
	what string // toast shown on success
	err  error
}

type copiedMsg struct {
	text, what string
	err        error
}

func (m *Model) notify(text string, isErr bool) tea.Cmd {
	m.toast, m.toastErr = text, isErr
	m.toastSeq++
	seq := m.toastSeq
	d := 2 * time.Second
	if isErr {
		d = 5 * time.Second
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return toastExpiredMsg{seq} })
}

func (m *Model) fail(err error) tea.Cmd { return m.notify(firstLine(err.Error()), true) }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		if m.form != nil {
			m.form.setWidth(m.modalWidth() - 6)
		}
		m.ensureVisible()
		return m, nil
	case toastExpiredMsg:
		if msg.seq == m.toastSeq {
			m.toast = ""
		}
		return m, nil
	case openedMsg:
		if msg.err != nil {
			return m, m.fail(msg.err)
		}
		return m, m.notify(msg.what, false)
	case copiedMsg:
		if errors.Is(msg.err, opener.ErrNoClipboard) {
			return m, tea.Batch(tea.SetClipboard(msg.text), m.notify(msg.what+" zkopírován přes terminál (OSC 52)", false))
		}
		if msg.err != nil {
			return m, m.fail(msg.err)
		}
		return m, m.notify(msg.what+" zkopírován do schránky", false)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, m.handleKey(msg)
	}
	if m.mode == modeForm && m.form != nil {
		_, cmd := m.form.Update(msg)
		return m, cmd
	}
	if m.mode == modeFilter {
		before := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != before { // e.g. a paste
			m.reload(m.selectedID())
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(km tea.KeyPressMsg) tea.Cmd {
	if now := m.clock(); now != m.today {
		// The app was left running over midnight.
		m.today = now
		m.reload(m.selectedID())
	}
	switch m.mode {
	case modeForm:
		return m.handleFormKey(km)
	case modeConfirmDelete:
		m.mode = modeNormal
		if km.String() == "y" || km.String() == "enter" {
			return m.deleteSelected()
		}
		return nil
	case modeConfirmOpen:
		m.mode = modeNormal
		if km.String() == "y" || km.String() == "enter" {
			return m.openInBM(m.lastSaved)
		}
		return nil
	case modeFilter:
		return m.handleFilterKey(km)
	case modeCommand:
		return m.handleCommandKey(km)
	case modeStatus:
		return m.handleStatusKey(km)
	case modeIssue:
		return m.handleIssueKey(km)
	}
	return m.handleListKey(km)
}

func (m *Model) handleListKey(km tea.KeyPressMsg) tea.Cmd {
	k := km.String()
	if m.pending != "" {
		seq := m.pending + k
		m.pending = ""
		switch seq {
		case "gg":
			m.moveTo(0)
		case "dd":
			return m.confirmDelete()
		case "gx":
			return m.openIssue()
		case "yy":
			if e, ok := m.selected(); ok {
				m.yanked = &e
				return m.notify("Záznam zapamatován – p vloží kopii", false)
			}
		}
		return nil
	}
	switch k {
	case "g", "d", "y":
		m.pending = k
		return nil
	case ":":
		m.mode, m.command = modeCommand, ""
		return nil
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.reload(m.selectedID())
		}
		m.help.ShowAll = false
		return nil
	}

	keys := m.keys
	switch {
	case key.Matches(km, keys.Quit):
		return tea.Quit
	case key.Matches(km, keys.Down):
		m.moveBy(1)
	case key.Matches(km, keys.Up):
		m.moveBy(-1)
	case key.Matches(km, keys.Top):
		m.moveTo(0)
	case key.Matches(km, keys.Bottom):
		m.moveTo(len(m.rows) - 1)
	case key.Matches(km, keys.HalfDown):
		m.moveBy(max(m.listHeight()/2, 1))
	case key.Matches(km, keys.HalfUp):
		m.moveBy(-max(m.listHeight()/2, 1))
	case key.Matches(km, keys.PrevPeriod):
		m.setPeriod(m.per.Prev())
	case key.Matches(km, keys.NextPeriod):
		m.setPeriod(m.per.Next())
	case key.Matches(km, keys.Today):
		m.setPeriod(period.Of(m.per.Kind, m.today))
		m.selectDate(m.today)
	case key.Matches(km, keys.Day):
		m.setKind(period.Day)
	case key.Matches(km, keys.Week):
		m.setKind(period.Week)
	case key.Matches(km, keys.Month):
		m.setKind(period.Month)
	case key.Matches(km, keys.CycleKind):
		m.setKind(m.per.Kind.Next())
	case key.Matches(km, keys.Add):
		return m.openForm(entry.Entry{Date: m.focusDate()}, "Nový záznam")
	case key.Matches(km, keys.Edit):
		if e, ok := m.selected(); ok {
			return m.openForm(e, "Úprava záznamu")
		}
	case key.Matches(km, keys.Copy):
		if e, ok := m.selected(); ok {
			return m.openForm(entryedit.CopyOf(e, m.today), "Kopie záznamu na dnešek")
		}
	case key.Matches(km, keys.Paste):
		if m.yanked == nil {
			return m.notify("Nejdřív si záznam zapamatuj (yy)", true)
		}
		return m.openForm(entryedit.CopyOf(*m.yanked, m.focusDate()), "Vložení kopie záznamu")
	case key.Matches(km, keys.Delete):
		return m.confirmDelete()
	case key.Matches(km, keys.OpenBM):
		if e, ok := m.selected(); ok {
			return m.openInBM(e)
		}
	case key.Matches(km, keys.CopyURL):
		if e, ok := m.selected(); ok {
			return m.copyBMURL(e)
		}
	case key.Matches(km, keys.OpenIssue):
		return m.openIssue()
	case key.Matches(km, keys.IssueSummary):
		return m.openIssueSummary()
	case key.Matches(km, keys.ToggleSent):
		return m.toggleSent()
	case key.Matches(km, keys.Status):
		return m.openStatus()
	case key.Matches(km, keys.Filter):
		m.mode = modeFilter
		return m.filter.Focus()
	case key.Matches(km, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.ensureVisible()
	}
	return nil
}

func (m *Model) handleFormKey(km tea.KeyPressMsg) tea.Cmd {
	outcome, cmd := m.form.Update(km)
	switch outcome {
	case formCancelled:
		m.mode, m.form = modeNormal, nil
	case formSaved:
		e, resent := entryedit.PrepareSave(m.form.base, m.form.result)
		saved, err := m.store.Save(e)
		if errors.Is(err, store.ErrConflict) {
			// Changed meanwhile in the web interface: keep what was typed and
			// base the form on the new version, so saving again overwrites it
			// knowingly.
			m.reload(e.ID)
			fresh, ok := m.findEntry(e.ID)
			if !ok {
				m.mode, m.form = modeNormal, nil
				return m.notify("Záznam mezitím v jiném okně někdo smazal", true)
			}
			m.form.base = fresh
			return m.notify("Záznam se mezitím změnil v jiném okně – formulář drží tvoje úpravy, dalším uložením je zapíšeš", true)
		}
		if err != nil {
			return m.fail(err)
		}
		m.mode, m.form = modeConfirmOpen, nil
		m.lastSaved = saved
		if !m.per.Contains(saved.Date) {
			m.per = period.Of(m.per.Kind, saved.Date)
		}
		m.reload(saved.ID)
		if resent {
			return m.notify("Uloženo – záznam byl už odeslaný, uprav ho i v BM", true)
		}
		return m.notify("Uloženo "+m.icons.Sent, false)
	}
	return cmd
}

func (m *Model) handleFilterKey(km tea.KeyPressMsg) tea.Cmd {
	switch km.String() {
	case "esc":
		m.filter.SetValue("")
		fallthrough
	case "enter":
		m.mode = modeNormal
		m.filter.Blur()
		m.reload(m.selectedID())
		return nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(km)
	m.reload(m.selectedID())
	return cmd
}

func (m *Model) handleCommandKey(km tea.KeyPressMsg) tea.Cmd {
	switch k := km.String(); k {
	case "esc":
		m.mode = modeNormal
	case "enter":
		m.mode = modeNormal
		switch strings.TrimSpace(m.command) {
		case "q", "q!", "qa", "wq", "x":
			return tea.Quit
		case "":
		default:
			return m.notify("Neznámý příkaz :"+m.command+" (k dispozici :q)", true)
		}
	case "backspace":
		if m.command == "" {
			m.mode = modeNormal
		} else {
			r := []rune(m.command)
			m.command = string(r[:len(r)-1])
		}
	default:
		if km.Text != "" {
			m.command += km.Text
		}
	}
	return nil
}

// --- actions -------------------------------------------------------------------

func (m *Model) openForm(e entry.Entry, title string) tea.Cmd {
	m.form = newForm(m.rules, m.st, m.today, e, title)
	m.form.setWidth(m.modalWidth() - 6)
	m.mode = modeForm
	return m.form.Init()
}

func (m *Model) confirmDelete() tea.Cmd {
	if _, ok := m.selected(); ok {
		m.mode = modeConfirmDelete
	}
	return nil
}

func (m *Model) deleteSelected() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}
	if err := m.store.Delete(e.ID); err != nil {
		return m.fail(err)
	}
	pos := m.cursor
	m.reload("")
	m.moveTo(pos) // stay where the deleted entry was
	return m.notify("Záznam smazán", false)
}

// findEntry returns the stored entry with id, wherever its date is.
func (m *Model) findEntry(id string) (entry.Entry, bool) {
	all, err := m.store.All()
	if err != nil {
		return entry.Entry{}, false
	}
	for _, e := range all {
		if e.ID == id {
			return e, true
		}
	}
	return entry.Entry{}, false
}

func (m *Model) toggleSent() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}
	e.Sent = !e.Sent
	if _, err := m.store.Save(e); err != nil {
		if errors.Is(err, store.ErrConflict) {
			m.reload(e.ID)
		}
		return m.fail(err)
	}
	m.reload(e.ID)
	if e.Sent {
		return m.notify("Označeno jako odeslané do BM", false)
	}
	return m.notify("Označeno jako neodeslané", false)
}

func (m *Model) bmURL(e entry.Entry) (string, error) {
	return entryedit.BMURL(m.cfg, m.results[e.ID], e)
}

func (m *Model) openInBM(e entry.Entry) tea.Cmd {
	if _, ok := m.results[e.ID]; !ok {
		m.results[e.ID] = m.rules.Validate(e, m.today)
	}
	u, err := m.bmURL(e)
	if err != nil {
		return m.fail(err)
	}
	return func() tea.Msg { return openedMsg{"Otevřeno v Bílém motýlu", m.opener.Open(u)} }
}

func (m *Model) openIssue() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}
	if e.IssueURL == "" {
		return m.notify("Záznam nemá odkaz na issue", true)
	}
	u := e.IssueURL
	return func() tea.Msg { return openedMsg{"Issue otevřeno v prohlížeči", m.opener.Open(u)} }
}

func (m *Model) copyBMURL(e entry.Entry) tea.Cmd {
	u, err := m.bmURL(e)
	if err != nil {
		return m.fail(err)
	}
	return m.copyText(u, "Odkaz do BM")
}

func (m *Model) copyText(text, what string) tea.Cmd {
	return func() tea.Msg { return copiedMsg{text: text, what: what, err: m.opener.Copy(text)} }
}

// --- data & navigation -----------------------------------------------------------

func (m *Model) setPeriod(p period.Period) {
	m.per = p
	m.cursor, m.offset = 0, 0
	m.reload("")
}

func (m *Model) setKind(k period.Kind) {
	d := m.focusDate()
	m.per = period.Of(k, d)
	m.reload(m.selectedID())
}

// reload re-reads the period, re-validates entries and rebuilds rows,
// keeping the selection on selectID when possible.
func (m *Model) reload(selectID string) {
	all, err := m.store.Range(m.per.Start, m.per.End())
	m.loadErr = err
	m.all = all
	m.summary = stats.Summarize(m.per, all, m.cfg)
	m.results = map[string]rules.Result{}
	m.entries = nil
	for _, e := range all {
		m.results[e.ID] = m.rules.Validate(e, m.today)
		if entryedit.Matches(e, m.filter.Value()) {
			m.entries = append(m.entries, e)
		}
	}
	m.buildRows()

	m.cursor = m.firstEntryRow(0, 1)
	if selectID != "" {
		for i, r := range m.rows {
			if !r.header && m.entries[r.entry].ID == selectID {
				m.cursor = i
			}
		}
	}
	m.ensureVisible()
}

func (m *Model) buildRows() {
	m.rows = nil
	if m.per.Kind == period.Day {
		for i := range m.entries {
			m.rows = append(m.rows, row{entry: i})
		}
		return
	}
	for i, e := range m.entries {
		if i == 0 || m.entries[i-1].Date != e.Date {
			var day stats.Day
			for _, d := range m.summary.Days {
				if d.Date == e.Date {
					day = d
				}
			}
			m.rows = append(m.rows, row{header: true, day: day})
		}
		m.rows = append(m.rows, row{entry: i})
	}
}

// firstEntryRow returns the first entry row from i in direction dir, or -1.
func (m *Model) firstEntryRow(i, dir int) int {
	for ; i >= 0 && i < len(m.rows); i += dir {
		if !m.rows[i].header {
			return i
		}
	}
	return -1
}

func (m *Model) moveBy(delta int) {
	if m.cursor < 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir, delta = -1, -delta
	}
	for range delta {
		next := m.firstEntryRow(m.cursor+dir, dir)
		if next < 0 {
			break
		}
		m.cursor = next
	}
	m.ensureVisible()
}

// moveTo selects the entry row nearest to row i (looking down first, then up).
func (m *Model) moveTo(i int) {
	i = max(min(i, len(m.rows)-1), 0)
	if next := m.firstEntryRow(i, 1); next >= 0 {
		m.cursor = next
	} else if prev := m.firstEntryRow(i, -1); prev >= 0 {
		m.cursor = prev
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	if m.cursor < 0 || h <= 0 {
		m.offset = 0
		return
	}
	top := m.cursor
	if top > 0 && m.rows[top-1].header {
		top-- // keep the day header visible with its first entry
	}
	if top < m.offset {
		m.offset = top
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(min(m.offset, len(m.rows)-h), 0)
}

// selectDate selects the first entry of date d, if there is one.
func (m *Model) selectDate(d civil.Date) {
	for i, r := range m.rows {
		if !r.header && m.entries[r.entry].Date == d {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

func (m *Model) selected() (entry.Entry, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].header {
		return entry.Entry{}, false
	}
	return m.entries[m.rows[m.cursor].entry], true
}

func (m *Model) selectedID() string {
	e, _ := m.selected()
	return e.ID
}

// focusDate is the date new entries default to: the selected entry's date,
// today when it is in the shown period, or the period start.
func (m *Model) focusDate() civil.Date {
	if m.per.Kind == period.Day {
		return m.per.Start
	}
	if e, ok := m.selected(); ok {
		return e.Date
	}
	if m.per.Contains(m.today) {
		return m.today
	}
	return m.per.Start
}

// unsentCount returns the number of entries in the period not yet sent to BM.
func (m *Model) unsentCount() int {
	return len(slices.DeleteFunc(slices.Clone(m.all), func(e entry.Entry) bool { return e.Sent }))
}
