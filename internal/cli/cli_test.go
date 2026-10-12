package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/just-tomdo/housenka/internal/config"
)

// The example config lives in the repository root, where main embeds it.
func init() {
	b, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		panic(err)
	}
	ExampleConfig = b
}

func run_(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.HomeEnv, dir)
	return dir
}

func TestExampleConfigIsValid(t *testing.T) {
	cfg, err := config.Parse(ExampleConfig)
	if err != nil {
		t.Fatalf("config.example.yaml is invalid: %v", err)
	}
	if w := cfg.Warnings(); len(w) > 0 {
		t.Errorf("config.example.yaml has warnings: %v", w)
	}
}

func TestConfigInitAndCheck(t *testing.T) {
	home := setHome(t)

	code, out, errOut := run_(t, "config", "init")
	if code != 0 {
		t.Fatalf("init failed: %s", errOut)
	}
	cfgPath := filepath.Join(home, "config.yaml")
	if !strings.Contains(out, cfgPath) {
		t.Errorf("init output = %q", out)
	}

	if code, _, errOut := run_(t, "config", "init"); code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("second init: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := run_(t, "config", "init", "--force"); code != 0 {
		t.Errorf("init --force failed: %s", errOut)
	}

	code, out, errOut = run_(t, "config", "check")
	if code != 0 || !strings.Contains(out, "je v pořádku") {
		t.Errorf("check: code=%d out=%q err=%q", code, out, errOut)
	}

	if err := os.WriteFile(cfgPath, []byte("gitlab_hosts: [x]\nactivitis: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = run_(t, "config", "check")
	if code != 1 || !strings.Contains(errOut, "activitis") {
		t.Errorf("check invalid: code=%d err=%q", code, errOut)
	}
}

func TestConfigFlag(t *testing.T) {
	setHome(t)
	path := filepath.Join(t.TempDir(), "jinde.yaml")
	if code, _, errOut := run_(t, "--config", path, "config", "init"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config not written to --config path: %v", err)
	}
	code, out, _ := run_(t, "--config", path, "config", "path")
	if code != 0 || !strings.Contains(out, path) {
		t.Errorf("path: %q", out)
	}
}

func TestParse(t *testing.T) {
	setHome(t)
	run_(t, "config", "init")

	code, out, errOut := run_(t, "parse", "https://gitlab.example.com/clients/shop-web/-/work_items/393#note_1")
	if code != 0 {
		t.Fatalf("parse failed: %s", errOut)
	}
	for _, want := range []string{"shop-web", "393", "SHOPDEV – E-shop – rozvoj", "DEV (výchozí)", "SHOPSUP"} {
		if !strings.Contains(out, want) {
			t.Errorf("parse output missing %q:\n%s", want, out)
		}
	}

	code, out, errOut = run_(t, "parse", "https://gitlab.example.com/clients/neznamy/-/issues/1")
	if code != 1 || !strings.Contains(out, "neznamy") || !strings.Contains(errOut, "repos") {
		t.Errorf("unmapped: code=%d out=%q err=%q", code, out, errOut)
	}

	code, out, _ = run_(t, "parse", "")
	if code != 0 || !strings.Contains(out, "INTERNI") {
		t.Errorf("empty link: code=%d out=%q", code, out)
	}
}

func TestUsageErrors(t *testing.T) {
	setHome(t)
	for _, args := range [][]string{{"nesmysl"}, {"config"}, {"parse"}, {"--bogus"}} {
		if code, _, _ := run_(t, args...); code != 2 {
			t.Errorf("%v: code = %d, want 2", args, code)
		}
	}
	if code, out, _ := run_(t, "help"); code != 0 || !strings.Contains(out, "Použití") {
		t.Errorf("help: code=%d", code)
	}
}

func TestTUIWithoutConfig(t *testing.T) {
	setHome(t)
	code, _, errOut := run_(t)
	if code != 1 || !strings.Contains(errOut, "config init") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0 činností", 1: "1 činnost", 3: "3 činnosti", 5: "5 činností", 12: "12 činností"} {
		if got := plural(n, "činnost", "činnosti", "činností"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}
