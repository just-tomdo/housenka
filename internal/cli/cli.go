// Package cli implements the housenka command line.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/just-tomdo/housenka"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/rules"
)

const usage = `Housenka – vykazování práce do Bílého motýla

Použití:
  housenka [--config SOUBOR] [příkaz]

Příkazy:
  (žádný)               spustí TUI
  config init [--force] vytvoří vzorový config (~/.housenka/config.yaml)
  config check          zkontroluje config a vypíše chyby a varování
  config path           vypíše, kde jsou config a data
  parse ODKAZ           rozebere odkaz na issue a vypíše povolené kódy a činnosti
  help                  tato nápověda

Volby:
  --config SOUBOR       jiný config než výchozí
  HOUSENKA_HOME=DIR     (proměnná prostředí) jiný adresář místo ~/.housenka
`

// TUI starts the interactive interface. It is set by the main package so
// that this package does not depend on the UI.
var TUI func(cfg *config.Config, cfgPath string) error

// errUsage signals a usage error that has already been reported.
var errUsage = errors.New("usage")

// Run executes the command line and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	err := run(args, stdout, stderr)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		return 2
	default:
		fmt.Fprintln(stderr, "chyba:", err)
		return 1
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("housenka", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	cfgPath := fs.String("config", "", "cesta ke configu")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errUsage
	}
	if *cfgPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return err
		}
		*cfgPath = p
	}

	rest := fs.Args()
	cmd := ""
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}
	switch cmd {
	case "":
		return runTUI(*cfgPath)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	case "parse":
		if len(rest) != 1 {
			fmt.Fprintln(stderr, "použití: housenka parse ODKAZ")
			return errUsage
		}
		return parse(stdout, *cfgPath, rest[0])
	case "config":
		sub := ""
		if len(rest) > 0 {
			sub, rest = rest[0], rest[1:]
		}
		switch sub {
		case "init":
			return configInit(stdout, stderr, *cfgPath, rest)
		case "check":
			return configCheck(stdout, *cfgPath)
		case "path":
			return configPath(stdout, *cfgPath)
		}
		fmt.Fprintln(stderr, "použití: housenka config init|check|path")
		return errUsage
	}
	fmt.Fprintf(stderr, "neznámý příkaz %q\n\n%s", cmd, usage)
	return errUsage
}

func runTUI(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if TUI == nil {
		return errors.New("TUI zatím není k dispozici")
	}
	return TUI(cfg, cfgPath)
}

func configInit(stdout, stderr io.Writer, cfgPath string, args []string) error {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "přepsat existující config")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if _, err := os.Stat(cfgPath); err == nil && !*force {
		return fmt.Errorf("config %s už existuje (přepsat: housenka config init --force)", cfgPath)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, housenka.ExampleConfig, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Vytvořen vzorový config %s\nUprav kódy projektů a činností podle Bílého motýla a ověř ho: housenka config check\n", cfgPath)
	return nil
}

func configCheck(stdout io.Writer, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Config %s je v pořádku: %s, %s, %s pro repozitáře.\n", cfgPath,
		plural(len(cfg.Activities), "činnost", "činnosti", "činností"),
		plural(len(cfg.Projects), "kód projektu", "kódy projektů", "kódů projektů"),
		plural(len(cfg.Repos), "pravidlo", "pravidla", "pravidel"))
	if w := cfg.Warnings(); len(w) > 0 {
		fmt.Fprintln(stdout, "\nVarování:")
		for _, line := range w {
			fmt.Fprintln(stdout, "  -", line)
		}
	}
	return nil
}

func configPath(stdout io.Writer, cfgPath string) error {
	data, err := config.DataDir()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "config: %s\ndata:   %s\n", cfgPath, data)
	return nil
}

func parse(stdout io.Writer, cfgPath, link string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	r := rules.New(cfg)
	codes, ref, err := r.AllowedProjects(link)
	if ref != nil {
		w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Skupina:\t%s\n", ref.Group)
		fmt.Fprintf(w, "Repozitář:\t%s\n", ref.Repo)
		fmt.Fprintf(w, "Typ:\t%s\n", ref.Kind)
		fmt.Fprintf(w, "Číslo:\t%d\n", ref.IID)
		w.Flush()
	}
	if err != nil {
		return err
	}
	if ref == nil {
		fmt.Fprintln(stdout, "Bez odkazu na issue lze vykázat na:")
	} else {
		fmt.Fprintln(stdout, "\nPovolené kódy projektů:")
	}
	for _, code := range codes {
		fmt.Fprintf(stdout, "  %s – %s\n", code, cfg.Projects[code].Name)
		def := r.DefaultActivity(code)
		var acts []string
		for _, a := range r.AllowedActivities(code) {
			label := a
			if a == def {
				label += " (výchozí)"
			}
			acts = append(acts, label)
		}
		fmt.Fprintf(stdout, "      činnosti: %s\n", strings.Join(acts, ", "))
	}
	return nil
}

// plural picks the Czech form for n: 1 → one, 2–4 → few, otherwise many.
func plural(n int, one, few, many string) string {
	form := many
	switch {
	case n == 1:
		form = one
	case n >= 2 && n <= 4:
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}
