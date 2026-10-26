package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/store"
)

// exampleConfig reads the example config from the repository root.
func exampleConfig(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeOpener struct{ opened, copied []string }

func (o *fakeOpener) Open(u string) error { o.opened = append(o.opened, u); return nil }
func (o *fakeOpener) Copy(t string) error { o.copied = append(o.copied, t); return nil }

var today = civil.Date{Year: 2026, Month: time.October, Day: 12}

func newTestModel(t *testing.T) (*Model, *store.Store, *fakeOpener) {
	t.Helper()
	cfg, err := config.Parse(exampleConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "entries"))
	if err != nil {
		t.Fatal(err)
	}
	o := &fakeOpener{}
	m := New(cfg, s, o, func() civil.Date { return today })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	return m, s, o
}

// press sends keys; multi-char strings are typed as text, names in <> are special keys.
func press(m *Model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "<enter>":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "<esc>":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "<tab>":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "<shift+tab>":
			msg = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
		case "<ctrl+s>":
			msg = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
		default:
			for _, r := range k {
				m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			continue
		}
		m.Update(msg)
	}
}

func screen(m *Model) string { return ansi.Strip(m.View().Content) }

func dump(t *testing.T, name string, m *Model) {
	t.Helper()
	if dir := os.Getenv("HOUSENKA_SCREENS"); dir != "" {
		os.WriteFile(filepath.Join(dir, name+".txt"), []byte(screen(m)), 0o644)
	}
}

const issueURL = "https://gitlab.example.com/clients/shop-web/-/work_items/393#note_123456"

func TestAddEntryThroughForm(t *testing.T) {
	m, s, o := newTestModel(t)
	dump(t, "01-empty", m)
	if !strings.Contains(screen(m), "Zatím nic vykázáno") {
		t.Fatalf("empty state missing:\n%s", screen(m))
	}

	press(m, "a")
	if m.mode != modeForm {
		t.Fatal("form not opened")
	}
	press(m, issueURL)
	dump(t, "02-form-issue", m)
	if got := m.form.projects; len(got) != 2 || got[0] != "SHOPDEV" {
		t.Fatalf("projects offered = %v", got)
	}
	if m.form.project != "" {
		t.Errorf("project preselected although 2 options: %q", m.form.project)
	}
	press(m, "<tab>", "l") // project: first option
	if m.form.project != "SHOPDEV" || m.form.activity != "DEV" {
		t.Fatalf("project=%q activity=%q", m.form.project, m.form.activity)
	}
	press(m, "<tab>", "<tab>", "2,75", "<tab>", "řešení JSON-LD")
	dump(t, "03-form-filled", m)
	press(m, "<ctrl+s>")
	if m.mode != modeConfirmOpen {
		t.Fatalf("mode = %v, form errors: %v", m.mode, m.form.errs)
	}
	press(m, "y")
	m.Update(m.openInBM(m.lastSaved)()) // execute the async open command
	if len(o.opened) == 0 || !strings.Contains(o.opened[0], "project=SHOPDEV") ||
		!strings.Contains(o.opened[0], "hours=2%2C75") {
		t.Errorf("opened = %v", o.opened)
	}

	got, _ := s.Range(today, today)
	if len(got) != 1 || got[0].Quarters != 11 || got[0].Description != "řešení JSON-LD" {
		t.Fatalf("stored = %+v", got)
	}
	dump(t, "04-list", m)
	scr := screen(m)
	for _, want := range []string{"shop-web", "#393", "SHOPDEV", "DEV", "2,75", "řešení JSON-LD"} {
		if !strings.Contains(scr, want) {
			t.Errorf("list missing %q:\n%s", want, scr)
		}
	}
}

func TestFormRejectsInvalid(t *testing.T) {
	m, s, _ := newTestModel(t)
	press(m, "a", "https://gitlab.example.com/clients/neznamy/-/issues/1", "<ctrl+s>")
	if m.mode != modeForm {
		t.Fatal("invalid form was saved")
	}
	if _, ok := m.form.errs[fieldIssue]; !ok {
		t.Errorf("no issue error: %v", m.form.errs)
	}
	dump(t, "05-form-errors", m)
	press(m, "<esc>")
	if m.mode != modeNormal {
		t.Error("esc did not close the form")
	}
	if got, _ := s.Range(today, today); len(got) != 0 {
		t.Errorf("stored = %+v", got)
	}
}

