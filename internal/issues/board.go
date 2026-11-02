package issues

import (
	"cmp"
	"slices"
	"strings"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/rules"
)

// RepoInfo is what housenka's configuration says about an issue's repository.
type RepoInfo struct {
	HostKnown bool     // the GitLab is in gitlab_hosts
	Projects  []string // project codes allowed for the repository; none = the repository is not in repos
	// The badge color, as the UIs color a project: a repository with one
	// project has the project's color, any other one a color derived from
	// its path.
	ColorKey, ColorSpec string
}

// Known reports whether work on the issue can be reported.
func (r RepoInfo) Known() bool { return r.HostKnown && len(r.Projects) > 0 }

// Repo looks up the issue's repository in the configuration (r may be nil
// when the main configuration is broken).
func Repo(is Issue, r *rules.Rules) RepoInfo {
	info := RepoInfo{ColorKey: strings.ToLower(is.ProjectPath)}
	if r == nil {
		return info
	}
	cfg := r.Config()
	info.HostKnown = slices.ContainsFunc(cfg.GitLabHosts, func(h string) bool { return strings.EqualFold(h, is.Host) })
	if _, codes, ok := r.MatchRepo(is.ProjectPath); ok {
		info.Projects = codes
		if len(codes) == 1 {
			info.ColorKey, info.ColorSpec = codes[0], cfg.Projects[codes[0]].Color
		}
	}
	return info
}

// Work is the work reported on one issue.
type Work struct {
	Total int        // quarters
	Last  civil.Date // the last day with work
}

// WorkByIssue sums entries by gitlaburl.KeyOf of their issue.
func WorkByIssue(entries []entry.Entry) map[string]Work {
	out := map[string]Work{}
	for _, e := range entries {
		if e.IssueURL == "" {
			continue
		}
		k := gitlaburl.KeyOf(e.IssueURL)
		w := out[k]
		w.Total += e.Quarters
		if e.Date.After(w.Last) {
			w.Last = e.Date
		}
		out[k] = w
	}
	return out
}

// Card is an issue with everything the overview shows about it.
type Card struct {
	Issue
	Repo       RepoInfo
	Work       Work
	Column     int  // index in Config.Statuses; -1 = new
	LostStatus bool // Status is set but no longer configured
	Assigned   int  // days since the issue was assigned to the user
	Idle       int  // days since the user last did anything with the issue
	Stale      bool // idle for StaleDays or longer in a watched place
}

// Cards prepares the issues of f for display.
func Cards(f File, cfg *Config, r *rules.Rules, work map[string]Work, today civil.Date) []Card {
	cards := make([]Card, 0, len(f.Issues))
	for _, is := range f.Issues {
		c := Card{Issue: is, Repo: Repo(is, r), Work: work[is.Key], Column: cfg.StatusIndex(is.Status)}
		c.LostStatus = is.Status != "" && c.Column < 0
		last := civil.Of(is.AssignedAt.Local())
		if !is.TouchedAt.IsZero() {
			last = maxDate(last, civil.Of(is.TouchedAt.Local()))
		}
		last = maxDate(last, c.Work.Last)
		c.Idle = max(last.DaysTo(today), 0)
		c.Assigned = max(civil.Of(is.AssignedAt.Local()).DaysTo(today), 0)
		watched := c.Column < 0 || cfg.Statuses[c.Column].Watched()
		c.Stale = watched && !is.Hidden && !is.Muted && !is.Closed && !is.Unassigned && c.Idle >= cfg.StaleDays
		cards = append(cards, c)
	}
	return cards
}

func maxDate(a, b civil.Date) civil.Date {
	if b.After(a) {
		return b
	}
	return a
}

// Board is the overview: new issues, the user's statuses and hidden issues.
type Board struct {
	New     []Card
	Columns []Column
	Hidden  []Card
}

// Column is one status of the board.
type Column struct {
	Status Status
	Cards  []Card
}

