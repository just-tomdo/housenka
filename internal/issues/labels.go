package issues

import (
	"strconv"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/text"
)

// Mark is a remark on a card; Kind is "err", "warn" or "muted".
type Mark struct {
	Text, Kind, Title string
}

// Marks returns the remarks the UIs show on a card, most important first.
func (c Card) Marks() []Mark {
	var out []Mark
	if c.Muted {
		out = append(out, Mark{"odložené", "muted", "Issue je odložené na později: je zašedlé a na konci sloupce. Klávesa z ho vrátí."})
	}
	if c.Closed {
		out = append(out, Mark{"zavřeno v GitLabu", "muted", "Issue je v GitLabu zavřené (nebo smazané či přesunuté). Až ho nebudeš potřebovat, skryj ho."})
	}
	if c.Unassigned {
		out = append(out, Mark{"už není na tobě", "warn", "Issue je v GitLabu přiřazené někomu jinému."})
	}
	switch {
	case !c.Repo.HostKnown:
		out = append(out, Mark{"GitLab není v housence", "warn", "Doménu " + c.Host + " doplň do gitlab_hosts, jinak na issue nepůjde vykázat."})
	case len(c.Repo.Projects) == 0:
		out = append(out, Mark{"repozitář není v housence", "warn", "Repozitář " + c.ProjectPath + " přidej do repos s kódy projektů, jinak na issue nepůjde vykázat."})
	}
	if c.LostStatus {
		out = append(out, Mark{"stav „" + c.Status + "“ už neexistuje", "warn", "Stav byl z issues.yaml odebraný – zařaď issue znovu."})
	}
	if c.Stale {
		out = append(out, Mark{"beze mě " + text.Plural(c.Idle, "den", "dny", "dní"), "warn",
			"Na issue se " + text.Plural(c.Idle, "den", "dny", "dní") + " nic nestalo: žádná změna stavu, poznámka ani výkaz."})
	}
	return out
}

// AgeLabel describes how many days ago something happened: "dnes",
// "včera", "před 5 dny".
func AgeLabel(days int) string {
	switch {
	case days <= 0: // also a clock set back
		return "dnes"
	case days == 1:
		return "včera"
	}
	return "před " + strconv.Itoa(days) + " dny"
}

// DueKind tells how urgent a due date is: "err" when past, "warn" within
// three days, "" otherwise.
func DueKind(due, today civil.Date) string {
	switch d := today.DaysTo(due); {
	case d < 0:
		return "err"
	case d <= 3:
		return "warn"
	}
	return ""
}
