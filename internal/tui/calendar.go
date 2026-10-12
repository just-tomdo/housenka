package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/period"
)

// calendar is a month view for picking a date with the keyboard.
type calendar struct {
	st       styles
	cursor   civil.Date
	today    civil.Date
	selected civil.Date // date in the field when the calendar opened; may be zero
}

type calendarOutcome int

const (
	calendarOpen calendarOutcome = iota
	calendarClosed
	calendarPicked
)

func newCalendar(st styles, selected, today civil.Date) *calendar {
	c := &calendar{st: st, cursor: selected, today: today, selected: selected}
	if !selected.Valid() {
		c.cursor = today
	}
	return c
}

// Update moves the cursor; on calendarPicked the date is in c.cursor.
func (c *calendar) Update(km tea.KeyPressMsg) calendarOutcome {
	switch km.String() {
	case "h", "left":
		c.cursor = c.cursor.AddDays(-1)
	case "l", "right":
		c.cursor = c.cursor.AddDays(1)
	case "k", "up":
		c.cursor = c.cursor.AddDays(-7)
	case "j", "down":
		c.cursor = c.cursor.AddDays(7)
	case "H", "pgup", "[":
		c.cursor = addMonths(c.cursor, -1)
	case "L", "pgdown", "]":
		c.cursor = addMonths(c.cursor, 1)
	case "t":
		c.cursor = c.today
	case "home", "0", "^":
		c.cursor.Day = 1
	case "end", "$":
		c.cursor.Day = daysIn(c.cursor.Year, c.cursor.Month)
	case "enter", "space":
		return calendarPicked
	case "esc", "q", "ctrl+k":
		return calendarClosed
	}
	return calendarOpen
}

// View renders the month with the cursor, today and the selected date.
func (c *calendar) View() string {
	st := c.st
	title := fmt.Sprintf("%s %d", period.MonthName(c.cursor.Month), c.cursor.Year)
	pad := max(20-len([]rune(title))-4, 0)
	lines := []string{
		st.Muted.Render("‹ ") + strings.Repeat(" ", pad/2) + st.Bold.Render(title) +
			strings.Repeat(" ", pad-pad/2) + st.Muted.Render(" ›"),
		st.Subtext.Render("po út st čt pá ") + st.Muted.Render("so ne"),
	}
	first := civil.Date{Year: c.cursor.Year, Month: c.cursor.Month, Day: 1}
	col := (int(first.Weekday()) + 6) % 7 // Monday first
	line := strings.Repeat("   ", col)
	for d := 1; d <= daysIn(c.cursor.Year, c.cursor.Month); d++ {
		day := civil.Date{Year: c.cursor.Year, Month: c.cursor.Month, Day: d}
		style := st.Text
		switch {
		case day == c.cursor:
			style = st.OptionFocused.Padding(0)
		case day == c.selected:
			style = st.OptionActive.Padding(0)
		case day == c.today:
			style = st.Accent.Bold(true).Underline(true)
		case col >= 5:
			style = st.Muted
		}
		line += style.Render(fmt.Sprintf("%2d", d))
		if col == 6 {
			lines = append(lines, line)
			line, col = "", 0
			continue
		}
		line += " "
		col++
	}
	if line != "" {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return strings.Join(lines, "\n")
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// addMonths shifts d by n months, clamping the day (31. 1. + 1 → 28. 2.).
func addMonths(d civil.Date, n int) civil.Date {
	t := time.Date(d.Year, d.Month+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	return civil.Date{Year: t.Year(), Month: t.Month(), Day: min(d.Day, daysIn(t.Year(), t.Month()))}
}
