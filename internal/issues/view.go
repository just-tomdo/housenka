package issues

import (
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
)

// Tab is one part of the overview, the same in both UIs.
type Tab struct {
	ID, Key, Label string
}

// Tabs lists the parts of the overview: key 1, 2, 3 switch between them.
var Tabs = []Tab{{"new", "1", "Nová"}, {"board", "2", "Nástěnka"}, {"hidden", "3", "Skrytá"}}

// Count returns the number of issues of the tab on the board b.
func (t Tab) Count(b Board) int {
	switch t.ID {
	case "new":
		return len(b.New)
	case "hidden":
		return len(b.Hidden)
	}
	return b.Total()
}

// DefaultTab is the tab the overview opens with: the new issues while
// there are any, else the board.
func DefaultTab(all Board) string {
	if len(all.New) > 0 {
		return "new"
	}
	return "board"
}

// EmptyText is what an empty tab says.
func EmptyText(tab, query string) string {
	switch {
	case query != "":
		return "Filtru „" + query + "“ nic neodpovídá."
	case tab == "new":
		return "Žádná nová issue 🐛 Vše je zařazené."
	case tab == "hidden":
		return "Žádná skrytá issue."
	}
	return "Na nástěnce nic není – zařaď issue ze záložky Nová."
}

// SyncedLabel says when GitLab was read last: "dnes 9:30", or "" never.
func SyncedLabel(f File, today civil.Date) string {
	if f.SyncedAt.IsZero() {
		return ""
	}
	t := f.SyncedAt.In(time.Local)
	return AgeLabel(civil.Of(t).DaysTo(today)) + " " + t.Format("15:04")
}

// Around returns the statuses before and after the column (H and L); a
// new issue (column -1) has only the first status after it.
func (c *Config) Around(column int) (prev, next string) {
	if column > 0 && column-1 < len(c.Statuses) {
		prev = c.Statuses[column-1].Code
	}
	if column+1 < len(c.Statuses) {
		next = c.Statuses[column+1].Code
	}
	return prev, next
}
