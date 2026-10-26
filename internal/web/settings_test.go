package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/configedit"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/store"
	"github.com/just-tomdo/housenka/internal/text"
)

// post sends a settings change with the hash of the current config file.
func (f *fixture) post(target string, form url.Values) (string, http.Header) {
	f.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	if form.Get("hash") == "" {
		data, _ := os.ReadFile(f.cfgPath)
		form.Set("hash", configedit.Hash(data))
	}
	code, hdr, body := f.do(http.MethodPost, target, form)
	if code != http.StatusOK {
		f.t.Fatalf("POST %s = %d: %s", target, code, body)
	}
	return body, hdr
}

func (f *fixture) cfg() *config.Config {
	f.t.Helper()
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		f.t.Fatal(err)
	}
	return cfg
}

func TestSettingsTabs(t *testing.T) {
	f := newFixture(t)
	f.save(sample(today, "Práce"))

	r := httptest.NewRequest("GET", "/settings", nil)
	r.Host = host
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if loc := w.Header().Get("Location"); w.Code != http.StatusSeeOther || loc != "/settings/projects" {
		t.Errorf("/settings = %d %q", w.Code, loc)
	}

	contains(t, f.get("/settings/projects"), "Kódy projektů", "SHOPDEV", "Shop rozvoj", "pc-teal", "--pc: #aa5500",
		`class="act good default"`, "clients/shop-web", "povinné", "volitelné", `name="hash"`, "Nastavení – Housenka")
	contains(t, f.get("/settings/repos"), "clients/shop-web", "SHOPSUP", `<td class="num muted">1</td>`)
	contains(t, f.get("/settings/activities"), "ANALYZA", "neproduktivní", "<code>SHOPDEV</code>")
	contains(t, f.get("/settings/general"), "gitlab.example.com", "https://bm.example.com/editor", `name="f_project" value="p"`,
		`name="goal" value="8"`, "dnes: 19. 10. 2026", `<option value="auto" selected>`)
	contains(t, f.get("/settings/yaml"), "SHOPDEV: { name: Shop rozvoj")
	if code, _, _ := f.do("GET", "/settings/nope", nil); code != http.StatusNotFound {
		t.Errorf("unknown tab = %d", code)
	}
}

func TestRepoParse(t *testing.T) {
	f := newFixture(t)
	body := f.get("/settings/repos/parse?" + url.Values{"repo": {"https://gitlab.example.com/clients/shop-web/-/issues/3"}}.Encode())
	contains(t, body, "✓ doména gitlab.example.com", `value="clients/shop-web" checked`, "jen tento repozitář",
		`value="clients/*"`, "všechny repozitáře ve skupině clients", "už existuje", "už pokrývá pravidlo clients/shop-web → SHOPDEV, SHOPSUP")

	body = f.get("/settings/repos/parse?" + url.Values{"repo": {"git@gitlab.other.example.org:interni/backend/api.git"}}.Encode())
	contains(t, body, "gitlab.other.example.org</strong> není mezi GitLab doménami", `name="add_host" value="1" checked`,
		`value="interni/backend/api" checked`, `value="interni/backend/*"`, `value="interni/*"`)
	lacks(t, body, "už existuje")

	contains(t, f.get("/settings/repos/parse?repo=jen-skupina"), "je jen skupina")

	// Copied without https://: a configured host is recognized, dots in an
	// unknown first segment make it a group.
	body = f.get("/settings/repos/parse?" + url.Values{"repo": {"gitlab.example.com/clients/shop-web"}}.Encode())
	contains(t, body, "✓ doména gitlab.example.com", `value="clients/shop-web" checked`)
	body = f.get("/settings/repos/parse?" + url.Values{"repo": {"group.with.dots/sub/web"}}.Encode())
	contains(t, body, `value="group.with.dots/sub/web" checked`, "vlož celý odkaz i s https://")
	lacks(t, body, "není mezi GitLab doménami")
}

