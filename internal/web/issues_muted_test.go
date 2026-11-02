package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestIssueMutedInWeb(t *testing.T) {
	f := newFixture(t)
	st := f.enableIssues(testIssue("clients/shop-web", 12, "Košík"), testIssue("clients/shop-web", 13, "Filtr"))
	st.SetStatus("clients/shop-web#12", "todo", "")
	st.SetStatus("clients/shop-web#13", "todo", "")
	contains(t, f.get("/issues?tab=board"), `data-key="z"`, `hx-post="/issues/muted"`)

	body := f.form("/issues/muted", url.Values{"key": {"clients/shop-web#12"}, "muted": {"1"}})
	contains(t, body, "odloženo", `icard muted`, "odložené")
	board := f.get("/issues?tab=board")
	if strings.Index(board, "Košík") < strings.Index(board, "Filtr") {
		t.Error("the muted card must go after the other one")
	}
	contains(t, f.get("/issues/detail?key=clients%2Fshop-web%2312"), "Vrátit z odložených")
	f.form("/issues/muted", url.Values{"key": {"clients/shop-web#12"}, "muted": {"0"}})
	lacks(t, f.get("/issues?tab=board"), "icard muted")
}