func seed(t *testing.T, s *store.Store, entries ...entry.Entry) {
	t.Helper()
	for _, e := range entries {
		if _, err := s.Save(e); err != nil {
			t.Fatal(err)
		}
	}
}

func sample(d civil.Date, project, activity string, q int, desc string) entry.Entry {
	e := entry.Entry{Date: d, ProjectCode: project, Activity: activity, Quarters: q, Description: desc}
	if project != "INTERNI" {
		e.IssueURL = issueURL
	}
	return e
}

func TestNavigationAndActions(t *testing.T) {
	m, s, o := newTestModel(t)
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 16, "první"),
		sample(today, "INTERNI", "OPS", 4, "porada"),
		sample(today, "SHOPSUP", "TEST", 8, "třetí"),
		sample(today.AddDays(-1), "SHOPDEV", "ANALYSIS", 12, "včerejší"),
	)
	m.reload("")
	dump(t, "06-day", m)

	if e, _ := m.selected(); e.Description != "první" {
		t.Fatalf("selected %q", e.Description)
	}
	press(m, "j", "j")
	if e, _ := m.selected(); e.Description != "třetí" {
		t.Errorf("after jj selected %q", e.Description)
	}
	press(m, "gg")
	if e, _ := m.selected(); e.Description != "první" {
		t.Errorf("after gg selected %q", e.Description)
	}
	press(m, "G")
	if e, _ := m.selected(); e.Description != "třetí" {
		t.Errorf("after G selected %q", e.Description)
	}

	press(m, "s")
	if e, _ := m.selected(); !e.Sent {
		t.Error("toggle sent failed")
	}
	press(m, "Y")
	if len(o.copied) != 0 {
		t.Error("copy must be async")
	}

	press(m, "yy", "h", "p") // yank, go to yesterday, paste
	if m.mode != modeForm || m.form.base.Date != today.AddDays(-1) || m.form.project != "SHOPSUP" {
		t.Fatalf("paste form: mode=%v date=%v project=%q", m.mode, m.form.base.Date, m.form.project)
	}
	press(m, "<ctrl+s>", "n")
	if got, _ := s.Range(today.AddDays(-1), today.AddDays(-1)); len(got) != 2 {
		t.Errorf("yesterday has %d entries", len(got))
	}

	press(m, "t", "2")
	dump(t, "07-week", m)
	if !strings.Contains(screen(m), "týden 42") {
		t.Errorf("week view missing:\n%s", screen(m))
	}
	press(m, "3")
	dump(t, "08-month", m)
	if !strings.Contains(screen(m), "říjen 2026") {
		t.Error("month view missing")
	}

	press(m, "S")
	dump(t, "09-status", m)
	if m.mode != modeStatus || !strings.Contains(screen(m), "Týdenní souhrn") {
		t.Fatalf("status modal missing:\n%s", screen(m))
	}
	press(m, "<esc>", "1", "t", "gg", "dd")
	if m.mode != modeConfirmDelete {
		t.Fatal("dd did not ask for confirmation")
	}
	dump(t, "10-confirm-delete", m)
	press(m, "y")
	if got, _ := s.Range(today, today); len(got) != 2 {
		t.Errorf("after delete today has %d entries", len(got))
	}

	press(m, "/", "porada", "<enter>")
	if len(m.entries) != 1 {
		t.Errorf("filter left %d entries", len(m.entries))
	}
	press(m, "<esc>")
	if len(m.entries) != 2 {
		t.Errorf("esc did not clear filter: %d", len(m.entries))
	}

	press(m, "?")
	dump(t, "11-help", m)
	press(m, "?")

	_, cmd := m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	_ = cmd
	press(m, "q")
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(":q did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error(":q did not return Quit")
	}
}

