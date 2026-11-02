package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
)

const issuesTestYAML = `
statuses:
  - {code: todo, name: Na řadě, color: blue}
  - {code: doing, name: Rozpracováno, color: orange}
  - {code: done, name: Hotovo, watch: false}
`

// enableIssues writes issues.yaml and the given issues into the home.
func (f *fixture) enableIssues(list ...issues.Issue) *issues.Store {
	f.t.Helper()
	home := os.Getenv(config.HomeEnv)
	if err := os.WriteFile(filepath.Join(home, "issues.yaml"), []byte(issuesTestYAML), 0o644); err != nil {
		f.t.Fatal(err)
	}
	st, err := issues.OpenStore(filepath.Join(home, "issues"))
	if err != nil {
		f.t.Fatal(err)
	}
	if err := st.Update(func(fl *issues.File, _ time.Time) error {
		fl.Issues = append(fl.Issues, list...)
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
	return st
}

func testIssue(path string, iid int, title string) issues.Issue {
	u := "https://gitlab.example.com/" + path + "/-/issues/" + itoa(iid)
	assigned := time.Date(2026, 10, 12, 9, 0, 0, 0, time.Local)
	return issues.Issue{
		Key: strings.ToLower(path) + "#" + itoa(iid), Host: "gitlab.example.com", ProjectPath: path, IID: iid,
		Title: title, URL: u, AssignedAt: assigned, FirstSeen: assigned,
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func (f *fixture) form(target string, form url.Values) string {
	f.t.Helper()
	form.Set("v_tab", "board")
	code, _, body := f.do(http.MethodPost, target, form)
	if code != http.StatusOK {
		f.t.Fatalf("POST %s = %d: %s", target, code, body)
	}
	return body
}

func TestIssuesOff(t *testing.T) {
	f := newFixture(t)
	lacks(t, f.get("/"), `href="/issues"`)
	if code, _, _ := f.do(http.MethodGet, "/issues", nil); code != http.StatusNotFound {
		t.Errorf("GET /issues without issues.yaml = %d", code)
	}
	if code, _, _ := f.do(http.MethodPost, "/issues/status", url.Values{"key": {"a#1"}, "status": {"todo"}}); code != http.StatusNotFound {
		t.Errorf("POST without issues.yaml = %d", code)
	}
}

func TestIssuesPage(t *testing.T) {
	f := newFixture(t)
	st := f.enableIssues(
		testIssue("clients/shop-web", 12, "Košík padá"),
		testIssue("clients/shop-web", 13, "Nový filtr"),
		testIssue("other/tool", 5, "Cizí repozitář"),
	)
	f.save(sample(today, "práce na košíku"))

	// The section link counts the new issues; T switches between the pages.
	contains(t, f.get("/"), `href="/issues"`, `data-key="T"`, `>3</span>`)

	page := f.get("/issues")
	contains(t, page, `id="issues"`, "Košík padá", "Cizí repozitář", `name="v_tab" value="new"`,
		"repozitář není v housence", `2,75 h`, // 11 quarters reported on #12
		`hx-get="/issues/detail?key=clients%2Fshop-web%2312"`, `/entries/new?issue=https%3A%2F%2Fgitlab.example.com`)

	// Sort an issue onto the board, then move it.
	body := f.form("/issues/status", url.Values{"key": {"clients/shop-web#12"}, "status": {"todo"}})
	contains(t, body, `hx-swap-oob="true"`, "shop-web#12 → Na řadě")
	f.form("/issues/status", url.Values{"key": {"clients/shop-web#13"}, "status": {"todo"}, "before": {"clients/shop-web#12"}})
	board := f.get("/issues?tab=board")
	if strings.Index(board, "Nový filtr") > strings.Index(board, "Košík padá") {
		t.Error("#13 must be before #12")
	}
	contains(t, board, `data-status="todo"`, `data-key="L"`, `data-key="J"`)
	f.form("/issues/move", url.Values{"key": {"clients/shop-web#13"}, "delta": {"1"}})
	board = f.get("/issues?tab=board")
	if strings.Index(board, "Nový filtr") < strings.Index(board, "Košík padá") {
		t.Error("move did not reorder")
	}
	if code, _, body := f.do(http.MethodPost, "/issues/status", url.Values{"key": {"clients/shop-web#12"}, "status": {"nope"}}); code != http.StatusOK || !strings.Contains(body, "neznámý stav") {
		t.Errorf("unknown status: %d %s", code, body)
	}

	// Lanes and filter.
	contains(t, f.get("/issues?tab=board&lanes=1"), `class="lane"`, "clients/shop-web")
	lacks(t, f.get("/issues?tab=new&q=r:shop"), "Cizí repozitář")

	// Hide and bring back.
	f.form("/issues/hidden", url.Values{"key": {"other/tool#5"}, "hidden": {"1"}})
	contains(t, f.get("/issues?tab=hidden"), "Cizí repozitář", "↺ Vrátit")
	f.form("/issues/hidden", url.Values{"key": {"other/tool#5"}, "hidden": {"0"}})
	lacks(t, f.get("/issues?tab=hidden"), "Cizí repozitář")

	// Detail with work, notes and the hint for an unknown repository.
	detail := f.get("/issues/detail?key=clients%2Fshop-web%2312")
	contains(t, detail, "Můj stav", "● Na řadě", "Vykázaná práce", "práce na košíku")
	contains(t, strings.ToLower(f.get("/issues/detail?key=other%2Ftool%235")), "přidat repozitář do housenky", "/settings/repos?add=https%3a%2f%2fgitlab.example.com%2fother%2ftool")

	body = f.form("/issues/notes", url.Values{"key": {"clients/shop-web#12"}, "notes": {"zeptat se na API"}, "version": {time.Time{}.Format(time.RFC3339Nano)}})
	contains(t, body, "uloženy")
	body = f.form("/issues/notes", url.Values{"key": {"clients/shop-web#12"}, "notes": {"jiný text"}, "version": {time.Time{}.Format(time.RFC3339Nano)}})
	contains(t, body, "změnily v jiném okně", "jiný text")
	fl, _ := st.Load()
	if fl.Issues[0].Notes != "zeptat se na API" {
		t.Errorf("notes %q", fl.Issues[0].Notes)
	}
	contains(t, f.get("/issues?tab=board"), "✎ poznámka")
}

func TestReportFromIssue(t *testing.T) {
	f := newFixture(t)
	f.enableIssues(testIssue("clients/shop-web", 12, "Košík padá"))
	form := f.get("/entries/new?issue=" + url.QueryEscape(issue))
	contains(t, form, `value="`+issue+`"`, `id="p-SHOPDEV"`)

	// Saving from the issue page rebuilds that page, not the entries.
	body := f.form("/entries", url.Values{"date": {"19. 10. 2026"}, "issue": {issue}, "project": {"SHOPDEV"}, "activity": {"DEV"}, "hours": {"1,5"}, "description": {"oprava"}})
	contains(t, body, `id="issues"`, "Uloženo")
	lacks(t, body, `id="main"`)
}

func TestIssuesBrokenConfig(t *testing.T) {
	f := newFixture(t)
	f.enableIssues()
	os.WriteFile(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"), []byte("statuses: []\n"), 0o644)
	contains(t, f.get("/issues"), "issues.yaml obsahuje chyby", "aspoň jeden stav")
	contains(t, f.get("/"), `href="/issues"`) // the entries still work
	if code, _, body := f.do(http.MethodPost, "/issues/status", url.Values{"key": {"a#1"}, "status": {"todo"}}); code != http.StatusOK || !strings.Contains(body, "aspoň jeden stav") {
		t.Errorf("POST with a broken issues.yaml: %d %s", code, body)
	}
}

func TestIssuesCSRF(t *testing.T) {
	f := newFixture(t)
	f.enableIssues(testIssue("clients/shop-web", 12, "Košík"))
	r := httptest.NewRequest(http.MethodPost, "/issues/hidden", strings.NewReader("key=clients%2Fshop-web%2312&hidden=1"))
	r.Host = host
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("POST without origin and token = %d", w.Code)
	}
}

// fakeSource is a GitLab with one assigned issue.
type fakeSource struct{}

func (fakeSource) Host() string { return "gitlab.example.com" }

func (fakeSource) CurrentUser(context.Context) (gitlab.User, error) {
	return gitlab.User{Username: "jnovak"}, nil
}

func (fakeSource) AssignedIssues(context.Context) ([]gitlab.Issue, error) {
	return []gitlab.Issue{{IID: 40, Title: "Z GitLabu", State: "opened", WebURL: "https://gitlab.example.com/clients/shop-web/-/issues/40"}}, nil
}

func (fakeSource) Issue(context.Context, string, int) (gitlab.Issue, error) {
	return gitlab.Issue{}, gitlab.ErrNotFound
}

func (fakeSource) AssignedAt(context.Context, string, int, string) (time.Time, error) {
	return time.Time{}, nil
}

func TestIssuesSync(t *testing.T) {
	f := newFixture(t)
	f.enableIssues()
	t.Setenv(gitlab.TokenEnv, "")
	body := f.form("/issues/sync", url.Values{})
	contains(t, body, "chybí token", "Poslední načtení z GitLabu selhalo")

	old := issues.Dial
	issues.Dial = func(host, token string) issues.Source { return fakeSource{} }
	t.Cleanup(func() { issues.Dial = old })
	t.Setenv(gitlab.TokenEnv, "secret")
	body = f.form("/issues/sync", url.Values{})
	contains(t, body, "1 nové issue", "Z GitLabu")
	lacks(t, body, "secret")

	// The automatic sync waits for sync_minutes after the last one.
	f.srv.syncIfDue(context.Background())
	fl, _ := f.srv.iss.store.Load()
	if len(fl.Issues) != 1 {
		t.Errorf("issues %d", len(fl.Issues))
	}
}
