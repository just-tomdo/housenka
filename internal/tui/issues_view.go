package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/issues"
	"github.com/just-tomdo/housenka/internal/text"
)

// colorOf returns the color of a status or repository badge, like the
// project colors: spec from the config, or derived from key.
func (m *Model) colorOf(key, spec string) color.Color {
	return m.st.p.ProjectColor(key, spec)
}

func (m *Model) colorBadge(label, key, spec string) string {
	bg := m.colorOf(key, spec)
	return lipgloss.NewStyle().Bold(true).Foreground(m.st.p.TextOn(bg)).Background(bg).Padding(0, 1).Render(label)
}

func (m *Model) issuesScreen() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.issuesHeader(),
		m.issuesBody(),
		m.statusLine(),
		m.helpView(),
	)
}

func (m *Model) issuesHeader() string {
	st, v := m.st, m.iss
	inner := m.width - 4
	title := st.Title.Render("🐛 Housenka") + "  " + st.Period.Render("Issues")
	all := v.view.All
	var tabs []string
	for _, t := range issues.Tabs {
		label := fmt.Sprintf("%s %s %d", t.Key, t.Label, t.Count(all))
		if t.ID == v.tab {
			tabs = append(tabs, st.TabActive.Render(label))
		} else {
			tabs = append(tabs, st.TabInactive.Render(label))
		}
	}
	line1 := spread(title, strings.Join(tabs, ""), inner)

	var line2 string
	switch {
	case v.cfgErr != nil:
		line2 = st.Err.Render("issues.yaml obsahuje chyby (housenka issues edit): " + text.OneLine(v.cfgErr.Error()))
	case v.syncing:
		line2 = st.Accent.Render("Načítám z GitLabu…")
	case v.data.SyncError != "":
		line2 = st.Err.Render(m.icons.Warning + " Poslední načtení z GitLabu selhalo: " + text.OneLine(v.data.SyncError))
	case v.cfg == nil:
		line2 = st.Warn.Render("Přehled issues je vypnutý (issues.yaml chybí).")
	case v.data.SyncedAt.IsZero():
		line2 = st.Muted.Render("Z GitLabu se zatím nic nenačetlo – r načte.")
	default:
		line2 = st.Muted.Render("Z GitLabu načteno " + issues.SyncedLabel(v.data, m.today) + " · r načte znovu")
	}
	line2 = spread(line2, st.Muted.Render("řazení: "+v.sort.Label()+map[bool]string{true: " · podle repozitáře"}[v.byRepo]), inner)
	return m.surfaceBlock([]string{line1, line2})
}

func (m *Model) issuesBodyHeight() int {
	used := lipgloss.Height(m.issuesHeader()) + 1 + lipgloss.Height(m.helpView())
	return max(m.height-used, 3)
}

func (m *Model) issuesBody() string {
	v := m.iss
	h := m.issuesBodyHeight()
	var lines []string
	switch {
	case v.cfg == nil && v.cfgErr != nil:
		lines = []string{"", m.st.Muted.Render("  Oprav issues.yaml (housenka issues edit) nebo se vrať k výkazům (T).")}
	case v.cfg == nil:
		lines = []string{"", m.st.Muted.Render("  Zapneš ho příkazem housenka issues init. T se vrátí k výkazům.")}
	case v.loadErr != nil:
		lines = []string{m.st.Err.Render("Chyba při načítání: " + v.loadErr.Error())}
	case v.tab == "board":
		return m.boardView(h)
	default:
		lines = m.issueListView(h)
	}
	return m.paint(strings.Join(lines, "\n"), m.width, h)
}

// issueListView renders the new or hidden issues as full-width cards.
func (m *Model) issueListView(h int) []string {
	st, v := m.st, m.iss
	list := m.issueList()
	var lines []string
	if v.tab == "new" && len(list) > 0 {
		lines = append(lines, st.Muted.Render(fmt.Sprintf("  L zařadí do „%s“ · s/enter vybere stav · x skryje · v vykáže", v.cfg.Statuses[0].Label())), "")
	}
	if len(list) == 0 {
		return append(lines, "", "  "+st.Muted.Render(issues.EmptyText(v.tab, v.filter.Value())))
	}
	var cards [][]string
	selected := 0
	for i, c := range list {
		if c.Key == v.sel {
			selected = i
		}
		cards = append(cards, m.cardLines(c, m.width-2, c.Key == v.sel, true))
	}
	return append(lines, scrollCards(cards, selected, h-len(lines))...)
}