func TestSaveRepo(t *testing.T) {
	f := newFixture(t)
	form := url.Values{"repo": {"https://gitlab.other.example.org/interni/backend/api"}, "pattern": {"interni/*"},
		"add_host": {"1"}, "projects": {"INTERNI"}, "new_code": {"BACKEND"}, "new_name": {"Backend"}}
	body, _ := f.post("/settings/repos", form)
	contains(t, body, `id="settings-content" class="main settings" hx-swap-oob="true"`, "Repozitář interni/* uložen, doména gitlab.other.example.org přidána")
	lacks(t, body, "<dialog")
	cfg := f.cfg()
	if !slices.Equal(cfg.Repos["interni/*"], []string{"INTERNI", "BACKEND"}) || cfg.Projects["BACKEND"].Name != "Backend" ||
		!slices.Contains(cfg.GitLabHosts, "gitlab.other.example.org") {
		t.Errorf("config = %+v %+v %v", cfg.Repos, cfg.Projects["BACKEND"], cfg.GitLabHosts)
	}

	// The entry form offers the new project right away.
	refresh := f.get("/form/refresh?" + url.Values{"issue": {"https://gitlab.other.example.org/interni/backend/api/-/issues/1"}}.Encode())
	contains(t, refresh, `id="p-INTERNI"`, `id="p-BACKEND"`)

	// Errors keep the form open with the input.
	body, _ = f.post("/settings/repos", url.Values{"repo": {"clients/shop-web"}, "pattern": {"clients/shop-web"}, "projects": {"SHOPDEV"}})
	contains(t, body, "<dialog", "Neuloženo", "už existuje", `value="clients/shop-web"`)
	body, _ = f.post("/settings/repos", url.Values{"repo": {"clients/new"}})
	contains(t, body, "vyber aspoň jeden projekt")

	// Edit: rename the rule.
	body, _ = f.post("/settings/repos", url.Values{"old": {"interni/*"}, "repo": {"interni/backend/*"}, "pattern": {"interni/backend/*"}, "projects": {"BACKEND"}})
	contains(t, body, "Repozitář interni/backend/* uložen")
	if cfg := f.cfg(); !slices.Equal(cfg.Repos["interni/backend/*"], []string{"BACKEND"}) || cfg.Repos["interni/*"] != nil {
		t.Errorf("after rename: %v", cfg.Repos)
	}
	contains(t, f.get("/settings/repos/form?pattern="+url.QueryEscape("interni/backend/*")), `value="interni/backend/*"`, `value="BACKEND" checked`)

	body, _ = f.post("/settings/repos/delete", url.Values{"pattern": {"interni/backend/*"}})
	contains(t, body, "Pravidlo interni/backend/* smazáno")
	if _, ok := f.cfg().Repos["interni/backend/*"]; ok {
		t.Error("rule not deleted")
	}
}

func TestSaveProject(t *testing.T) {
	f := newFixture(t)
	f.save(sample(today, "Práce"))
	contains(t, f.get("/settings/projects/form?code=SHOPDEV"), "Upravit projekt SHOPDEV", `value="SHOPDEV"`, `value="teal" checked`,
		`value="DEV" checked`, `value="ANALYZA" checked`, `<option value="DEV" selected>`, `value="clients/shop-web" checked`,
		"zůstane 1 záznam se starým kódem SHOPDEV")

	form := url.Values{"old": {"SHOPDEV"}, "code": {"SHOP"}, "name": {"Shop"}, "color": {"custom"}, "custom": {"#112233"},
		"activities": {"DEV"}, "default_activity": {"DEV"}, "repos": {"clients/shop-web"}}
	body, _ := f.post("/settings/projects", form)
	contains(t, body, "Projekt SHOPDEV přejmenován na SHOP")
	cfg := f.cfg()
	if p := cfg.Projects["SHOP"]; p.Color != "#112233" || !slices.Equal(p.Activities, []string{"DEV"}) ||
		!slices.Equal(cfg.Repos["clients/shop-web"], []string{"SHOP", "SHOPSUP"}) {
		t.Errorf("SHOP = %+v, repos = %v", p, cfg.Repos)
	}

	// Validation: the default activity must be allowed; a duplicate code is refused.
	body, _ = f.post("/settings/projects", url.Values{"code": {"NEW"}, "name": {"N"}, "activities": {"DEV"}, "default_activity": {"REZIE"}})
	contains(t, body, "<dialog", "výchozí činnost REZIE není mezi povolenými")
	body, _ = f.post("/settings/projects", url.Values{"code": {"SHOPSUP"}, "name": {"N"}})
	contains(t, body, "projekt SHOPSUP už existuje")
	body, _ = f.post("/settings/projects", url.Values{"code": {"NEW"}, "name": {""}})
	contains(t, body, "projects.NEW.name: chybí název")

	body, _ = f.post("/settings/projects/delete", url.Values{"code": {"SHOP"}})
	contains(t, body, "Projekt SHOP smazán")
	if cfg := f.cfg(); cfg.Projects["SHOP"].Name != "" || !slices.Equal(cfg.Repos["clients/shop-web"], []string{"SHOPSUP"}) {
		t.Errorf("after delete: %v", cfg.Repos)
	}
}

