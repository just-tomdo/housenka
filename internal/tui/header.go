package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/stats"
)

// periodGoal is the expected work in quarters: the daily goal for a day, or
// the daily goal times the number of working days (Mon–Fri) in the period.
func (m *Model) periodGoal() int {
	goal := int(m.cfg.Time.DailyGoal)
	if m.per.Kind == period.Day {
		return goal
	}
	n := 0
	for _, d := range m.per.Days() {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return goal * n
}

// headerView renders the top panel: title, period tabs, totals and the bar.
func (m *Model) headerView() string {
	st := m.st
	inner := m.width - 4 // two spaces of padding on each side

	title := st.Title.Render("🐛 Housenka") + "  " + st.Period.Render(m.per.Label())
	var tabs []string
	for _, k := range []period.Kind{period.Day, period.Week, period.Month} {
		label := fmt.Sprintf("%d %s", int(k)+1, k)
		if k == m.per.Kind {
			tabs = append(tabs, st.TabActive.Render(label))
		} else {
			tabs = append(tabs, st.TabInactive.Render(label))
		}
	}
	line1 := spread(title, strings.Join(tabs, ""), inner)

	s := m.summary
	goal := m.periodGoal()
	total := st.Bold.Render(hours.Format(s.Total)) + st.Muted.Render(" / "+hours.Format(goal)+" h")
	if m.per.Kind == period.Day && s.Total > int(m.cfg.Time.DailyWarning) {
		total = st.Warn.Render(hours.Format(s.Total)+" h "+m.icons.Warning) + st.Muted.Render(" (nad "+hours.Format(int(m.cfg.Time.DailyWarning))+" h)")
	}
	parts := []string{
		total,
		st.Good.Render("produktivní " + hours.Format(s.Productive) + " h"),
		st.Bad.Render("neproduktivní " + hours.Format(s.Unproductive) + " h"),
	}
	if s.Unknown > 0 {
		parts = append(parts, st.Err.Render("neznámá činnost "+hours.Format(s.Unknown)+" h"))
	}
	if m.per.Kind != period.Day {
		parts = append(parts, st.Subtext.Render(fmt.Sprintf("dní %d · ⌀ %s h/den", s.WorkedDays,
			strings.Replace(fmt.Sprintf("%.2f", s.AvgHoursPerWorkedDay()), ".", ",", 1))))
	}
	line2 := strings.Join(parts, st.Muted.Render(" · "))

	pct := ""
	if s.Total > 0 {
		p := s.ProductivePercent()
		pct = st.Good.Render(fmt.Sprintf("%d %%", p)) + st.Muted.Render(" / ") + st.Bad.Render(fmt.Sprintf("%d %%", 100-p))
	}
	barWidth := max(inner-lipgloss.Width(pct)-2, 10)
	line3 := m.progressBar(barWidth, s.Totals, goal) + "  " + pct

	lines := []string{line1, line2, line3}
	if m.per.Kind == period.Week {
		if wl := m.weekDaysLine(inner); wl != "" {
			lines = append(lines, wl)
		}
	}
	return m.surfaceBlock(lines)
}

// progressBar draws productive (green) and unproductive (orange) work
// relative to goal. With ui.caterpillar it is a caterpillar crawling towards
// a leaf that turns into a butterfly when the goal is reached.
func (m *Model) progressBar(width int, t stats.Totals, goal int) string {
	st := m.st
	reached := goal > 0 && t.Total >= goal
	caterpillar := m.cfg.UI.Caterpillar
	body := width
	if caterpillar {
		body = width - 4 // head (2 cells) + leaf (2 cells)
	}
	scale := max(goal, t.Total, 1)
	prod := cells(t.Productive, scale, body)
	unprod := min(cells(t.Productive+t.Unproductive+t.Unknown, scale, body)-prod, body-prod)
	empty := body - prod - unprod

	if !caterpillar {
		return st.Good.Render(strings.Repeat("█", prod)) +
			st.Bad.Render(strings.Repeat("█", unprod)) +
			st.Muted.Render(strings.Repeat("░", empty))
	}
	head, leaf := "🐛", "🍃"
	if reached {
		head, leaf = "🦋", "  "
	}
	if t.Total == 0 {
		head = "🥚"
	}
	return st.Good.Render(strings.Repeat("●", prod)) +
		st.Bad.Render(strings.Repeat("●", unprod)) +
		head +
		st.Muted.Render(strings.Repeat("·", empty)) +
		leaf
}

// cells converts value/scale to a number of cells out of width, rounded.
func cells(value, scale, width int) int {
	if width <= 0 || scale <= 0 {
		return 0
	}
	return min(int(math.Round(float64(value)*float64(width)/float64(scale))), width)
}

// miniBar is a compact two-color bar used for days.
func (m *Model) miniBar(t stats.Totals, width int) string {
	scale := max(int(m.cfg.Time.DailyGoal), t.Total, 1)
	prod := cells(t.Productive, scale, width)
	other := min(cells(t.Total, scale, width)-prod, width-prod)
	return m.st.Good.Render(strings.Repeat("▆", prod)) +
		m.st.Bad.Render(strings.Repeat("▆", other)) +
		m.st.Muted.Render(strings.Repeat("▁", width-prod-other))
}

// weekDaysLine shows each working day (and worked weekend days) of the week.
func (m *Model) weekDaysLine(width int) string {
	var parts []string
	for _, d := range m.summary.Days {
		wd := d.Date.Weekday()
		if (wd == time.Saturday || wd == time.Sunday) && d.Total == 0 {
			continue
		}
		label := m.st.Subtext.Render(period.WeekdayShort(wd))
		if d.Date == m.today {
			label = m.st.Accent.Render(period.WeekdayShort(wd))
		}
		parts = append(parts, label+" "+m.miniBar(d.Totals, 6)+" "+m.st.Text.Render(hours.Format(d.Total)))
	}
	line := strings.Join(parts, "   ")
	if lipgloss.Width(line) > width {
		return ""
	}
	return line
}

// surfaceBlock renders lines on a full-width, softly colored background
// with one empty line above and below.
func (m *Model) surfaceBlock(lines []string) string {
	bg := m.st.p.Surface
	out := []string{fillBackground("", m.width, bg)}
	for _, l := range lines {
		for _, w := range strings.Split(ansi.Wrap(l, m.width-4, " "), "\n") {
			out = append(out, fillBackground("  "+w, m.width, bg))
		}
	}
	out = append(out, fillBackground("", m.width, bg))
	return strings.Join(out, "\n")
}

// fillBackground paints line on bg up to width. Styled segments end with a
// reset that would also clear the background, so bg is re-applied after
// every reset.
func fillBackground(line string, width int, bg color.Color) string {
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	on := probe[:strings.Index(probe, "x")]
	const reset = "\x1b[m"
	line = strings.ReplaceAll(line, "\x1b[0m", reset)
	line = strings.ReplaceAll(line, reset, reset+on)
	pad := max(width-lipgloss.Width(line), 0)
	return on + line + strings.Repeat(" ", pad) + reset
}

// spread places left and right on one line of the given width.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}