// Arrange sorts cards into the board, keeping their order.
func Arrange(cards []Card, cfg *Config) Board {
	b := Board{Columns: make([]Column, len(cfg.Statuses))}
	for i, s := range cfg.Statuses {
		b.Columns[i].Status = s
	}
	for _, c := range cards {
		switch {
		case c.Hidden:
			b.Hidden = append(b.Hidden, c)
		case c.Column < 0:
			b.New = append(b.New, c)
		default:
			b.Columns[c.Column].Cards = append(b.Columns[c.Column].Cards, c)
		}
	}
	return b
}

// Lane is the cards of one repository.
type Lane struct {
	ProjectPath string
	Cards       []Card
}

// Lanes groups cards by repository (sorted by path), keeping their order.
func Lanes(cards []Card) []Lane {
	var lanes []Lane
	for _, c := range cards {
		i := slices.IndexFunc(lanes, func(l Lane) bool { return strings.EqualFold(l.ProjectPath, c.ProjectPath) })
		if i < 0 {
			lanes = append(lanes, Lane{ProjectPath: c.ProjectPath})
			i = len(lanes) - 1
		}
		lanes[i].Cards = append(lanes[i].Cards, c)
	}
	slices.SortStableFunc(lanes, func(a, b Lane) int {
		return strings.Compare(strings.ToLower(a.ProjectPath), strings.ToLower(b.ProjectPath))
	})
	return lanes
}

// Shift finds where J/K moves the card key among the cards as shown (one
// status, filtered, in board order): the neighbour to put it next to, and
// whether after it. With sameRepo (cards grouped by repository) a card does
// not leave its repository's group. ok is false at the end of the list.
func Shift(shown []Card, key string, delta int, sameRepo bool) (neighbour string, after, ok bool) {
	i := slices.IndexFunc(shown, func(c Card) bool { return c.Key == key })
	j := i + delta
	if i < 0 || delta == 0 || j < 0 || j >= len(shown) {
		return "", false, false
	}
	if sameRepo && !strings.EqualFold(shown[i].ProjectPath, shown[j].ProjectPath) || shown[i].Muted != shown[j].Muted {
		return "", false, false // a card stays in its group, and among the issues put off (or not)
	}
	return shown[j].Key, delta > 0, true
}

// Sort is an order of the cards.
type Sort string

const (
	SortManual   Sort = ""         // the user's own order (new issues: assigned first)
	SortAssigned Sort = "assigned" // assigned longest ago first
	SortIdle     Sort = "idle"     // untouched longest first
	SortDue      Sort = "due"      // due soonest first, no due date last
	SortRepo     Sort = "repo"     // by repository, then manual
)

// Sorts lists the orders as the UIs cycle through them.
var Sorts = []Sort{SortManual, SortAssigned, SortIdle, SortDue, SortRepo}

// Label is the Czech name of the order.
func (s Sort) Label() string {
	switch s {
	case SortAssigned:
		return "přiřazeno nejdřív"
	case SortIdle:
		return "nejdéle beze mě"
	case SortDue:
		return "termín"
	case SortRepo:
		return "repozitář"
	}
	return "vlastní pořadí"
}

// Next returns the order after s in Sorts.
func (s Sort) Next() Sort {
	return Sorts[(slices.Index(Sorts, s)+1)%len(Sorts)]
}

// ParseSort returns the order named s, or SortManual.
func ParseSort(s string) Sort {
	if slices.Contains(Sorts, Sort(s)) {
		return Sort(s)
	}
	return SortManual
}