func TestSaveActivity(t *testing.T) {
	f := newFixture(t)
	body, _ := f.post("/settings/activities", url.Values{"code": {"TEST"}, "name": {"Testování"}, "productive": {"true"}})
	contains(t, body, "Činnost TEST uložena")
	body, _ = f.post("/settings/activities", url.Values{"old": {"ANALYZA"}, "code": {"ANALYSIS"}, "name": {"Analýza"}, "productive": {"false"}})
	contains(t, body, "Činnost ANALYZA přejmenována na ANALYSIS")
	cfg := f.cfg()
	if cfg.Activities["ANALYSIS"].IsProductive() || !slices.Contains(cfg.Projects["SHOPDEV"].Activities, "ANALYSIS") {
		t.Errorf("after rename: %+v", cfg.Projects["SHOPDEV"])
	}
	_, hdr, body := f.do(http.MethodPost, "/settings/activities/delete", withHash(f, url.Values{"code": {"REZIE"}}))
	if hdr.Get("HX-Reswap") != "none" || !strings.Contains(body, "jediná povolená u projektu SHOPSUP") {
		t.Errorf("deleting the only activity: %s", body)
	}
	body, _ = f.post("/settings/activities/delete", url.Values{"code": {"TEST"}})
	contains(t, body, "Činnost TEST smazána")
}

func withHash(f *fixture, v url.Values) url.Values {
	data, _ := os.ReadFile(f.cfgPath)
	v.Set("hash", configedit.Hash(data))
	return v
}

func TestConflict(t *testing.T) {
	f := newFixture(t)
	body, _ := f.post("/settings/activities", url.Values{"code": {"TEST"}, "name": {"T"}, "productive": {"true"}, "hash": {"stale"}})
	contains(t, body, "config se mezitím změnil")
	if _, ok := f.cfg().Activities["TEST"]; ok {
		t.Error("stale change written")
	}
}

func TestSaveGeneral(t *testing.T) {
	f := newFixture(t)
	body, _ := f.post("/settings/general/time", url.Values{"goal": {"7,5"}, "warning": {"abc"}, "max": {"12"}})
	contains(t, body, "Neuloženo", "varování: neplatný čas", `name="warning" value="abc"`)
	body, _ = f.post("/settings/general/time", url.Values{"goal": {"7,5"}, "warning": {"8"}, "max": {"12"}})
	contains(t, body, "Časové limity uloženy")
	if f.cfg().Time.DailyGoal != 30 {
		t.Error("goal not saved")
	}

	f.post("/settings/general/gitlab", url.Values{"hosts": {"gitlab.example.com\ngitlab.other.example.org"}})
	f.post("/settings/general/bm", url.Values{"base_url": {"https://bm.example.com/new"}, "f_project": {"a"}, "f_issue": {"b"},
		"f_hours": {"c"}, "f_activity": {"d"}, "f_date": {"e"}, "f_description": {"g"}, "open_command": {"wslview"}})
	f.post("/settings/general/ui", url.Values{"theme": {"gruvbox"}, "date_format": {"dd.mm.yyyy"}, "caterpillar": {"on"}})
	body, _ = f.post("/settings/general/web", url.Values{"port": {"9000"}, "hostnames": {"housenka.local"}, "web_theme": {"gruvbox-light"}})
	contains(t, body, "platí po restartu")
	// Until the restart the page stays reachable under its old name.
	if !f.srv.allowedHost("housenka.localhost:8765") || f.srv.allowedHost("housenka.local:8765") {
		t.Error("host names changed before a restart")
	}
	cfg := f.cfg()
	if len(cfg.GitLabHosts) != 2 || cfg.BM.Fields.Description != "g" || cfg.BM.OpenCommand[0] != "wslview" ||
		cfg.UI.Theme != "gruvbox" || cfg.UI.Background || cfg.Web.Port != 9000 || cfg.Web.Theme != "gruvbox-light" {
		t.Errorf("cfg = %+v %+v %+v %+v", cfg.GitLabHosts, cfg.BM, cfg.UI, cfg.Web)
	}

	body = f.get("/settings/bm/from-link?" + url.Values{"link": {"https://bm.example.com/ed?x=1&pp=A&ii=B&hh=1&aa=D&dd=1.1.&tt=t"}}.Encode())
	contains(t, body, "načtené z odkazu", `value="https://bm.example.com/ed?x=1"`, `name="f_issue" value="ii"`, `name="f_description" value="tt"`)
	// A link that cannot be read keeps the field names typed so far.
	contains(t, f.get("/settings/bm/from-link?link=nesmysl&f_project=typed"), "vlož celý odkaz", `name="f_project" value="typed"`)
	contains(t, f.get("/settings/date-preview?date_format=yyyy-mm-dd"), "dnes: 2026-10-19")
}