func TestSmallTerminal(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if !strings.Contains(screen(m), "příliš malý") {
		t.Error("small terminal warning missing")
	}
}

// A pasted link arrives as one PasteMsg, not as key presses; the allowed
// projects must be recomputed anyway.
func TestPastedIssueRefreshesProjects(t *testing.T) {
	m, _, _ := newTestModel(t)
	press(m, "a")
	if got := m.form.projects; len(got) != 1 || got[0] != "INTERNI" {
		t.Fatalf("without issue offered %v", got)
	}
	m.Update(tea.PasteMsg{Content: issueURL})
	if got := m.form.projects; len(got) != 2 || got[0] != "SHOPDEV" {
		t.Fatalf("after paste offered %v", got)
	}
	if m.form.project == "INTERNI" {
		t.Error("project without issue still selected after pasting an issue")
	}
	if m.form.issueRef == nil || m.form.issueRef.IID != 393 {
		t.Errorf("issueRef = %+v", m.form.issueRef)
	}
}

func TestMultilineDescription(t *testing.T) {
	m, s, _ := newTestModel(t)
	press(m, "a", issueURL, "<tab>", "l", "<tab>", "<tab>", "1", "<tab>")
	if m.form.focus != fieldDesc {
		t.Fatalf("focus = %v", m.form.focus)
	}
	press(m, "první řádek", "<enter>", "druhý řádek  ")
	if m.mode != modeForm {
		t.Fatal("enter in description must insert a newline, not save")
	}
	dump(t, "13-multiline-form", m)
	press(m, "<ctrl+s>")
	got, _ := s.Range(today, today)
	if len(got) != 1 || got[0].Description != "první řádek\ndruhý řádek" {
		t.Fatalf("stored description = %q", got[0].Description)
	}
	press(m, "n")
	dump(t, "14-multiline-list", m)
	if !strings.Contains(screen(m), "první řádek ↵ druhý řádek") {
		t.Errorf("list does not show flattened description:\n%s", screen(m))
	}
}

func TestFormButtons(t *testing.T) {
	m, s, _ := newTestModel(t)

	// Fill the form, Tab from the description onto "Uložit" and press Enter.
	press(m, "a", issueURL, "<tab>", "l", "<tab>", "<tab>", "1,5", "<tab>", "popis", "<tab>")
	if m.form.focus != fieldSave {
		t.Fatalf("focus after description = %v, want Save button", m.form.focus)
	}
	dump(t, "15-form-buttons", m)
	press(m, "<enter>")
	if m.mode != modeConfirmOpen {
		t.Fatalf("Save button did not save, errors: %v", m.form.errs)
	}
	if got, _ := s.Range(today, today); len(got) != 1 || got[0].Quarters != 6 {
		t.Fatalf("stored = %+v", got)
	}
	press(m, "n")

	// An invalid form stays open, focuses the first error and shows a summary.
	press(m, "a", "<tab>", "<tab>", "<tab>", "<tab>", "<tab>")
	if m.form.focus != fieldSave {
		t.Fatalf("focus = %v, want Save button", m.form.focus)
	}
	press(m, "<enter>")
	if m.mode != modeForm || m.form.focus == fieldSave {
		t.Fatalf("invalid form: mode=%v focus=%v", m.mode, m.form.focus)
	}
	if !strings.Contains(screen(m), "formulář obsahuje chyby") {
		t.Error("error summary missing next to the buttons")
	}

	// "Zrušit" closes the form without saving.
	m.form.focus = fieldSave
	press(m, "l", "<enter>")
	if m.mode != modeNormal {
		t.Fatalf("Cancel button did not close the form, mode=%v", m.mode)
	}
	if got, _ := s.Range(today, today); len(got) != 1 {
		t.Errorf("cancel saved something: %d entries", len(got))
	}
}