// boardView renders the statuses side by side; a narrow terminal shows the
// selected one only.
func (m *Model) boardView(h int) string {
	st, v := m.st, m.iss
	cols := v.view.Shown.Columns
	n := len(cols)
	gap := 1
	colW := (m.width - gap*(n-1)) / max(n, 1)
	first, last := 0, n-1
	if colW < 26 {
		first, last, colW = v.col, v.col, m.width
	}
	var blocks []string
	for i := first; i <= last; i++ {
		col := cols[i]
		c := m.colorOf(col.Status.Code, col.Status.Color)
		name := lipgloss.NewStyle().Bold(true).Foreground(c).Render("● " + col.Status.Label())
		if first == last {
			name = st.Muted.Render("‹ ") + name + st.Muted.Render(fmt.Sprintf(" (%d/%d) ›", i+1, n))
		}
		head := spread(" "+name, st.Muted.Render(strconv.Itoa(len(col.Cards))+" "), colW)
		underline := lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("━", colW))
		if i == v.col {
			underline = lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("▀", colW))
		}
		lines := []string{head, underline}
		var cards [][]string
		selected := 0
		repo := ""
		for _, card := range col.Cards {
			if v.byRepo && card.ProjectPath != repo {
				repo = card.ProjectPath
				cards = append(cards, []string{st.Muted.Render(ansi.Truncate(" ── "+repo+" ", colW, "…"))})
			}
			if card.Key == v.sel && i == v.col {
				selected = len(cards)
			}
			cards = append(cards, m.cardLines(card, colW, card.Key == v.sel && i == v.col, false))
		}
		if len(col.Cards) == 0 {
			cards = append(cards, []string{"", st.Muted.Render("  (prázdné)")})
		}
		lines = append(lines, scrollCards(cards, selected, h-2)...)
		for k := range lines {
			lines[k] = fit(lines[k], colW)
		}
		for len(lines) < h {
			lines = append(lines, strings.Repeat(" ", colW))
		}
		blocks = append(blocks, strings.Join(lines[:h], "\n"))
		if i < last {
			blocks = append(blocks, strings.Repeat(" \n", h-1)+" ")
		}
	}
	return m.paint(lipgloss.JoinHorizontal(lipgloss.Top, blocks...), m.width, h)
}

// fit truncates s to w cells (only when it is longer) and pads it to w.
func fit(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w-1, "…")
	}
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// scrollCards joins the cards' lines (a blank line between cards) into at
// most h lines with the selected card visible.
func scrollCards(cards [][]string, selected, h int) []string {
	var lines []string
	start, end := 0, 0
	for i, c := range cards {
		if i > 0 && len(c) > 1 {
			lines = append(lines, "")
		}
		if i == selected {
			start = len(lines)
		}
		lines = append(lines, c...)
		if i == selected {
			end = len(lines)
		}
	}
	if h <= 0 {
		return nil
	}
	offset := 0
	if end > h {
		offset = min(start, end-h)
	}
	return lines[offset:min(offset+h, len(lines))]
}

// cardLines renders one issue: repository and number, title, and facts.
func (m *Model) cardLines(c issues.Card, width int, selected, showStatus bool) []string {
	st, v := m.st, m.iss
	barColor := st.p.Overlay
	if c.Column >= 0 {
		s := v.cfg.Statuses[c.Column]
		barColor = m.colorOf(s.Code, s.Color)
	}
	bar := lipgloss.NewStyle().Foreground(barColor).Render("▌")
	if selected {
		bar = lipgloss.NewStyle().Foreground(st.p.Accent).Render("█")
	}
	inner := width - 2

	head := m.colorBadge(c.Issue.Repo(), c.Repo.ColorKey, c.Repo.ColorSpec) + " " + st.Accent.Render("#"+strconv.Itoa(c.IID))
	if showStatus && c.Column >= 0 {
		s := v.cfg.Statuses[c.Column]
		head += " " + lipgloss.NewStyle().Foreground(m.colorOf(s.Code, s.Color)).Render(s.Label())
	}
	right := ""
	if c.Work.Total > 0 {
		right = st.Subtext.Render(hours.Format(c.Work.Total) + " h")
	}
	title := st.Bold.Render(ansi.Truncate(c.Title, inner, "…"))
	if c.Closed || c.Unassigned {
		title = st.Subtext.Render(ansi.Truncate(c.Title, inner, "…"))
	}

	facts := []string{st.Subtext.Render("přiřazeno " + issues.AgeLabel(c.Assigned))}
	if c.Due != nil {
		due := "termín " + m.layout.Format(*c.Due)
		switch issues.DueKind(*c.Due, m.today) {
		case "err":
			facts = append(facts, st.Err.Render(due))
		case "warn":
			facts = append(facts, st.Warn.Render(due))
		default:
			facts = append(facts, st.Subtext.Render(due))
		}
	}
	if c.Notes != "" {
		facts = append(facts, st.Accent.Render("✎"))
	}
	for _, l := range c.Labels {
		facts = append(facts, st.Muted.Render(l))
	}
	var marks []string
	for _, mk := range c.Marks() {
		switch mk.Kind {
		case "err":
			marks = append(marks, st.Err.Render(mk.Text))
		case "warn":
			marks = append(marks, st.Warn.Render(mk.Text))
		default:
			marks = append(marks, st.Muted.Render(mk.Text))
		}
	}

	lines := []string{
		spread(head, right, inner),
		title,
		ansi.Truncate(strings.Join(facts, st.Muted.Render(" · ")), inner, "…"),
	}
	if len(marks) > 0 {
		lines = append(lines, ansi.Truncate(strings.Join(marks, st.Muted.Render(" · ")), inner, "…"))
	}
	for i, l := range lines {
		if c.Muted {
			l = st.Muted.Render(ansi.Strip(l)) // put off: the whole card greyed
		}
		l = bar + " " + l
		if selected {
			l = st.Selected.Render(fit(l, width))
		}
		lines[i] = l
	}
	return lines
}