// The browser command survives the BM form even with spaces in a path or
// an empty argument (`cmd.exe /c start ""`).
func TestBMOpenCommandRoundTrip(t *testing.T) {
	f := newFixture(t)
	cmd := []string{"/mnt/c/Program Files/Mozilla Firefox/firefox.exe", "--new-tab", ""}
	d, _ := configedit.Load(f.cfgPath)
	d.SetBM("https://bm.example.com/editor", f.cfg().BM.Fields, cmd)
	if err := d.Save(f.cfgPath); err != nil {
		t.Fatal(err)
	}
	body := f.get("/settings/general")
	shown := text.JoinArgs(cmd)
	contains(t, body, template.HTMLEscapeString(shown))

	fields := f.cfg().BM.Fields
	f.post("/settings/general/bm", url.Values{"base_url": {"https://bm.example.com/other"}, "open_command": {shown},
		"f_project": {fields.Project}, "f_issue": {fields.Issue}, "f_hours": {fields.Hours},
		"f_activity": {fields.Activity}, "f_date": {fields.Date}, "f_description": {fields.Description}})
	if got := f.cfg().BM.OpenCommand; !slices.Equal(got, cmd) {
		t.Errorf("open_command = %q", got)
	}
	body, _ = f.post("/settings/general/bm", url.Values{"base_url": {"https://bm.example.com/x"}, "open_command": {`"firefox`},
		"f_project": {fields.Project}, "f_issue": {fields.Issue}, "f_hours": {fields.Hours},
		"f_activity": {fields.Activity}, "f_date": {fields.Date}, "f_description": {fields.Description}})
	contains(t, body, "neukončené uvozovky")
}

func TestSettingsYAML(t *testing.T) {
	f := newFixture(t)
	body, _ := f.post("/settings/yaml", url.Values{"yaml": {"gitlab_hosts: []\n"}, "action": {"save"}})
	contains(t, body, "Config obsahuje chyby", "gitlab_hosts: chybí")
	data, _ := os.ReadFile(f.cfgPath)
	changed := strings.Replace(string(data), "Shop rozvoj", "Shop vývoj", 1)
	body, _ = f.post("/settings/yaml", url.Values{"yaml": {changed}, "action": {"check"}})
	contains(t, body, "Config je v pořádku")
	if f.cfg().Projects["SHOPDEV"].Name != "Shop rozvoj" {
		t.Error("check saved the file")
	}
	body, _ = f.post("/settings/yaml", url.Values{"yaml": {changed}, "action": {"save"}})
	contains(t, body, "Uloženo. Předchozí verze je v")
	if f.cfg().Projects["SHOPDEV"].Name != "Shop vývoj" {
		t.Error("not saved")
	}
	if _, err := os.Stat(f.cfgPath + ".bak"); err != nil {
		t.Error("no backup")
	}
}

func TestYAMLKeepsLeadingNewline(t *testing.T) {
	f := newFixture(t)
	os.WriteFile(f.cfgPath, []byte("\n"+testYAML), 0o644)
	contains(t, f.get("/settings/yaml"), "spellcheck=\"false\">\n\n\ngitlab_hosts")
}

// Check and failed saves keep the version the text is based on, so a later
// save still notices a change made elsewhere meanwhile.
func TestYAMLKeepsBaseVersion(t *testing.T) {
	f := newFixture(t)
	orig, _ := os.ReadFile(f.cfgPath)
	base := configedit.Hash(orig)
	mine := strings.Replace(string(orig), "Shop rozvoj", "Z webu", 1)

	// Someone edits the file in vim meanwhile.
	theirs := strings.Replace(string(orig), "Shop podpora", "Z vimu", 1)
	os.WriteFile(f.cfgPath, []byte(theirs), 0o644)

	body, _ := f.post("/settings/yaml", url.Values{"yaml": {mine}, "action": {"check"}, "hash": {base}})
	contains(t, body, "Config je v pořádku", `name="hash" value="`+base+`"`)
	body, _ = f.post("/settings/yaml", url.Values{"yaml": {mine}, "action": {"save"}, "hash": {base}})
	contains(t, body, "config se mezitím změnil", "Tvůj text zůstal", `name="hash" value="`+base+`"`)
	body, _ = f.post("/settings/yaml", url.Values{"yaml": {mine}, "action": {"save"}, "hash": {base}})
	contains(t, body, "config se mezitím změnil")
	if got, _ := os.ReadFile(f.cfgPath); string(got) != theirs {
		t.Error("the change made in vim was overwritten")
	}
}