// Every rendered line must fit the terminal width, also in narrow terminals.
func TestLinesFitWidth(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s, sample(today, "SHOPDEV", "DEV", 16, strings.Repeat("dlouhý popis ", 20)))
	m.reload("")
	for _, w := range []int{50, 80, 120, 200} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for _, keys := range [][]string{{}, {"?"}, {"?", "a"}} {
			press(m, keys...)
			for i, line := range strings.Split(m.View().Content, "\n") {
				if got := lipgloss.Width(line); got > w {
					t.Errorf("width %d, keys %v: line %d is %d cells wide: %q", w, keys, i, got, ansi.Strip(line))
				}
			}
			press(m, "<esc>")
		}
	}
}

// The header is a colored block: no styled segment may punch a hole into
// the background, every line spans the full width, and the first and last
// lines are empty padding.
func TestHeaderBackgroundHasNoHoles(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s, sample(today, "SHOPDEV", "DEV", 16, "x"))
	m.reload("")
	probe := lipgloss.NewStyle().Background(m.st.p.Surface).Render("x")
	on := probe[:strings.Index(probe, "x")]
	lines := strings.Split(m.headerView(), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, on) {
			t.Errorf("line %d does not start with the background", i)
		}
		body := strings.TrimSuffix(line, "\x1b[m")
		for _, part := range strings.Split(body, "\x1b[m")[1:] {
			if !strings.HasPrefix(part, on) {
				t.Errorf("line %d: background not restored after a reset: %q", i, ansi.Strip(part))
			}
		}
		if w := lipgloss.Width(line); w != m.width {
			t.Errorf("line %d is %d wide, want %d", i, w, m.width)
		}
	}
	if first, last := ansi.Strip(lines[0]), ansi.Strip(lines[len(lines)-1]); strings.TrimSpace(first+last) != "" {
		t.Errorf("first/last line should be empty padding: %q / %q", first, last)
	}
}

func TestMidnightRollover(t *testing.T) {
	cfg, _ := config.Parse(exampleConfig(t))
	s, _ := store.Open(filepath.Join(t.TempDir(), "entries"))
	now := today
	m := New(cfg, s, &fakeOpener{}, func() civil.Date { return now })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})

	now = today.AddDays(1) // the app stays open over midnight
	press(m, "t")
	if m.per.Start != now {
		t.Errorf("t jumped to %v, want new today %v", m.per.Start, now)
	}
	press(m, "a")
	if m.form.base.Date != now {
		t.Errorf("new entry date = %v, want %v", m.form.base.Date, now)
	}
}

func TestEditingSentEntryMarksItUnsent(t *testing.T) {
	m, s, _ := newTestModel(t)
	e := sample(today, "SHOPDEV", "DEV", 8, "původní")
	e.Sent = true
	seed(t, s, e)
	m.reload("")

	// Saving without changes keeps it sent.
	press(m, "e", "<ctrl+s>", "n")
	if got, _ := s.Range(today, today); !got[0].Sent {
		t.Fatal("unchanged entry lost its sent flag")
	}
	// Changing hours means BM has an outdated version.
	press(m, "e")
	m.form.focus = fieldHours
	m.form.focusCmd()
	m.form.inputs[fieldHours].SetValue("3")
	press(m, "<ctrl+s>")
	if got, _ := s.Range(today, today); got[0].Sent || got[0].Quarters != 12 {
		t.Errorf("after edit: sent=%v quarters=%d", got[0].Sent, got[0].Quarters)
	}
	if !m.toastErr || !strings.Contains(m.toast, "uprav ho i v BM") {
		t.Errorf("toast = %q", m.toast)
	}
}

func TestPasteIntoFilter(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s, sample(today, "SHOPDEV", "DEV", 8, "košík"), sample(today, "INTERNI", "OPS", 4, "porada"))
	m.reload("")
	press(m, "/")
	m.Update(tea.PasteMsg{Content: "porada"})
	if len(m.entries) != 1 || m.entries[0].Description != "porada" {
		t.Errorf("paste did not filter: %d entries", len(m.entries))
	}
}

