package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/store"
)

const testYAML = `
gitlab_hosts: [gitlab.example.com]
bm: { base_url: https://bm.example.com/editor, fields: { project: p, issue: i, hours: h, activity: a, date: d, description: t } }
web: { port: 8765, hostnames: [housenka.localhost] }
activities:
  DEV:     { name: Vývoj, productive: true }
  ANALYZA: { name: Analýza, productive: true }
  REZIE:   { name: Režie, productive: false }
projects:
  SHOPDEV: { name: Shop rozvoj, activities: [DEV, ANALYZA], default_activity: DEV, color: teal }
  SHOPSUP: { name: Shop podpora, activities: [REZIE], color: "#aa5500" }
  INTERNI: { name: Interní, issue: optional }
repos:
  clients/shop-web: [SHOPDEV, SHOPSUP]
`

const (
	issue = "https://gitlab.example.com/clients/shop-web/-/issues/12"
	host  = "127.0.0.1:8765"
	token = "0123456789abcdef0123456789abcdef"
)

var today = civil.Date{Year: 2026, Month: 10, Day: 19} // Monday

type fixture struct {
	t       *testing.T
	srv     *Server
	h       http.Handler
	store   *store.Store
	cfgPath string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(testYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "entries"))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(cfg, cfgPath, st, func() civil.Date { return today }, 8765)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, srv: srv, h: srv.Handler(), store: st, cfgPath: cfgPath}
}

func (f *fixture) save(e entry.Entry) entry.Entry {
	f.t.Helper()
	saved, err := f.store.Save(e)
	if err != nil {
		f.t.Fatal(err)
	}
	return saved
}

// do sends a request as the browser would: right host, origin and CSRF token.
func (f *fixture) do(method, target string, form url.Values) (int, http.Header, string) {
	f.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, target, body)
	r.Host = host
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method != http.MethodGet {
		r.Header.Set("Origin", "http://"+host)
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (f *fixture) get(target string) string {
	f.t.Helper()
	code, _, body := f.do(http.MethodGet, target, nil)
	if code != http.StatusOK {
		f.t.Fatalf("GET %s = %d: %s", target, code, body)
	}
	return body
}

func contains(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("response does not contain %q", w)
		}
	}
}

func lacks(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(body, w) {
			t.Errorf("response contains %q", w)
		}
	}
}

func sample(date civil.Date, desc string) entry.Entry {
	return entry.Entry{Date: date, IssueURL: issue, ProjectCode: "SHOPDEV", Activity: "DEV", Quarters: 11, Description: desc}
}

func TestPageDayWeekMonth(t *testing.T) {
	f := newFixture(t)
	f.save(sample(today, "Oprava košíku\ndruhý řádek"))
	f.save(sample(today.AddDays(2), "Středeční práce"))
	f.save(entry.Entry{Date: today, ProjectCode: "SHOPSUP", Activity: "REZIE", Quarters: 4, Description: "porada", IssueURL: issue})

	day := f.get("/")
	contains(t, day, `data-theme="auto"`, "Oprava košíku", "druhý řádek", "shop-web", "#12", "3,75", "SHOPDEV",
		"pc-teal", "--pc: #aa5500", `class="badge today">dnes`, "BM ↗", "https://bm.example.com/editor?", `name="v_kind" value="day"`)
	lacks(t, day, "Středeční práce")

	week := f.get("/?kind=week&date=2026-10-21")
	contains(t, week, "Středeční práce", "Oprava košíku", `class="day-head`, `class="weekdays"`, "tento týden")

	month := f.get("/?kind=month&date=2026-09-03")
	contains(t, month, "V tomto období nejsou žádné záznamy", "tento měsíc →")

	view := f.get("/view?kind=day&date=2026-10-21")
	contains(t, view, `<div id="main"`, "Středeční práce")
	lacks(t, view, "<html", "hx-swap-oob")
}

func TestFilter(t *testing.T) {
	f := newFixture(t)
	f.save(sample(today, "Oprava košíku"))
	f.save(sample(today, "Faktura"))
	r := httptest.NewRequest(http.MethodGet, "/view?kind=day&date=2026-10-19&q=KOŠ", nil)
	r.Host = host
	r.Header.Set("HX-Trigger", "filter")
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	contains(t, w.Body.String(), "Oprava košíku", "1 záznam")
	lacks(t, w.Body.String(), "Faktura")
	if got := w.Header().Get("HX-Replace-Url"); !strings.Contains(got, "q=KO") {
		t.Errorf("HX-Replace-Url = %q", got)
	}
}

func TestNewEntryForm(t *testing.T) {
	f := newFixture(t)
	body := f.get("/entries/new?date=2026-10-16")
	contains(t, body, "Nový záznam", `value="16. 10. 2026"`, `value="2026-10-16"`, `hx-post="/entries"`,
		"bez issue – jen projekty s issue: optional", `id="p-INTERNI"`)

	orig := f.save(sample(today.AddDays(-3), "Původní"))
	body = f.get("/entries/new?copy=" + orig.ID + "&date=2026-10-19")
	contains(t, body, "Kopie záznamu", `value="19. 10. 2026"`, "Původní", `value="2,75"`, `id="p-SHOPDEV" value="SHOPDEV" checked`)
}

