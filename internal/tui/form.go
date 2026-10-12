package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/gitlaburl"
	"github.com/just-tomdo/housenka/internal/hours"
	"github.com/just-tomdo/housenka/internal/period"
	"github.com/just-tomdo/housenka/internal/rules"
)

type field int

const (
	fieldDate field = iota
	fieldIssue
	fieldProject
	fieldActivity
	fieldHours
	fieldDesc
	fieldSave   // button
	fieldCancel // button
	fieldCount
)

// fieldLabels label the data fields (everything before the buttons).
var fieldLabels = [fieldSave]string{"Datum", "Issue", "Projekt", "Činnost", "Hodiny", "Popis"}

var fieldOf = map[rules.Field]field{
	rules.FieldDate: fieldDate, rules.FieldIssue: fieldIssue, rules.FieldProject: fieldProject,
	rules.FieldActivity: fieldActivity, rules.FieldHours: fieldHours, rules.FieldDescription: fieldDesc,
}

type formOutcome int

const (
	formOpen formOutcome = iota
	formCancelled
	formSaved
)

// form edits one entry. Choices offered for project and activity are always
// limited to what the rules allow for the current issue link and project.
type form struct {
	rules  *rules.Rules
	st     styles
	keys   formKeyMap
	today  civil.Date
	layout civil.Layout // date format
	title  string
	base   entry.Entry // entry being edited (ID, Sent, timestamps are kept)

	focus  field
	inputs map[field]*textinput.Model // single-line fields
	desc   textarea.Model             // multi-line description

	projects   []string
	project    string
	activities []string
	activity   string

	issueRef *gitlaburl.Ref
	issueErr error
	errs     map[field]string
	warns    map[field]string
	width    int
	result   entry.Entry
}

func newForm(r *rules.Rules, st styles, today civil.Date, e entry.Entry, title string) *form {
	layout := r.Config().UI.DateLayout()
	f := &form{
		rules: r, st: st, keys: newFormKeyMap(), today: today, layout: layout, title: title, base: e,
		inputs: map[field]*textinput.Model{},
		errs:   map[field]string{}, warns: map[field]string{},
		project: e.ProjectCode, activity: e.Activity,
	}
	mk := func(fl field, value, placeholder string) {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.SetValue(value)
		f.inputs[fl] = &ti
	}
	date := ""
	if e.Date.Valid() {
		date = layout.Format(e.Date)
	}
	hrs := ""
	if e.Quarters > 0 {
		hrs = hours.Format(e.Quarters)
	}
	mk(fieldDate, date, layout.Format(today)+" · dnes · včera · -1")
	mk(fieldIssue, e.IssueURL, "https://gitlab.example.com/skupina/repo/-/work_items/123")
	mk(fieldHours, hrs, "2,75 · 2.5 · 1:45")
	f.desc = newDescArea(st, e.Description)
	f.refreshProjects()

	// Start where typing is most likely needed.
	f.focus = fieldIssue
	if e.IssueURL != "" {
		f.focus = fieldHours
	}
	f.setWidth(70)
	return f
}

// descHeight is the number of visible lines of the description.
const descHeight = 4

func newDescArea(st styles, value string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.Placeholder = "co jsem dělal (enter = nový řádek)"
	ta.SetHeight(descHeight)
	s := ta.Styles()
	for _, state := range []*textarea.StyleState{&s.Focused, &s.Blurred} {
		state.CursorLine = lipgloss.NewStyle()
		state.EndOfBuffer = lipgloss.NewStyle()
		state.Placeholder = st.Muted
		state.Text = st.Text
	}
	s.Blurred.Text = st.Subtext
	ta.SetStyles(s)
	ta.SetValue(value)
	return ta
}

// Init returns the command starting the cursor blink.
func (f *form) Init() tea.Cmd { return f.focusCmd() }

func (f *form) setWidth(w int) {
	f.width = w
	for fl, ti := range f.inputs {
		if fl == fieldDate || fl == fieldHours {
			ti.SetWidth(30)
		} else {
			ti.SetWidth(max(w-14, 20))
		}
	}
	f.desc.SetWidth(max(w-14, 20))
}

func (f *form) focusCmd() tea.Cmd {
	var cmd tea.Cmd
	for fl, ti := range f.inputs {
		if fl == f.focus {
			cmd = ti.Focus()
		} else {
			ti.Blur()
		}
	}
	if f.focus == fieldDesc {
		cmd = f.desc.Focus()
	} else {
		f.desc.Blur()
	}
	return cmd
}

func (f *form) isSelect(fl field) bool { return fl == fieldProject || fl == fieldActivity }

func (f *form) isButton(fl field) bool { return fl == fieldSave || fl == fieldCancel }