func TestDeleteKeepsCursorPosition(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 4, "a"),
		sample(today, "SHOPDEV", "DEV", 4, "b"),
		sample(today, "SHOPDEV", "DEV", 4, "c"),
	)
	m.reload("")
	press(m, "j", "dd", "y")
	if e, _ := m.selected(); e.Description != "c" {
		t.Errorf("after deleting b selected %q, want c", e.Description)
	}
	press(m, "dd", "y")
	if e, _ := m.selected(); e.Description != "a" {
		t.Errorf("after deleting the last entry selected %q, want a", e.Description)
	}
}

func TestOpenIssue(t *testing.T) {
	m, s, o := newTestModel(t)
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 4, "s issue"),
		sample(today, "INTERNI", "OPS", 4, "bez issue"),
	)
	m.reload("")

	for _, k := range []string{"i", "gx"} {
		var cmd tea.Cmd
		for _, r := range k {
			_, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if cmd == nil {
			t.Fatalf("%s returned no command", k)
		}
		m.Update(cmd())
	}
	if len(o.opened) != 2 || o.opened[0] != issueURL || o.opened[1] != issueURL {
		t.Errorf("opened = %v", o.opened)
	}
	if m.toastErr || !strings.Contains(m.toast, "Issue") {
		t.Errorf("toast = %q (err %v)", m.toast, m.toastErr)
	}

	press(m, "j", "i")
	if len(o.opened) != 2 || !m.toastErr {
		t.Errorf("entry without issue: opened = %v, toast = %q", o.opened, m.toast)
	}
}

// TestThemeBackgroundCoversEveryCell renders the screen in each mode and
// checks no cell falls back to the terminal's default background.
func TestThemeBackgroundCoversEveryCell(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 16, "první"),
		sample(today.AddDays(-1), "INTERNI", "OPS", 4, "porada"),
	)
	m.reload("")
	check := func(name string) {
		t.Helper()
		buf := uv.NewScreenBuffer(m.width, m.height)
		uv.NewStyledString(m.View().Content).Draw(buf, uv.Rect(0, 0, m.width, m.height))
		for y := range m.height {
			for x := range m.width {
				// Width 0 is the second half of a wide character (🐛).
				if c := buf.CellAt(x, y); c != nil && c.Width > 0 && c.Style.Bg == nil {
					t.Errorf("%s: cell %d,%d has no background", name, x, y)
					return
				}
			}
		}
	}
	check("day")
	press(m, "2")
	check("week")
	press(m, "?")
	check("help")
	press(m, "?", "a")
	check("form")
	press(m, "<shift+tab>")
	m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	check("calendar")
	press(m, "<esc>", "<esc>", "S")
	check("status")
	press(m, "<esc>", "dd")
	check("confirm")
	press(m, "n")

	m.cfg.UI.Background = false
	probe := lipgloss.NewStyle().Background(m.st.p.Base).Render("x")
	if v := m.View(); v.BackgroundColor != nil || strings.Contains(v.Content, probe[:strings.Index(probe, "x")]) {
		t.Error("background painted although disabled")
	}
}

func TestDateFormat(t *testing.T) {
	m, s, _ := newTestModel(t)
	m.cfg.UI.DateFormat = "yyyy-mm-dd"
	m = New(m.cfg, s, &fakeOpener{}, func() civil.Date { return today })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	seed(t, s, sample(today, "SHOPDEV", "DEV", 4, "x"))
	m.reload("")
	if !strings.Contains(screen(m), "po 2026-10-12") {
		t.Errorf("header does not use the date format:\n%s", screen(m))
	}
	press(m, "e")
	if got := m.form.inputs[fieldDate].Value(); got != "2026-10-12" {
		t.Errorf("form date = %q", got)
	}
	press(m, "<esc>", "2")
	if !strings.Contains(screen(m), "2026-10-12 – 2026-10-18") {
		t.Errorf("week range does not use the date format:\n%s", screen(m))
	}
}