func TestFormRefresh(t *testing.T) {
	f := newFixture(t)
	body := f.get("/form/refresh?" + url.Values{"issue": {issue}, "date": {"včera"}, "hours": {"1:45"}}.Encode())
	contains(t, body, `id="choices"`, `id="p-SHOPDEV"`, `id="p-SHOPSUP"`, "✓ clients / shop-web / #12",
		`id="hint-date" class="hint " hx-swap-oob="true">ne 18. 10. 2026`, "= 1 h 45 min", "nejdřív vyber projekt")

	body = f.get("/form/refresh?" + url.Values{"issue": {issue}, "project": {"SHOPDEV"}}.Encode())
	contains(t, body, `id="a-DEV" value="DEV" checked`, "Vývoj · produktivní")

	body = f.get("/form/refresh?" + url.Values{"issue": {"https://elsewhere.example.org/a/b/-/issues/1"}}.Encode())
	contains(t, body, "elsewhere.example.org není v configu")
}

func TestSaveEntry(t *testing.T) {
	f := newFixture(t)
	form := url.Values{"date": {"dnes"}, "issue": {issue}, "project": {"SHOPDEV"}, "activity": {"DEV"},
		"hours": {"2,5"}, "description": {"Nová práce  "}, "title": {"Nový záznam"},
		"v_kind": {"week"}, "v_date": {"2026-10-19"}}

	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Set("hours", "abc")
	bad.Set("description", "")
	code, _, body := f.do(http.MethodPost, "/entries", bad)
	if code != http.StatusOK {
		t.Fatalf("invalid save = %d", code)
	}
	contains(t, body, "<dialog", "Nový záznam", "✗ neplatný čas", `id="hint-description" class="hint err"`)
	if all, _ := f.store.All(); len(all) != 0 {
		t.Fatalf("invalid entry stored: %v", all)
	}

	code, hdr, body := f.do(http.MethodPost, "/entries", form)
	if code != http.StatusOK {
		t.Fatalf("save = %d: %s", code, body)
	}
	all, _ := f.store.All()
	if len(all) != 1 || all[0].Description != "Nová práce" || all[0].Quarters != 10 {
		t.Fatalf("stored = %+v", all)
	}
	contains(t, body, `<div id="main" class="main" hx-swap-oob="true">`, "Nová práce", "Uloženo ✓", "Otevřít v BM ↗", `name="v_kind" value="week"`)
	lacks(t, body, "<dialog")
	contains(t, hdr.Get("HX-Trigger"), all[0].ID)

	// Editing a sent entry marks it unsent again.
	e := all[0]
	e.Sent = true
	f.save(e)
	form.Set("hours", "3")
	_, _, body = f.do(http.MethodPost, "/entries/"+e.ID, form)
	contains(t, body, "záznam byl už odeslaný, uprav ho i v BM")
	all, _ = f.store.All()
	if all[0].Sent || all[0].Quarters != 12 || all[0].ID != e.ID {
		t.Errorf("after edit = %+v", all[0])
	}
}

func TestSaveMovesView(t *testing.T) {
	f := newFixture(t)
	form := url.Values{"date": {"1. 9. 2026"}, "issue": {issue}, "project": {"SHOPDEV"}, "activity": {"DEV"},
		"hours": {"1"}, "description": {"Zpětně"}, "v_kind": {"day"}, "v_date": {"2026-10-19"}}
	_, hdr, body := f.do(http.MethodPost, "/entries", form)
	contains(t, body, "Zpětně", `name="v_date" value="2026-09-01"`)
	contains(t, hdr.Get("HX-Push-Url"), "date=2026-09-01")
}

func TestToggleSentAndDelete(t *testing.T) {
	f := newFixture(t)
	e := f.save(sample(today, "Práce"))
	state := url.Values{"v_kind": {"day"}, "v_date": {"2026-10-19"}}

	_, _, body := f.do(http.MethodPost, "/entries/"+e.ID+"/sent", state)
	contains(t, body, "Označeno jako odeslané", "state-sent")
	all, _ := f.store.All()
	if !all[0].Sent {
		t.Error("not marked sent")
	}
	_, _, body = f.do(http.MethodPost, "/entries/"+e.ID+"/sent", state)
	contains(t, body, "Označeno jako neodeslané")

	_, _, body = f.do(http.MethodDelete, "/entries/"+e.ID, state)
	contains(t, body, "Záznam smazán", "Zatím nic vykázáno")
	if all, _ := f.store.All(); len(all) != 0 {
		t.Errorf("not deleted: %v", all)
	}

	_, hdr, body := f.do(http.MethodDelete, "/entries/"+e.ID, state)
	if hdr.Get("HX-Reswap") != "none" || !strings.Contains(body, `class="toast err"`) {
		t.Errorf("deleting a missing entry: %v %s", hdr, body)
	}
}