// Update handles a message and reports whether the form is still open.
func (f *form) Update(msg tea.Msg) (formOutcome, tea.Cmd) {
	km, isKey := msg.(tea.KeyPressMsg)
	if isKey {
		switch {
		case key.Matches(km, f.keys.Cancel):
			return formCancelled, nil
		case key.Matches(km, f.keys.Save):
			return f.submit()
		case f.isButton(f.focus):
			switch km.String() {
			case "enter", "space":
				if f.focus == fieldSave {
					return f.submit()
				}
				return formCancelled, nil
			case "left", "right", "h", "l":
				f.focus = fieldSave + fieldCancel - f.focus // toggle between the two buttons
				return formOpen, nil
			case "tab", "down", "j":
				return formOpen, f.move(1)
			case "shift+tab", "up", "k":
				return formOpen, f.move(-1)
			}
			return formOpen, nil
		case f.focus == fieldDesc:
			// Enter and arrows belong to the text area; only Tab leaves it.
			switch km.String() {
			case "tab":
				return formOpen, f.move(1)
			case "shift+tab":
				return formOpen, f.move(-1)
			}
		case key.Matches(km, f.keys.Next), km.String() == "down" && !f.isSelect(f.focus):
			return formOpen, f.move(1)
		case key.Matches(km, f.keys.Prev), km.String() == "up" && !f.isSelect(f.focus):
			return formOpen, f.move(-1)
		}
	}

	if f.focus == fieldDesc {
		before := f.desc.Value()
		var cmd tea.Cmd
		f.desc, cmd = f.desc.Update(msg)
		if f.desc.Value() != before {
			delete(f.errs, fieldDesc)
		}
		return formOpen, cmd
	}

	if isKey && f.isSelect(f.focus) {
		switch km.String() {
		case "l", "right", "j", "down":
			f.cycleOption(1)
		case "h", "left", "k", "up":
			f.cycleOption(-1)
		}
		return formOpen, nil
	}

	ti, ok := f.inputs[f.focus]
	if !ok {
		return formOpen, nil
	}
	// Any message may change the value – typed keys as well as a paste.
	before := ti.Value()
	var cmd tea.Cmd
	*ti, cmd = ti.Update(msg)
	if ti.Value() != before {
		delete(f.errs, f.focus)
		if f.focus == fieldIssue {
			f.refreshProjects()
		}
	}
	return formOpen, cmd
}

// move focuses the next/previous field, skipping empty selects.
func (f *form) move(delta int) tea.Cmd {
	next := f.focus
	for range fieldCount {
		next = (next + field(delta) + fieldCount) % fieldCount
		if next == fieldProject && len(f.projects) == 0 || next == fieldActivity && len(f.activities) == 0 {
			continue
		}
		break
	}
	f.focus = next
	return f.focusCmd()
}

func (f *form) cycleOption(delta int) {
	opts, cur := f.projects, f.project
	if f.focus == fieldActivity {
		opts, cur = f.activities, f.activity
	}
	if len(opts) == 0 {
		return
	}
	i := slices.Index(opts, cur)
	switch {
	case i < 0 && delta > 0:
		i = 0
	case i < 0:
		i = len(opts) - 1
	default:
		i = (i + delta + len(opts)) % len(opts)
	}
	if f.focus == fieldProject {
		f.project = opts[i]
		delete(f.errs, fieldProject)
		f.refreshActivities()
	} else {
		f.activity = opts[i]
		delete(f.errs, fieldActivity)
	}
}

// refreshProjects recomputes allowed project codes from the issue link.
func (f *form) refreshProjects() {
	codes, ref, err := f.rules.AllowedProjects(f.inputs[fieldIssue].Value())
	f.projects, f.issueRef, f.issueErr = codes, ref, err
	switch {
	case slices.Contains(codes, f.project):
	case len(codes) == 1:
		f.project = codes[0]
	default:
		f.project = ""
	}
	f.refreshActivities()
}

// refreshActivities recomputes allowed activities for the selected project.
func (f *form) refreshActivities() {
	if f.project == "" {
		f.activities, f.activity = nil, ""
		return
	}
	f.activities = f.rules.AllowedActivities(f.project)
	if !slices.Contains(f.activities, f.activity) {
		f.activity = f.rules.DefaultActivity(f.project)
	}
}

// submit validates the form; on success the entry is in f.result.
func (f *form) submit() (formOutcome, tea.Cmd) {
	clear(f.errs)
	clear(f.warns)
	e := f.base
	e.IssueURL = strings.TrimSpace(f.inputs[fieldIssue].Value())
	e.ProjectCode = f.project
	e.Activity = f.activity
	e.Description = cleanDescription(f.desc.Value())

	d, err := civil.ParseHuman(f.inputs[fieldDate].Value(), f.today, f.layout)
	if err != nil {
		f.errs[fieldDate] = err.Error()
	}
	e.Date = d
	q, err := hours.Parse(f.inputs[fieldHours].Value())
	if err != nil {
		f.errs[fieldHours] = err.Error()
	}
	e.Quarters = q

	for _, fd := range f.rules.Validate(e, f.today).Findings {
		fl := fieldOf[fd.Field]
		if _, taken := f.errs[fl]; taken {
			continue
		}
		if fd.Warning {
			f.warns[fl] = fd.Message
		} else {
			f.errs[fl] = fd.Message
		}
	}
	if len(f.errs) > 0 {
		for fl := range fieldCount {
			if _, bad := f.errs[fl]; bad {
				f.focus = fl
				break
			}
		}
		return formOpen, f.focusCmd()
	}
	f.result = e
	return formSaved, nil
}