func TestCalendarPicksDate(t *testing.T) {
	m, _, _ := newTestModel(t)
	press(m, "a", "<shift+tab>") // the form starts on the issue field
	ctrlK := tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
	m.Update(ctrlK)
	if m.form.cal == nil {
		t.Fatal("ctrl+k did not open the calendar")
	}
	dump(t, "12-calendar", m)
	if sc := screen(m); !strings.Contains(sc, "říjen 2026") || !strings.Contains(sc, "po út st čt pá so ne") {
		t.Errorf("calendar not shown:\n%s", sc)
	}
	press(m, "k", "h", "<enter>") // a week back, a day back
	if m.form.cal != nil {
		t.Fatal("enter did not close the calendar")
	}
	if got := m.form.inputs[fieldDate].Value(); got != "4. 10. 2026" {
		t.Errorf("picked %q, want 4. 10. 2026", got)
	}

	m.Update(ctrlK)
	press(m, "L", "<esc>") // esc closes only the calendar
	if m.mode != modeForm || m.form.cal != nil || m.form.inputs[fieldDate].Value() != "4. 10. 2026" {
		t.Fatalf("esc: mode=%v cal=%v date=%q", m.mode, m.form.cal != nil, m.form.inputs[fieldDate].Value())
	}

	// Opens on the date in the field; months clamp the day.
	m.form.inputs[fieldDate].SetValue("31. 1. 2026")
	m.Update(ctrlK)
	press(m, "L", "<enter>")
	if got := m.form.inputs[fieldDate].Value(); got != "28. 2. 2026" {
		t.Errorf("next month from 31. 1. = %q", got)
	}
}

func TestCalendarOnlyInDateField(t *testing.T) {
	m, _, _ := newTestModel(t)
	press(m, "a") // focus on the issue field
	m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.form.cal != nil {
		t.Error("calendar opened outside the date field")
	}
}

func TestProjectColorFromConfig(t *testing.T) {
	m, s, _ := newTestModel(t)
	p := m.cfg.Projects["SHOPDEV"]
	p.Color = "red"
	m.cfg.Projects["SHOPDEV"] = p
	m = New(m.cfg, s, &fakeOpener{}, func() civil.Date { return today })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	seed(t, s, sample(today, "SHOPDEV", "DEV", 4, "x"))
	m.reload("")

	red := m.st.p.Colors["red"]
	probe := lipgloss.NewStyle().Background(red).Render("x")
	on := probe[:strings.Index(probe, "x")]
	if !strings.Contains(m.View().Content, on) {
		t.Error("list does not draw SHOPDEV in red")
	}
	press(m, "e")
	if !strings.Contains(m.View().Content, on) {
		t.Error("form does not draw the selected project in red")
	}
}

func TestCurrentPeriodIsMarked(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 4, "dnešní"),
		sample(today.AddDays(-1), "SHOPDEV", "DEV", 4, "včerejší"),
	)
	m.reload("")
	header := func() string { return strings.Split(ansi.Strip(m.headerView()), "\n")[1] }

	for _, tt := range []struct{ keys, want string }{{"1", "dnes"}, {"2", "tento týden"}, {"3", "tento měsíc"}} {
		press(m, tt.keys)
		if !strings.Contains(header(), tt.want) {
			t.Errorf("%s: header %q lacks %q", tt.keys, header(), tt.want)
		}
	}
	if !strings.Contains(screen(m), "po 12. 10. 2026  dnes") || strings.Contains(screen(m), "ne 11. 10. 2026  dnes") {
		t.Errorf("only today's day header should be marked:\n%s", screen(m))
	}
	press(m, "h") // previous month
	if h := header(); !strings.Contains(h, "září 2026  tento měsíc →") {
		t.Errorf("past month header = %q", h)
	}
	press(m, "l", "l") // next month
	if h := header(); !strings.Contains(h, "← tento měsíc") {
		t.Errorf("future month header = %q", h)
	}
	press(m, "t", "1", "h")
	if h := header(); !strings.Contains(h, "dnes →") {
		t.Errorf("yesterday header = %q", h)
	}
	press(m, "t", "3", "t", "2") // t selects today's entry, so the week is the current one
	if h := header(); !strings.Contains(h, "týden 42") || !strings.Contains(h, "tento týden") {
		t.Errorf("t then 2: header = %q", h)
	}
}

