package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
)

// A change on the issue page also updates the count in the top bar.
func TestIssuesNavBadgeUpdates(t *testing.T) {
	f := newFixture(t)
	f.enableIssues(testIssue("clients/shop-web", 12, "Košík"), testIssue("clients/shop-web", 13, "Filtr"))
	body := f.form("/issues/status", url.Values{"key": {"clients/shop-web#12"}, "status": {"todo"}})
	contains(t, body, `id="nav-issues"`, `hx-swap-oob="true" title="Issues přiřazená tobě (T)">Issues <span class="count new"`, `>1</span></a>`)
}

// Saving an entry from the issue page after the overview was turned off
// elsewhere saves it once and goes to the entries.
func TestReportAfterIssuesTurnedOff(t *testing.T) {
	f := newFixture(t)
	f.enableIssues(testIssue("clients/shop-web", 12, "Košík"))
	os.Remove(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"))
	form := url.Values{"date": {"19. 10. 2026"}, "issue": {issue}, "project": {"SHOPDEV"}, "activity": {"DEV"},
		"hours": {"1"}, "description": {"oprava"}, "v_tab": {"board"}}
	code, _, body := f.do(http.MethodPost, "/entries", form)
	if code != http.StatusOK {
		t.Errorf("code %d", code)
	}
	// The form closes (nothing but the toast) and the BM link stays.
	contains(t, body, `id="toast" hx-swap-oob="true"`, "přehled issues je mezitím vypnutý", "Otevřít v BM")
	lacks(t, body, "<form", `id="issues"`)
	if all, _ := f.store.All(); len(all) != 1 {
		t.Errorf("%d entries saved", len(all))
	}
}