func TestYAMLKeepsCRLF(t *testing.T) {
	f := newFixture(t)
	crlf := strings.ReplaceAll(testYAML, "\n", "\r\n")
	os.WriteFile(f.cfgPath, []byte(crlf), 0o644)
	f.post("/settings/yaml", url.Values{"yaml": {crlf}, "action": {"save"}})
	if got, _ := os.ReadFile(f.cfgPath); string(got) != crlf {
		t.Errorf("line endings changed: %q", got[:40])
	}
}

func TestBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("gitlab_hosts: [\n"), 0o644)
	st, _ := store.Open(filepath.Join(dir, "entries"))
	srv, err := New(nil, cfgPath, st, func() civil.Date { return today }, 8765)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, srv: srv, h: srv.Handler(), cfgPath: cfgPath}

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = host
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/settings/yaml" {
		t.Errorf("/ with a broken config = %d %q", w.Code, w.Header().Get("Location"))
	}
	body := f.get("/settings/yaml")
	contains(t, body, "Config obsahuje chyby", "chyba syntaxe YAML", "gitlab_hosts: [")
	lacks(t, body, "Projekty</a>")
	if code, _, _ := f.do("GET", "/", nil); code != http.StatusSeeOther {
		t.Errorf("redirect = %d", code)
	}

	f.post("/settings/yaml", url.Values{"yaml": {testYAML}, "action": {"save"}})
	contains(t, f.get("/"), "Zatím nic vykázáno")
}

func TestSetupGuide(t *testing.T) {
	defer func(b []byte) { ExampleConfig = b }(ExampleConfig)
	example, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ExampleConfig = example
	f := newFixture(t)
	os.WriteFile(f.cfgPath, example, 0o644) // a fresh home: the example config

	contains(t, f.get("/"), "Config ještě obsahuje vzorové hodnoty", "Dokončit nastavení", `title="Nastavení není dokončené"`)
	r := httptest.NewRequest("GET", "/settings", nil)
	r.Host = host
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if loc := w.Header().Get("Location"); loc != "/settings/setup" {
		t.Errorf("/settings → %q", loc)
	}
	contains(t, f.get("/settings/setup"), "Průvodce nastavením", "Doména GitLabu", "Smazat vzorové položky")

	// The examples cannot go before own activities and projects exist.
	_, hdr, body := f.do(http.MethodPost, "/settings/examples/delete", withHash(f, url.Values{}))
	if hdr.Get("HX-Reswap") != "none" || !strings.Contains(body, "nejdřív přidej vlastní") {
		t.Errorf("deleting all examples: %s", body)
	}
	f.post("/settings/activities", url.Values{"code": {"VYVOJ"}, "name": {"Vývoj"}, "productive": {"true"}})
	f.post("/settings/projects", url.Values{"code": {"MUJ"}, "name": {"Můj projekt"}})
	f.post("/settings/repos", url.Values{"repo": {"tym/web"}, "pattern": {"tym/web"}, "projects": {"MUJ"}})
	body, _ = f.post("/settings/examples/delete", url.Values{})
	contains(t, body, "Odstraněny vzorové položky: clients/acme-web, clients/shop-web")
	cfg := f.cfg()
	if len(cfg.Activities) != 1 || len(cfg.Projects) != 1 || len(cfg.Repos) != 1 {
		t.Errorf("left: %v %v %v", cfg.ActivityCodes(), cfg.ProjectCodes(), cfg.Repos)
	}
	f.save(entry.Entry{Date: today, ProjectCode: "MUJ", Activity: "VYVOJ", Quarters: 4, Description: "x"})
	contains(t, f.get("/settings/setup"), `class="step done"`)
}

func TestSettingsCSRF(t *testing.T) {
	f := newFixture(t)
	r := httptest.NewRequest("POST", "/settings/activities", strings.NewReader("code=X&name=X&productive=true"))
	r.Host = host
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("without token = %d", w.Code)
	}
}
