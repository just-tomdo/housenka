package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
)

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Housenka"
	if m.cfg.UI.Background {
		// Sets the terminal's default colors (OSC 10/11), restored on exit.
		// This also covers the window padding outside the cell grid.
		v.BackgroundColor, v.ForegroundColor = m.st.p.Base, m.st.p.Text
	}
	return v
}

func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < 50 || m.height < 15 {
		return m.paint(m.st.Warn.Render("Terminál je příliš malý – zvětši okno (min. 50×15)."), m.width, m.height)
	}
	base := m.paint(lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		m.listView(),
		m.detailView(),
		m.statusLine(),
		m.helpView(),
	), m.width, m.height)
	switch {
	case m.mode == modeForm && m.form != nil:
		return m.overlay(base, m.st.Modal.Width(m.modalWidth()).Render(m.form.View()))
	case m.mode == modeStatus && m.status != nil:
		return m.overlay(base, m.statusModal())
	}
	return base
}

// paint fills a block of at least width×height cells with the theme
// background when ui.background is on. Terminals that ignore OSC 11 still
// get the theme colors this way. Both compositor layers must be painted,
// otherwise the compositor switches unpainted cells back to the default.
func (m *Model) paint(block string, width, height int) string {
	if !m.cfg.UI.Background {
		return block
	}
	return paintBlock(block, width, height, m.st.p.Base)
}

// paintBlock fills a block of at least width×height cells with bg.
func paintBlock(block string, width, height int, bg color.Color) string {
	lines := strings.Split(block, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = fillBackground(l, width, bg)
	}
	return strings.Join(lines, "\n")
}

// overlay draws modal centered over base.
func (m *Model) overlay(base, modal string) string {
	modal = m.paint(modal, lipgloss.Width(modal), 0)
	x := max((m.width-lipgloss.Width(modal))/2, 0)
	y := max((m.height-lipgloss.Height(modal))/2, 0)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
	).Render()
}

func (m *Model) modalWidth() int { return min(m.width-4, 100) }

// detailHeight is the height of the detail panel: 2 border lines + 5 content lines.
const detailHeight = 7

// listHeight is the number of rows available for the list (without its column header).
func (m *Model) listHeight() int {
	if m.height == 0 {
		return 0
	}
	used := lipgloss.Height(m.headerView()) + 1 /* column header */ + detailHeight + 1 /* status */ + lipgloss.Height(m.helpView())
	return max(m.height-used, 1)
}

// columns computes list column widths for the current terminal width.
type columns struct{ group, repo, iid, project, activity, hours, desc int }

func (m *Model) columns() columns {
	c := columns{group: 10, repo: 20, iid: 7, project: 12, activity: 10, hours: 9}
	fixed := func() int { return 3 + c.group + c.repo + c.iid + c.project + c.activity + c.hours }
	if m.width-fixed() < 30 {
		c.group = 0
	}
	if m.width-fixed() < 25 {
		c.repo = 14
	}
	c.desc = max(m.width-fixed(), 10)
	return c
}

// cell truncates s to w cells and pads it to exactly w.
func cell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w-1, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func (m *Model) listView() string {
	st := m.st
	h := m.listHeight()
	c := m.columns()
	head := " " + "  " + cell("Skupina", c.group) + cell("Repozitář", c.repo) + cell("Issue", c.iid) +
		cell("Kód", c.project) + cell("Činnost", c.activity) + fmt.Sprintf("%6s   ", "Hodiny") + "Popis"
	lines := []string{st.Muted.Render(ansi.Truncate(head, m.width, ""))}

	switch {
	case m.loadErr != nil:
		lines = append(lines, st.Err.Render("Chyba při načítání dat: "+m.loadErr.Error()))
	case len(m.rows) == 0:
		lines = append(lines, "", m.emptyMessage())
	default:
		end := min(m.offset+h, len(m.rows))
		for i := m.offset; i < end; i++ {
			lines = append(lines, ansi.Truncate(m.rowView(m.rows[i], i == m.cursor, c), m.width, "…"))
		}
	}
	for len(lines) < h+1 {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h+1], "\n")
}

func (m *Model) emptyMessage() string {
	st := m.st
	if m.filter.Value() != "" {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, st.Muted.Render("Filtru „"+m.filter.Value()+"“ nic neodpovídá – esc ho zruší"))
	}
	msg := "Zatím nic vykázáno 🐛  stiskni " + st.Accent.Render("a") + st.Muted.Render(" a vykaž první práci")
	if m.per.Kind != period.Day {
		msg = "V tomto období nejsou žádné záznamy – " + st.Accent.Render("a") + st.Muted.Render(" přidá nový")
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, st.Muted.Render(msg))
}

