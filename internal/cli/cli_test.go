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

func TestEditorCommand(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	installed := map[string]bool{"vim": true, "vi": true}
	lookPath := func(n string) (string, error) {
		if installed[n] {
			return "/usr/bin/" + n, nil
		}
		return "", os.ErrNotExist
	}
	check := func(want string) {
		t.Helper()
		got, err := editorCommand(getenv, lookPath)
		if err != nil || strings.Join(got, " ") != want {
			t.Errorf("editorCommand = %v, %v; want %q", got, err, want)
		}
	}
	check("vim") // first installed fallback
	env["EDITOR"] = "nano"
	check("nano")
	env["VISUAL"] = "code --wait"
	check("code --wait")

	clear(env)
	clear(installed)
	if _, err := editorCommand(getenv, lookPath); err == nil || !strings.Contains(err.Error(), "EDITOR") {
		t.Errorf("no editor: %v", err)
	}
}

// fakeEditor replaces the editor with edits applied to the file, one per run.
func fakeEditor(t *testing.T, edits ...func(string) string) *[][]string {
	t.Helper()
	t.Setenv("VISUAL", "myeditor --wait")
	var calls [][]string
	old := runEditor
	runEditor = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		edit := edits[min(len(calls), len(edits))-1]
		path := args[len(args)-1]
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(edit(string(b))), 0o644)
	}
	t.Cleanup(func() { runEditor = old })
	return &calls
}

func setStdin(t *testing.T, s string) {
	t.Helper()
	old := Stdin
	Stdin = strings.NewReader(s)
	t.Cleanup(func() { Stdin = old })
}

func TestConfigEdit(t *testing.T) {
	home := setHome(t)
	cfgPath := filepath.Join(home, "config.yaml")
	keep := func(s string) string { return s }

	// A missing config is created from the example first.
	calls := fakeEditor(t, keep)
	code, out, errOut := run_(t, "config", "edit")
	if code != 0 || !strings.Contains(out, "vytvořil jsem ho ze vzoru") || !strings.Contains(out, "beze změny") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if got := strings.Join((*calls)[0], " "); got != "myeditor --wait "+cfgPath {
		t.Errorf("editor call = %q", got)
	}

	// A valid change is checked.
	fakeEditor(t, func(s string) string { return strings.Replace(s, "theme: tokyonight", "theme: gruvbox", 1) })
	if code, out, _ := run_(t, "config", "edit"); code != 0 || !strings.Contains(out, "je v pořádku") {
		t.Errorf("valid edit: code=%d out=%q", code, out)
	}

	// A broken config is reopened until fixed.
	broken := func(s string) string { return strings.Replace(s, "theme: gruvbox", "theme: neon", 1) }
	fixed := func(s string) string { return strings.Replace(s, "theme: neon", "theme: gruvbox", 1) }
	calls = fakeEditor(t, broken, fixed)
	setStdin(t, "\n")
	code, out, errOut = run_(t, "config", "edit")
	if code != 0 || len(*calls) != 2 || !strings.Contains(errOut, `"neon"`) || !strings.Contains(out, "je v pořádku") {
		t.Errorf("reopen: code=%d calls=%d out=%q err=%q", code, len(*calls), out, errOut)
	}

	// Declining to reopen (or no input at all) fails without looping.
	for _, answer := range []string{"n\n", ""} {
		calls = fakeEditor(t, broken)
		setStdin(t, answer)
		if code, _, errOut := run_(t, "config", "edit"); code != 1 || len(*calls) != 1 || !strings.Contains(errOut, "obsahuje chyby") {
			t.Errorf("answer %q: code=%d calls=%d err=%q", answer, code, len(*calls), errOut)
		}
		fakeEditor(t, fixed)
		run_(t, "config", "edit")
	}
}