// issueDetailModal renders the detail of the issue: facts, the status
// choice, notes and the reported work.
func (m *Model) issueDetailModal() string {
	st, v, d := m.st, m.iss, m.iss.detail
	card, ok := m.issueByKey(d.key)
	if !ok {
		return ""
	}
	width := m.modalWidth() - 6
	lines := []string{
		m.colorBadge(card.Issue.Repo(), card.Repo.ColorKey, card.Repo.ColorSpec) + " " + st.Accent.Render("#"+strconv.Itoa(card.IID)) + " " +
			st.Title.Render(ansi.Truncate(card.Title, max(width-20, 10), "…")),
		st.Muted.Render(ansi.Truncate(card.URL, width, "…")),
		"",
	}
	facts := []string{st.Text.Render("přiřazeno " + issues.AgeLabel(card.Assigned) + " (" + m.layout.Format(civil.Of(card.AssignedAt.Local())) + ")")}
	if card.Due != nil {
		facts = append(facts, st.Warn.Render("termín "+m.layout.Format(*card.Due)))
	}
	facts = append(facts, st.Muted.Render("v přehledu od "+m.layout.Format(civil.Of(card.FirstSeen.Local()))))
	lines = append(lines, strings.Join(facts, st.Muted.Render(" · ")))
	var extra []string
	for _, l := range card.Labels {
		extra = append(extra, st.Subtext.Render(l))
	}
	for _, mk := range card.Marks() {
		extra = append(extra, st.Warn.Render(mk.Text))
	}
	if len(extra) > 0 {
		lines = append(lines, strings.Join(extra, st.Muted.Render(" · ")))
	}
	if !card.Repo.Known() {
		for _, l := range wrap("Na issue zatím nejde vykázat: housenka nezná repozitář "+card.ProjectPath+
			" (nebo jeho GitLab). Doplň ho do repos (housenka config edit) nebo v Nastavení webu.", width) {
			lines = append(lines, st.Warn.Render(l))
		}
	}

	lines = append(lines, "", st.DayHeader.Render("Můj stav"))
	var chips []string
	for i, s := range v.cfg.Statuses {
		label := fmt.Sprintf("%d %s", i+1, s.Label())
		c := m.colorOf(s.Code, s.Color)
		if i == card.Column {
			chips = append(chips, lipgloss.NewStyle().Bold(true).Foreground(st.p.TextOn(c)).Background(c).Padding(0, 1).Render("● "+label))
		} else {
			chips = append(chips, lipgloss.NewStyle().Foreground(c).Padding(0, 1).Render(label))
		}
	}
	lines = append(lines, strings.Join(chips, " "))

	lines = append(lines, "", st.DayHeader.Render("Poznámky"))
	switch {
	case d.editing:
		lines = append(lines, strings.Split(d.notes.View(), "\n")...)
	case card.Notes == "":
		lines = append(lines, st.Muted.Render("žádné – n je napíše"))
	default:
		for _, p := range strings.Split(card.Notes, "\n") {
			for _, l := range wrap(p, width) {
				lines = append(lines, st.Text.Render(l))
			}
		}
	}

	lines = append(lines, "", st.DayHeader.Render("Vykázaná práce"))
	if len(d.work.Entries) == 0 {
		lines = append(lines, st.Muted.Render("Na issue zatím nic vykázáno – v vykáže."))
	} else {
		lines = append(lines, m.issueWorkLines(d.work, width)...)
	}

	maxLines := max(m.height-6, 8)
	if d.editing {
		d.offset = 0 // keep the notes in view while typing
	}
	d.offset = max(min(d.offset, len(lines)-maxLines), 0)
	visible := lines[d.offset:min(d.offset+maxLines, len(lines))]
	if len(lines) > maxLines {
		visible = append(visible, st.Muted.Render("… j/k posun"))
	}
	return st.Modal.Width(m.modalWidth()).Render(strings.Join(visible, "\n"))
}