func (m *Model) rowView(r row, selected bool, c columns) string {
	st := m.st
	if r.header {
		d := r.day
		label := st.DayHeader.Render(period.WeekdayShort(d.Date.Weekday()) + " " + m.layout.Format(d.Date))
		if d.Date == m.today {
			label += " " + st.Today.Render("dnes")
		}
		pct := ""
		if d.Total > 0 {
			pct = st.Muted.Render(fmt.Sprintf(" · %d %%", d.ProductivePercent()))
		}
		line := label + "  " + m.miniBar(d.Totals, 10) + "  " + st.Text.Render(hours.Format(d.Total)+" h") + pct
		return line
	}

	e := m.entries[r.entry]
	res := m.results[e.ID]
	icon := st.Muted.Render(m.icons.Pending)
	switch {
	case !res.Valid():
		icon = st.Err.Render(m.icons.Invalid)
	case e.Sent:
		icon = st.Good.Render(m.icons.Sent)
	}

	group, repo, iid := st.Muted.Render(m.icons.NoIssue), st.Muted.Render("bez issue"), ""
	if ref, err := gitlaburl.Parse(e.IssueURL); err == nil {
		group, repo = st.Subtext.Render(ref.Group), st.Text.Render(ref.Repo)
		iid = st.Accent.Render(issuePrefix(ref) + fmt.Sprint(ref.IID))
	} else if e.IssueURL != "" {
		repo = st.Err.Render("neplatný odkaz")
	}

	activity := st.Good.Render(e.Activity)
	if a, ok := m.cfg.Activities[e.Activity]; !ok {
		activity = st.Err.Render(e.Activity)
	} else if !a.IsProductive() {
		activity = st.Bad.Render(e.Activity)
	}

	desc := st.Subtext.Render(flat(e.Description))
	marker := " "
	if selected {
		marker = st.Accent.Render("▌")
		desc = st.Bold.Render(flat(e.Description))
	}
	hrs := fmt.Sprintf("%6s   ", hours.Format(e.Quarters))

	return marker + icon + " " + cell(group, c.group) + cell(repo, c.repo) + cell(iid, c.iid) +
		cell(st.badge(e.ProjectCode), c.project) + cell(activity, c.activity) + st.Bold.Render(hrs) +
		cell(desc, c.desc)
}

// flat shows a multi-line description on one line.
func flat(s string) string {
	return strings.Join(nonEmptyLines(s), " ↵ ")
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func issuePrefix(ref gitlaburl.Ref) string {
	if ref.Kind == gitlaburl.MergeRequest {
		return "!"
	}
	return "#"
}

// detailView shows the selected entry in a framed panel as a two-column
// grid of labeled values, followed by the description (two lines) or errors.
func (m *Model) detailView() string {
	st := m.st
	inner := m.width - 4
	const labelW, leftW = 9, 34
	label := func(s string) string { return st.Muted.Render(cell(s, labelW)) }
	row := func(l1, v1, l2, v2 string) string { return label(l1) + cell(v1, leftW-labelW) + label(l2) + v2 }

	var lines []string
	e, ok := m.selected()
	if !ok {
		lines = []string{st.Muted.Render("Žádný záznam není vybraný."), "", "", "", ""}
	} else {
		cfg := m.cfg
		res := m.results[e.ID]

		project := st.badge(e.ProjectCode)
		if p, ok := cfg.Projects[e.ProjectCode]; ok {
			project += " " + st.Text.Render(p.Name)
		}
		activity := st.Err.Render(e.Activity + " (není v configu)")
		if a, ok := cfg.Activities[e.Activity]; ok {
			kind, word := st.Good, "produktivní"
			if !a.IsProductive() {
				kind, word = st.Bad, "neproduktivní"
			}
			activity = kind.Render(e.Activity) + " " + st.Text.Render(a.Name) + st.Muted.Render(" · ") + kind.Render(word)
		}
		state := st.Subtext.Render(m.icons.Pending + " čeká na odeslání")
		switch {
		case !res.Valid():
			state = st.Err.Render(m.icons.Invalid + " neplatný")
		case e.Sent:
			state = st.Good.Render(m.icons.Sent + " odesláno do BM")
		}
		issue := st.Muted.Render("bez odkazu na issue")
		if ref, err := gitlaburl.Parse(e.IssueURL); err == nil {
			issue = st.Accent.Render(ref.Short()) + "   " + st.Muted.Render(e.IssueURL)
		} else if e.IssueURL != "" {
			issue = st.Err.Render(e.IssueURL)
		}

		desc := nonEmptyLines(e.Description)
		descLines := []string{"", ""}
		switch {
		case len(desc) == 1:
			wrapped := strings.Split(ansi.Wrap(desc[0], inner-labelW, " "), "\n")
			descLines[0] = wrapped[0]
			descLines[1] = strings.Join(wrapped[1:], " ")
		case len(desc) > 1:
			descLines[0], descLines[1] = desc[0], strings.Join(desc[1:], " ↵ ")
		}
		last := label("") + st.Text.Render(descLines[1])
		if errs := res.Errors(); len(errs) > 0 {
			var msgs []string
			for _, f := range errs {
				msgs = append(msgs, firstLine(f.Message))
			}
			last = label("Chyba") + st.Err.Render(m.icons.Invalid+" "+strings.Join(msgs, " · "))
		}

		lines = []string{
			row("Datum", st.Text.Render(period.WeekdayShort(e.Date.Weekday())+" "+m.layout.Format(e.Date)), "Projekt", project),
			row("Hodiny", st.Bold.Render(hours.Format(e.Quarters)+" h"), "Činnost", activity),
			row("Stav", state, "Issue", issue),
			label("Popis") + st.Text.Render(descLines[0]),
			last,
		}
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, inner, "…")
	}
	return m.titledPanel("Detail", strings.Join(lines, "\n"))
}

