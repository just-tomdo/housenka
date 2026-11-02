package web

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
)

// cardAt returns where the card of key is on the page (-1 when missing).
func cardAt(page, key string) int {
	return strings.Index(page, `data-issue="`+key+`"`)
}

func TestIssueMoveToStartInWeb(t *testing.T) {
	f := newFixture(t)
	st := f.enableIssues(testIssue("clients/shop-web", 12, "Košík"), testIssue("clients/shop-web", 13, "Filtr"), testIssue("clients/shop-web", 14, "Kupóny"))
	st.SetStatus("clients/shop-web#12", "doing", "")
	st.SetStatus("clients/shop-web#14", "doing", "")

	// Through the settings: move_to start is written to issues.yaml.
	f.ipost("/settings/issues/general", url.Values{"host": {""}, "sync_minutes": {"15"}, "stale_days": {"14"}, "move_to": {"start"}})
	data, _ := os.ReadFile(filepath.Join(os.Getenv(config.HomeEnv), "issues.yaml"))
	if !strings.Contains(string(data), "move_to: start") {
		t.Fatalf("issues.yaml:\n%s", data)
	}

	f.form("/issues/status", url.Values{"key": {"clients/shop-web#13"}, "status": {"doing"}})
	board := f.get("/issues?tab=board")
	if i13, i12 := cardAt(board, "clients/shop-web#13"), cardAt(board, "clients/shop-web#12"); i13 > i12 {
		t.Error("with move_to: start the moved issue must be first")
	}
	// A drop keeps the place the user let go, whatever move_to says.
	f.form("/issues/status", url.Values{"key": {"clients/shop-web#13"}, "status": {"todo"}})
	f.form("/issues/status", url.Values{"key": {"clients/shop-web#13"}, "status": {"doing"}, "end": {"1"}})
	board = f.get("/issues?tab=board")
	if i13, i14 := cardAt(board, "clients/shop-web#13"), cardAt(board, "clients/shop-web#14"); i13 < i14 {
		t.Error("a drop at the end of the column must stay at the end")
	}
	contains(t, f.get("/settings/issues"), `<option value="start" selected>`)
}