// View renders the form body (without the modal frame).
func (f *form) View() string {
	st := f.st
	var b strings.Builder
	b.WriteString(st.Title.Render(f.title) + "\n\n")

	for fl := range fieldSave {
		label := st.Label.Render(fieldLabels[fl])
		marker := "  "
		if fl == f.focus {
			label = st.LabelFocused.Render(fieldLabels[fl])
			marker = st.Accent.Render("▸ ")
		}
		b.WriteString(marker + label + f.fieldView(fl) + "\n")
		if hint := f.hint(fl); hint != "" {
			indent := strings.Repeat(" ", 12)
			wrapped := ansi.Wrap(hint, max(f.width-12, 20), " ")
			b.WriteString(indent + strings.ReplaceAll(wrapped, "\n", "\n"+indent) + "\n")
		}
	}
	b.WriteString("\n" + f.buttonsView())
	return b.String()
}

// buttonsView renders the Save and Cancel buttons, plus an error summary
// after a failed save.
func (f *form) buttonsView() string {
	st := f.st
	save, cancel := st.Button.Render("Uložit"), st.Button.Render("Zrušit")
	switch f.focus {
	case fieldSave:
		save = st.ButtonSave.Render("▸ Uložit")
	case fieldCancel:
		cancel = st.ButtonCancel.Render("▸ Zrušit")
	}
	line := strings.Repeat(" ", 12) + save + "  " + cancel
	if n := len(f.errs); n > 0 {
		line += "   " + st.Err.Render(fmt.Sprintf("✗ formulář obsahuje chyby (%d)", n))
	}
	return line
}

func (f *form) fieldView(fl field) string {
	switch fl {
	case fieldProject:
		if len(f.projects) == 0 {
			return f.st.Muted.Render("— nejdřív zadej platný odkaz na issue —")
		}
		return f.options(f.projects, f.project, fl == f.focus)
	case fieldActivity:
		if len(f.activities) == 0 {
			return f.st.Muted.Render("— nejdřív vyber projekt —")
		}
		return f.options(f.activities, f.activity, fl == f.focus)
	case fieldDesc:
		return strings.ReplaceAll(f.desc.View(), "\n", "\n"+strings.Repeat(" ", 12))
	default:
		return f.inputs[fl].View()
	}
}

// options renders choices as chips, wrapping to the available width.
func (f *form) options(opts []string, selected string, focused bool) string {
	var lines []string
	line := ""
	limit := max(f.width-14, 20)
	for _, o := range opts {
		style := f.st.Option
		if o == selected {
			style = f.st.OptionActive
			if focused {
				style = f.st.OptionFocused
			}
		}
		chip := style.Render(o)
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(chip) > limit {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += chip
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n"+strings.Repeat(" ", 12))
}

// hint is the line under a field: an error, a warning, or helpful context.
func (f *form) hint(fl field) string {
	st := f.st
	if msg, ok := f.errs[fl]; ok {
		return st.Err.Render("✗ " + oneLine(msg))
	}
	if msg, ok := f.warns[fl]; ok {
		return st.Warn.Render("! " + msg)
	}
	cfg := f.rules.Config()
	switch fl {
	case fieldDate:
		if d, err := civil.ParseHuman(f.inputs[fieldDate].Value(), f.today, f.layout); err == nil {
			return st.Muted.Render(period.WeekdayShort(d.Weekday()) + " " + f.layout.Format(d))
		}
	case fieldIssue:
		switch {
		case f.issueRef != nil && f.issueErr == nil:
			return st.Good.Render("✓ " + f.issueRef.Short())
		case f.issueErr != nil && strings.TrimSpace(f.inputs[fieldIssue].Value()) != "":
			return st.Err.Render("✗ " + oneLine(f.issueErr.Error()))
		case f.issueErr == nil:
			return st.Muted.Render("bez issue – jen projekty s issue: optional")
		}
	case fieldProject:
		if p, ok := cfg.Projects[f.project]; ok {
			return st.Muted.Render(p.Name)
		}
	case fieldActivity:
		if a, ok := cfg.Activities[f.activity]; ok {
			kind := st.Good.Render("produktivní")
			if !a.IsProductive() {
				kind = st.Bad.Render("neproduktivní")
			}
			return st.Muted.Render(a.Name+" · ") + kind
		}
	case fieldHours:
		if q, err := hours.Parse(f.inputs[fieldHours].Value()); err == nil && q > 0 {
			return st.Muted.Render(fmt.Sprintf("= %d h %02d min", q/4, q%4*15))
		} else if err != nil && f.inputs[fieldHours].Value() != "" {
			return st.Err.Render("✗ " + err.Error())
		}
	}
	return ""
}

// cleanDescription trims trailing spaces of every line and surrounding empty lines.
func cleanDescription(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}

// oneLine joins a multi-line message into one line for wrapping.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