// SortCards orders cards in place.
func SortCards(cards []Card, s Sort) {
	manual := func(a, b Card) int { return byRank(a.Issue, b.Issue) }
	var f func(a, b Card) int
	switch s {
	case SortAssigned:
		f = func(a, b Card) int { return cmp.Or(a.AssignedAt.Compare(b.AssignedAt), manual(a, b)) }
	case SortIdle:
		f = func(a, b Card) int { return cmp.Or(cmp.Compare(b.Idle, a.Idle), manual(a, b)) }
	case SortDue:
		f = func(a, b Card) int {
			switch {
			case a.Due == nil && b.Due == nil:
				return manual(a, b)
			case a.Due == nil:
				return 1
			case b.Due == nil:
				return -1
			}
			return cmp.Or(a.Due.Compare(*b.Due), manual(a, b))
		}
	case SortRepo:
		f = func(a, b Card) int {
			return cmp.Or(strings.Compare(strings.ToLower(a.ProjectPath), strings.ToLower(b.ProjectPath)), manual(a, b))
		}
	default:
		f = manual
	}
	slices.SortStableFunc(cards, f)
	// Issues put off go after the others, in any order.
	slices.SortStableFunc(cards, func(a, b Card) int { return cmpBool(a.Muted, b.Muted) })
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Filter keeps the cards matching query: words separated by spaces, all
// of which must match (case-insensitive). A word is
//   - r:text – the repository path contains text,
//   - p:CODE – a project code allowed for the repository starts with CODE,
//   - l:text – a GitLab label contains text,
//   - is:closed, is:unassigned, is:unknown (repository or GitLab not in
//     housenka), is:stale, is:notes, is:due, is:muted (put off),
//   - anything else – found in the title, the key, labels, notes or the path.
//
// A word starting with "-" must not match.
func Filter(cards []Card, query string) []Card {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return cards
	}
	var out []Card
	for _, c := range cards {
		if slices.IndexFunc(words, func(w string) bool { return !matchWord(c, w) }) < 0 {
			out = append(out, c)
		}
	}
	return out
}

func matchWord(c Card, w string) bool {
	if neg, ok := strings.CutPrefix(w, "-"); ok && neg != "" {
		return !matchWord(c, neg)
	}
	switch {
	case strings.HasPrefix(w, "r:"):
		return strings.Contains(strings.ToLower(c.ProjectPath), w[2:])
	case strings.HasPrefix(w, "p:"):
		return slices.ContainsFunc(c.Repo.Projects, func(p string) bool { return strings.HasPrefix(strings.ToLower(p), w[2:]) })
	case strings.HasPrefix(w, "l:"):
		return slices.ContainsFunc(c.Labels, func(l string) bool { return strings.Contains(strings.ToLower(l), w[2:]) })
	case strings.HasPrefix(w, "is:"):
		switch w[3:] {
		case "closed":
			return c.Closed
		case "unassigned":
			return c.Unassigned
		case "unknown":
			return !c.Repo.Known()
		case "stale":
			return c.Stale
		case "notes":
			return c.Notes != ""
		case "muted":
			return c.Muted
		case "due":
			return c.Due != nil
		}
		return false
	}
	text := strings.ToLower(strings.Join(append([]string{c.Title, c.Key, c.Notes, c.ProjectPath}, c.Labels...), "\n"))
	return strings.Contains(text, w)
}

// Overview is the board as the UIs show it: All counts every issue, Shown
// holds the cards matching the filter in the chosen order.
type Overview struct {
	All, Shown Board
}

// BuildOverview prepares the overview. With byRepo the cards of each
// status are grouped by repository (keeping the order within a repository).
func BuildOverview(f File, cfg *Config, r *rules.Rules, entries []entry.Entry, today civil.Date, query string, s Sort, byRepo bool) Overview {
	cards := Cards(f, cfg, r, WorkByIssue(entries), today)
	shown := Filter(cards, query)
	SortCards(shown, s)
	if byRepo {
		slices.SortStableFunc(shown, func(a, b Card) int {
			return strings.Compare(strings.ToLower(a.ProjectPath), strings.ToLower(b.ProjectPath))
		})
	}
	return Overview{All: Arrange(cards, cfg), Shown: Arrange(shown, cfg)}
}

// Total returns the number of cards on the board (all statuses).
func (b Board) Total() int {
	n := 0
	for _, c := range b.Columns {
		n += len(c.Cards)
	}
	return n
}
