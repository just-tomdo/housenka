package configedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/issues"
)

func issuesExample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../issues.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func issuesDoc(t *testing.T, data []byte) *Doc {
	t.Helper()
	d, err := ParseIssues(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func renderIssues(t *testing.T, d *Doc) string {
	t.Helper()
	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIssuesUnchangedIsIdentical(t *testing.T) {
	handMade := "host: gitlab.example.com\r\n\r\nstatuses:\r\n  - {code: a, name: A}   # first\r\n  - {code: bb,   name: Bee, color: \"#112233\"}\r\n"
	for name, src := range map[string][]byte{"example": issuesExample(t), "hand-made": []byte(handMade)} {
		if got := renderIssues(t, issuesDoc(t, src)); got != string(src) {
			t.Errorf("%s changed:\n%s\nwant:\n%s", name, got, src)
		}
	}
}

func TestIssuesStatusOps(t *testing.T) {
	d := issuesDoc(t, issuesExample(t))
	off := false
	if err := d.SetStatus("", issues.Status{Code: "review", Name: "Review", Color: "teal", Watch: &off}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetStatus("todo", issues.Status{Code: "next", Name: "Na řadě", Color: "blue"}); err != nil {
		t.Fatal(err)
	}
	if err := d.MoveStatus("review", -10); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteStatus("waiting"); err != nil {
		t.Fatal(err)
	}
	d.SetIssueSettings("GitLab.Example.com", 30, 7)
	cfg, err := d.IssuesConfig()
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, s := range cfg.Statuses {
		codes = append(codes, s.Code)
	}
	if strings.Join(codes, ",") != "review,next,doing,done" || cfg.Statuses[0].Watched() || cfg.SyncMinutes != 30 || cfg.StaleDays != 7 || cfg.Host != "gitlab.example.com" {
		t.Errorf("config %+v", cfg)
	}
	out := renderIssues(t, d)
	for _, want := range []string{
		"# Housenka – přehled issues z GitLabu",
		"  - { code: review, name: Review,       color: teal,  watch: false }",
		"  - { code: next,   name: Na řadě,      color: blue }",
		"první z gitlab_hosts\nsync_minutes: 30          # jak často webové rozhraní GitLab načítá\nstale_days: 7 ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	if err := d.SetStatus("", issues.Status{Code: "next"}); err == nil {
		t.Error("duplicate code accepted")
	}
	if err := d.SetStatus("", issues.Status{Code: "x", Color: "brown"}); err == nil {
		t.Error("bad color accepted")
	}
	for _, c := range []string{"review", "next", "doing"} {
		if err := d.DeleteStatus(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.DeleteStatus("done"); err == nil {
		t.Error("deleted the last status")
	}
	d.SetIssueSettings("", 15, 14)
	if strings.Contains(renderIssues(t, d), "host:") {
		t.Error("empty host must be removed")
	}
}

func TestIssuesSaveValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.yaml")
	os.WriteFile(path, issuesExample(t), 0o644)
	d, err := LoadIssues(path)
	if err != nil {
		t.Fatal(err)
	}
	d.SetIssueSettings("", 0, 14)
	if err := d.Save(path); err == nil || !strings.Contains(err.Error(), "sync_minutes") {
		t.Errorf("invalid issues.yaml saved: %v", err)
	}
	d.SetIssueSettings("", 5, 14)
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("no backup")
	}
}