// titledPanel renders content in a rounded border with a title in the top edge.
func (m *Model) titledPanel(title, content string) string {
	st := m.st
	body := st.Panel.BorderTop(false).Width(m.width).Render(content)
	border := lipgloss.NewStyle().Foreground(st.p.Overlay)
	head := border.Render("┌─ ") + st.DayHeader.Render(title) + " "
	head += border.Render(strings.Repeat("─", max(m.width-lipgloss.Width(head)-1, 0)) + "┐")
	return head + "\n" + body
}

// statusLine is a lualine-like bar: mode block, context, and info on the right.
func (m *Model) statusLine() string {
	st := m.st
	modeName, modeStyle := "NORMAL", st.ModeNormal
	middle := st.StatusBar.Render(" " + m.per.Kind.String() + " · " + m.per.Label(m.layout) + " ")
	switch m.mode {
	case modeForm:
		modeName, modeStyle = "INSERT", st.ModeInsert
	case modeFilter:
		modeName, modeStyle = "FILTR", st.ModeOther
		middle = " " + m.filter.View()
	case modeCommand:
		modeName, modeStyle = "PŘÍKAZ", st.ModeOther
		middle = st.Text.Render(" :" + m.command + "█")
	case modeConfirmDelete:
		modeName, modeStyle = "SMAZAT?", st.ToastErr
		e, _ := m.selected()
		middle = st.Warn.Render(fmt.Sprintf(" Smazat záznam „%s“ (%s h)? y/n", ansi.Truncate(flat(e.Description), 40, "…"), hours.Format(e.Quarters)))
	case modeConfirmOpen:
		modeName, modeStyle = "ULOŽENO", st.ModeInsert
		middle = st.Text.Render(" Otevřít záznam v Bílém motýlu? y/n")
	case modeStatus:
		modeName, modeStyle = "SOUHRN", st.ModeOther
	}
	if m.pending != "" {
		middle += st.Accent.Render(" " + m.pending)
	}
	if f := m.filter.Value(); f != "" && m.mode != modeFilter {
		middle += st.Warn.Render(" filtr: " + f)
	}

	var right string
	switch {
	case m.toast != "" && m.toastErr:
		right = st.ToastErr.Render(m.toast)
	case m.toast != "":
		right = st.Toast.Render(m.toast)
	default:
		info := fmt.Sprintf("Σ %s h", hours.Format(m.summary.Total))
		if n := m.unsentCount(); n > 0 {
			info += fmt.Sprintf(" · %s %d neodesláno", m.icons.Pending, n)
		}
		if m.per.Kind == period.Day && m.summary.Total >= int(m.cfg.Time.DailyGoal) {
			info += " · 🦋 cíl splněn"
		}
		right = st.StatusSeg.Render(info)
	}

	left := modeStyle.Render(modeName) + middle
	left = ansi.Truncate(left, max(m.width-lipgloss.Width(right)-1, 0), "…")
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return left + st.StatusBar.Render(strings.Repeat(" ", gap)) + right
}

func (m *Model) helpView() string {
	var v string
	switch m.mode {
	case modeForm:
		k := m.form.keys
		k.inDesc = m.form.focus == fieldDesc
		k.inButton = m.form.isButton(m.form.focus)
		k.inDate = m.form.focus == fieldDate
		k.inCal = m.form.cal != nil
		v = m.help.View(k)
	case modeStatus:
		v = m.help.View(statusKeys)
	default:
		v = m.help.View(m.keys)
	}
	// help.Model does not always respect its width (when even the ellipsis
	// does not fit it keeps adding items), so clip every line ourselves.
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}