func TestIssueSummary(t *testing.T) {
	m, s, o := newTestModel(t)
	lastYear := civil.Date{Year: 2025, Month: time.December, Day: 18}
	seed(t, s,
		sample(today, "SHOPDEV", "DEV", 8, "dnešní práce"),
		sample(today.AddDays(-1), "SHOPSUP", "TEST", 4, "testování"),
		sample(lastYear, "SHOPDEV", "ANALYSIS", 6, "loňská analýza"),
		sample(today, "INTERNI", "OPS", 4, "porada bez issue"),
	)
	m.reload("")
	press(m, "I")
	if m.mode != modeIssue {
		t.Fatalf("I did not open the issue summary (mode %v)", m.mode)
	}
	dump(t, "13-issue", m)
	sc := screen(m)
	for _, want := range []string{"Issue shop-web#393", "celkem 4,50 h", "3 výkazy", "3 dny",
		"DEV", "ANALYSIS", "TEST", "Kódy projektů", "loňská analýza", "testování"} {
		if !strings.Contains(sc, want) {
			t.Errorf("summary lacks %q:\n%s", want, sc)
		}
	}
	if strings.Contains(sc, "porada bez issue") {
		t.Error("entry without the issue is listed")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m.Update(cmd())
	if len(o.opened) != 1 || !strings.HasPrefix(o.opened[0], "https://gitlab.example.com/clients/shop-web/-/work_items/393") {
		t.Errorf("opened = %v", o.opened)
	}
	press(m, "<esc>")
	if m.mode != modeNormal {
		t.Error("esc did not close the summary")
	}

	press(m, "j", "I") // the entry without an issue
	if m.mode != modeNormal || !m.toastErr {
		t.Errorf("entry without issue: mode %v, toast %q", m.mode, m.toast)
	}
}

// An entry changed in another process (the web interface) is not
// overwritten by the TUI's older copy.
func TestStaleEntryIsNotOverwritten(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s, entry.Entry{Date: m.today, IssueURL: "https://gitlab.example.com/clients/shop-web/-/issues/1",
		ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 8, Description: "práce"})
	m.reload("")

	other, err := store.Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	all, _ := other.All()
	all[0].Quarters = 12
	if _, err := other.Save(all[0]); err != nil {
		t.Fatal(err)
	}

	press(m, "s")
	got, _ := other.All()
	if got[0].Quarters != 12 || got[0].Sent {
		t.Errorf("stored = %+v", got[0])
	}
	if !strings.Contains(m.toast, "změnil v jiném okně") || m.entries[0].Quarters != 12 {
		t.Errorf("toast %q, shown %+v", m.toast, m.entries[0])
	}
}

// Saving the form over a version changed elsewhere keeps the typed values
// and saves on the second try.
func TestStaleFormKeepsTypedValues(t *testing.T) {
	m, s, _ := newTestModel(t)
	seed(t, s, entry.Entry{Date: m.today, IssueURL: "https://gitlab.example.com/clients/shop-web/-/issues/1",
		ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 8, Description: "práce"})
	m.reload("")
	press(m, "e")

	other, _ := store.Open(s.Dir())
	all, _ := other.All()
	all[0].Quarters = 12
	other.Save(all[0])

	m.form.inputs[fieldHours].SetValue("5")
	press(m, "<ctrl+s>")
	if m.mode != modeForm || m.form == nil || m.form.inputs[fieldHours].Value() != "5" {
		t.Fatalf("form closed or lost input: mode %v, form %v, toast %q", m.mode, m.form != nil, m.toast)
	}
	if !strings.Contains(m.toast, "formulář drží tvoje úpravy") {
		t.Errorf("toast = %q", m.toast)
	}
	if got, _ := other.All(); got[0].Quarters != 12 {
		t.Errorf("stored = %+v", got[0])
	}
	press(m, "<ctrl+s>")
	if got, _ := other.All(); got[0].Quarters != 20 {
		t.Errorf("second save stored %+v", got[0])
	}
}