func TestIssueAndStatus(t *testing.T) {
	f := newFixture(t)
	f.save(sample(today, "Košík"))
	f.save(sample(today.AddDays(-30), "Starší práce"))
	f.save(entry.Entry{Date: today, ProjectCode: "INTERNI", Activity: "REZIE", Quarters: 2, Description: "Porada"})

	body := f.get("/issue?url=" + url.QueryEscape(issue+"#note_1"))
	contains(t, body, "Issue shop-web#12", "5,50 h", "2 výkazy", "2 dny", "Košík", "Starší práce", "neodesláno")
	lacks(t, body, "Porada")

	body = f.get("/status?date=2026-10-21")
	contains(t, body, "Týdenní souhrn", "SHOPDEV", "INTERNI", "Košík", "Porada", "Kopírovat Markdown", "/status?date=2026-10-12")
	lacks(t, body, "Starší práce")

	body = f.get("/help")
	contains(t, body, "Klávesové zkratky")
}

func TestSecurity(t *testing.T) {
	f := newFixture(t)
	e := f.save(sample(today, "Práce"))

	send := func(method, target, host string, mutate func(*http.Request)) int {
		r := httptest.NewRequest(method, target, nil)
		r.Host = host
		r.Header.Set("Origin", "http://"+host)
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
		if mutate != nil {
			mutate(r)
		}
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		return w.Code
	}
	if c := send("GET", "/", "evil.example.com:8765", nil); c != http.StatusForbidden {
		t.Errorf("foreign host = %d", c)
	}
	if c := send("GET", "/", "housenka.localhost:8765", nil); c != http.StatusOK {
		t.Errorf("configured host = %d", c)
	}
	if c := send("GET", "/", "localhost:9999", nil); c != http.StatusForbidden {
		t.Errorf("other port = %d", c)
	}
	sent := "/entries/" + e.ID + "/sent"
	if c := send("POST", sent, host, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }); c != http.StatusForbidden {
		t.Errorf("missing token = %d", c)
	}
	if c := send("POST", sent, host, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example.com") }); c != http.StatusForbidden {
		t.Errorf("foreign origin = %d", c)
	}
	if c := send("POST", sent, host, nil); c != http.StatusOK {
		t.Errorf("valid request = %d", c)
	}
	if c := send("POST", "/_/shutdown", host, func(r *http.Request) { r.Header.Set("X-Shutdown-Token", "wrong") }); c != http.StatusForbidden {
		t.Errorf("shutdown with a wrong token = %d", c)
	}

	// A page view issues the CSRF cookie.
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = host
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != csrfCookie || !strings.Contains(w.Body.String(), c[0].Value) {
		t.Errorf("csrf cookie = %v", c)
	}
}

func TestThemeCookie(t *testing.T) {
	f := newFixture(t)
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = host
	r.AddCookie(&http.Cookie{Name: themeCookie, Value: "gruvbox-light"})
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	contains(t, w.Body.String(), `data-theme="gruvbox-light"`, `<option value="gruvbox-light" selected>`)

	r.Header.Del("Cookie")
	r.AddCookie(&http.Cookie{Name: themeCookie, Value: "neon"})
	w = httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	contains(t, w.Body.String(), `data-theme="auto"`)

	css := f.get("/themes.css")
	contains(t, css, `[data-theme="tokyonight"]`, `[data-theme="auto"]`, "@media (prefers-color-scheme: dark)", "color-scheme: light")
}

func TestConfigReload(t *testing.T) {
	f := newFixture(t)
	f.save(entry.Entry{Date: today, ProjectCode: "NOVY", Activity: "DEV", Quarters: 4, Description: "x"})
	contains(t, f.get("/"), "state-invalid")

	later := time.Now().Add(2 * time.Second)
	changed := strings.Replace(testYAML, "  INTERNI:", "  NOVY: { name: Nový, issue: optional }\n  INTERNI:", 1)
	os.WriteFile(f.cfgPath, []byte(changed), 0o644)
	os.Chtimes(f.cfgPath, later, later)
	body := f.get("/")
	contains(t, body, "state-pending")
	lacks(t, body, "state-invalid")

	later = later.Add(2 * time.Second)
	os.WriteFile(f.cfgPath, []byte("gitlab_hosts: ["), 0o644)
	os.Chtimes(f.cfgPath, later, later)
	body = f.get("/")
	contains(t, body, "Config se po úpravě nepodařilo načíst", "state-pending")
}

func TestThemes(t *testing.T) {
	ts, err := loadThemes(assets, "static/themes")
	if err != nil {
		t.Fatal(err)
	}
	if len(ts.list) < 9 || ts.list[0].Scheme != "light" {
		t.Errorf("themes = %v", ts.ids())
	}
	bad := fstest.MapFS{
		"t/housenka-light.css": {Data: []byte("/* name: A; scheme: light */\n:root { --base: #fff; }")},
	}
	if _, err := loadThemes(bad, "t"); err == nil || !strings.Contains(err.Error(), "chybí proměnná --surface") {
		t.Errorf("incomplete theme: %v", err)
	}
}
