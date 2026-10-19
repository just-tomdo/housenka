package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/stats"
)

// issueView is the modal summarizing all work on one issue.
type issueView struct {
	summary stats.IssueSummary
	offset  int
}

type issueKeyMap struct {
	Scroll, Open, Close key.Binding
}

var issueKeys = issueKeyMap{
	Scroll: key.NewBinding(key.WithKeys("j", "k", "down", "up"), key.WithHelp("j/k", "posun")),
	Open:   key.NewBinding(key.WithKeys("i", "o"), key.WithHelp("i", "otevřít issue")),
	Close:  key.NewBinding(key.WithKeys("esc", "q", "I"), key.WithHelp("esc/q", "zavřít")),
}

func (k issueKeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Scroll, k.Open, k.Close} }

func (k issueKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// openIssueSummary shows the work of all years on the selected entry's issue.
func (m *Model) openIssueSummary() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}
	if e.IssueURL == "" {
		return m.notify("Záznam nemá odkaz na issue", true)
	}
	all, err := m.store.All()
	if err != nil {
		return m.fail(err)
	}
	m.issue = &issueView{summary: stats.BuildIssueSummary(e.IssueURL, all, m.cfg)}
	m.mode = modeIssue
	return nil
}

func (m *Model) handleIssueKey(km tea.KeyPressMsg) tea.Cmd {
	v := m.issue
	switch km.String() {
	case "j", "down":
		v.offset++
	case "k", "up":
		v.offset = max(v.offset-1, 0)
	case "i", "o":
		u := v.summary.URL
		return func() tea.Msg { return openedMsg{"Issue otevřeno v prohlížeči", m.opener.Open(u)} }
	case "esc", "q", "I":
		m.mode, m.issue = modeNormal, nil
	}
	return nil
}

// issueModal renders the issue summary: totals, shares per activity and
// project, and every entry.
func (m *Model) issueModal() string {
	st, s := m.st, m.issue.summary
	width := m.modalWidth() - 6
	lines := []string{
		st.Title.Render("Issue "+s.Label) + "  " + st.Muted.Render(ansi.Truncate(s.URL, max(width-len(s.Label)-8, 10), "…")),
		"",
	}
	facts := []string{
		st.Subtext.Render("celkem ") + st.Bold.Render(hours.Format(s.Total)+" h"),
		st.Text.Render(czPlural(len(s.Entries), "výkaz", "výkazy", "výkazů")),
		st.Text.Render(czPlural(s.Days, "den", "dny", "dní")),
	}
	if s.Days > 0 {
		span := m.layout.Format(s.First)
		if s.Last != s.First {
			span = m.layout.FormatRange(s.First, s.Last)
		}
		facts = append(facts, st.Subtext.Render(span))
	}
	if s.Unsent > 0 {
		facts = append(facts, st.Warn.Render(m.icons.Pending+" neodesláno "+hours.Format(s.Unsent)+" h"))
	}
	lines = append(lines, strings.Join(facts, st.Muted.Render(" · ")), "")

	share := func(sh stats.IssueShare, label string, labelWidth int) string {
		pct := 0
		if s.Total > 0 {
			pct = sh.Total * 100 / s.Total
		}
		name := st.Subtext.Render(ansi.Truncate(sh.Name, 24, "…"))
		if !sh.Known {
			name = st.Err.Render("není v configu")
		}
		return "  " + cell(label, labelWidth) + cell(name, 26) +
			st.Bold.Render(fmt.Sprintf("%7s h", hours.Format(sh.Total))) + st.Muted.Render(fmt.Sprintf("  %3d %%", pct))
	}
	lines = append(lines, st.DayHeader.Render("Činnosti"))
	for _, a := range s.Activities {
		label := st.Good.Render(a.Code)
		switch {
		case !a.Known:
			label = st.Err.Render(a.Code)
		case !a.Productive:
			label = st.Bad.Render(a.Code)
		}
		lines = append(lines, share(a, label, 12))
	}
	if len(s.Projects) > 1 {
		lines = append(lines, "", st.DayHeader.Render("Kódy projektů"))
		for _, p := range s.Projects {
			lines = append(lines, share(p, st.badge(p.Code), 12))
		}
	}

	lines = append(lines, "", st.DayHeader.Render("Výkazy"))
	for _, e := range s.Entries {
		icon := st.Muted.Render(m.icons.Pending)
		if e.Sent {
			icon = st.Good.Render(m.icons.Sent)
		}
		date := period.WeekdayShort(e.Date.Weekday()) + " " + m.layout.Format(e.Date)
		head := "  " + icon + " " + cell(st.Subtext.Render(date), 17) + cell(st.badge(e.ProjectCode), 12) +
			cell(st.Text.Render(e.Activity), 10) + st.Bold.Render(fmt.Sprintf("%6s", hours.Format(e.Quarters))) + "   "
		lines = append(lines, head+st.Text.Render(ansi.Truncate(flat(e.Description), max(width-ansi.StringWidth(head), 10), "…")))
	}

	maxLines := max(m.height-10, 5)
	m.issue.offset = max(min(m.issue.offset, len(lines)-maxLines), 0)
	visible := lines[m.issue.offset:min(m.issue.offset+maxLines, len(lines))]
	if len(lines) > maxLines {
		visible = append(visible, st.Muted.Render("… j/k posun"))
	}
	return st.Modal.Width(m.modalWidth()).Render(strings.Join(visible, "\n"))
}

// czPlural picks the Czech form for n: 1 → one, 2–4 → few, otherwise many.
func czPlural(n int, one, few, many string) string {
	form := many
	switch {
	case n == 1:
		form = one
	case n >= 2 && n <= 4:
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}
