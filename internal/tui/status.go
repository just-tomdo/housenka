package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/stats"
)

// statusView is the weekly summary modal for team status reports.
type statusView struct {
	per    period.Period
	status stats.Status
	offset int
}

type statusKeyMap struct {
	Scroll, Week, CopyText, CopyMD, Close key.Binding
}

var statusKeys = statusKeyMap{
	Scroll:   key.NewBinding(key.WithKeys("j", "k", "down", "up"), key.WithHelp("j/k", "posun")),
	Week:     key.NewBinding(key.WithKeys("h", "l", "left", "right"), key.WithHelp("h/l", "jiný týden")),
	CopyText: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "kopírovat text")),
	CopyMD:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "kopírovat Markdown")),
	Close:    key.NewBinding(key.WithKeys("esc", "q", "S"), key.WithHelp("esc/q", "zavřít")),
}

func (k statusKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Scroll, k.Week, k.CopyText, k.CopyMD, k.Close}
}

func (k statusKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (m *Model) openStatus() tea.Cmd {
	return m.loadStatus(period.Of(period.Week, m.focusDate()))
}

func (m *Model) loadStatus(p period.Period) tea.Cmd {
	entries, err := m.store.Range(p.Start, p.End())
	if err != nil {
		return m.fail(err)
	}
	m.status = &statusView{per: p, status: stats.BuildStatus(p, entries, m.cfg)}
	m.mode = modeStatus
	return nil
}

func (m *Model) handleStatusKey(km tea.KeyPressMsg) tea.Cmd {
	s := m.status
	switch km.String() {
	case "j", "down":
		s.offset++
	case "k", "up":
		s.offset = max(s.offset-1, 0)
	case "h", "left":
		return m.loadStatus(s.per.Prev())
	case "l", "right":
		return m.loadStatus(s.per.Next())
	case "y":
		return m.copyText(s.status.Text(), "Souhrn")
	case "m":
		return m.copyText(s.status.Markdown(), "Souhrn (Markdown)")
	case "esc", "q", "S":
		m.mode, m.status = modeNormal, nil
	}
	return nil
}

// statusModal renders the summary with colors; the copied text is plain.
func (m *Model) statusModal() string {
	st, s := m.st, m.status.status
	width := m.modalWidth() - 6
	var lines []string
	lines = append(lines,
		st.Title.Render("Týdenní souhrn")+"  "+st.Period.Render(s.Period.Label(s.Layout)),
		st.Subtext.Render("celkem ")+st.Bold.Render(hours.Format(s.Total)+" h"),
		"")
	if len(s.Projects) == 0 {
		lines = append(lines, st.Muted.Render("V tomto týdnu nejsou žádné záznamy."))
	}
	for _, p := range s.Projects {
		title := st.badge(p.Code)
		if p.Name != "" {
			title += " " + st.Bold.Render(p.Name)
		}
		lines = append(lines, title+st.Muted.Render("  "+hours.Format(p.Total)+" h"))
		for _, i := range p.Issues {
			label := st.Accent.Render(i.Label)
			if i.Label == "" {
				label = st.Muted.Render("bez issue")
			}
			head := "  • " + label + st.Muted.Render(" "+hours.Format(i.Total)+" h  ")
			descs := strings.Join(i.Descriptions, "; ")
			for j, part := range wrap(descs, width-lipgloss.Width(head)) {
				if j == 0 {
					lines = append(lines, head+st.Text.Render(part))
				} else {
					lines = append(lines, strings.Repeat(" ", lipgloss.Width(head))+st.Text.Render(part))
				}
			}
		}
		lines = append(lines, "")
	}

	maxLines := max(m.height-10, 5)
	m.status.offset = max(min(m.status.offset, len(lines)-maxLines), 0)
	visible := lines[m.status.offset:min(m.status.offset+maxLines, len(lines))]
	if len(lines) > maxLines {
		visible = append(visible, st.Muted.Render("… j/k posun"))
	}
	return st.Modal.Width(m.modalWidth()).Render(strings.Join(visible, "\n"))
}

// wrap breaks plain text into lines of at most width cells.
func wrap(s string, width int) []string {
	if width < 10 {
		width = 10
	}
	return strings.Split(ansi.Wrap(s, width, ""), "\n")
}
