// Package cli implements the housenka command line.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/rules"
	"github.com/just-tomdo/housenka/internal/text"
)

const usage = `Housenka – vykazování práce do Bílého motýla

Použití:
  housenka [--config SOUBOR] [příkaz]

Příkazy:
  (žádný)               spustí TUI
  web [--port N] [--no-open]
                        spustí webové rozhraní v prohlížeči (Ctrl+C ho ukončí)
  web start [--port N] [--no-open]
                        spustí webové rozhraní na pozadí
  web stop [--force]    vypne webové rozhraní běžící na pozadí
                        (--force ukončí i server, který neodpovídá)
  web status            vypíše, jestli a kde webové rozhraní běží
  config init [--force] vytvoří vzorový config (~/.housenka/config.yaml)
  config edit           otevře config v editoru ($VISUAL, $EDITOR, jinak nano/nvim/vim/vi)
                        a po uložení ho zkontroluje
  config check          zkontroluje config a vypíše chyby a varování
  config path           vypíše, kde jsou config a data
  parse ODKAZ           rozebere odkaz na issue a vypíše povolené kódy a činnosti
  version               vypíše verzi
  help                  tato nápověda

Volby:
  --config SOUBOR       jiný config než výchozí
  HOUSENKA_HOME=DIR     (proměnná prostředí) jiný adresář místo ~/.housenka
  HOUSENKA_WEB_PORT=N   (proměnná prostředí) port webového rozhraní místo web.port
`

// Version is the release version; it is set by the main package.
var Version = "dev"

// ExampleConfig is the commented example configuration written by
// `housenka config init`. It is embedded and set by the main package.
var ExampleConfig []byte

// TUI starts the interactive interface. It is set by the main package so
// that this package does not depend on the UI.
var TUI func(cfg *config.Config, cfgPath string) error

// WebCommand is one `housenka web` invocation. Config is nil for stop and
// status, and when the config file is invalid.
type WebCommand struct {
	Action     string // "run", "start", "stop", "status"
	Config     *config.Config
	ConfigPath string
	Port       int  // 0 = web.port from the config; stop and status get it resolved
	Open       bool // open the browser
	Force      bool // stop: end a server that does not answer
}

// Web runs the web interface. It is set by the main package, like TUI.
var Web func(c WebCommand, stdout io.Writer) error

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
	case "version":
		fmt.Fprintln(stdout, "housenka", Version)
		return nil
	case "parse":
		if len(rest) != 1 {
			fmt.Fprintln(stderr, "použití: housenka parse ODKAZ")
			return errUsage
		}
		return parse(stdout, *cfgPath, rest[0])
	case "web":
		return web(stdout, stderr, *cfgPath, rest)
	case "config":
		sub := ""
		if len(rest) > 0 {
			sub, rest = rest[0], rest[1:]
		}
		switch sub {
		case "init":
			return configInit(stdout, stderr, *cfgPath, rest)
		case "edit":
			return configEdit(stdout, stderr, *cfgPath)
		case "check":
			return configCheck(stdout, *cfgPath)
		case "path":
			return configPath(stdout, *cfgPath)
		}
		fmt.Fprintln(stderr, "použití: housenka config init|edit|check|path")
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

const webUsage = "použití: housenka web [start|stop|status] [--port N] [--no-open] [--force]"

func web(stdout, stderr io.Writer, cfgPath string, args []string) error {
	c := WebCommand{Action: "run", ConfigPath: cfgPath}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		c.Action, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, webUsage) }
	port := fs.Int("port", 0, "port (jinak "+webPortEnv+" nebo web.port z configu)")
	noOpen := fs.Bool("no-open", false, "neotevírat prohlížeč")
	force := fs.Bool("force", false, "ukončit i server, který neodpovídá (jen web stop)")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errUsage
	}
	c.Port = *port
	if c.Port == 0 {
		if v := os.Getenv(webPortEnv); v != "" {
			p, err := strconv.Atoi(v)
			if err != nil || p < 1 || p > 65535 {
				return fmt.Errorf("%s=%q není platný port", webPortEnv, v)
			}
			c.Port = p
		}
	}
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("--port %d není platný port", c.Port)
	}
	if *force && c.Action != "stop" {
		fmt.Fprintln(stderr, webUsage)
		return errUsage
	}
	switch c.Action {
	case "stop", "status":
		if *noOpen {
			fmt.Fprintln(stderr, webUsage)
			return errUsage
		}
		// Where to look for a server whose state file is missing.
		if c.Port == 0 {
			c.Port = config.DefaultWebPort
			if cfg, err := config.Load(cfgPath); err == nil {
				c.Port = cfg.Web.Port
			}
		}
		c.Force = *force
	case "run", "start":
		// The web interface helps with a missing or broken config: a
		// missing one is created from the example and finished in the
		// browser, a broken one is fixed in its text editor there.
		if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
			if err := writeExample(cfgPath); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Vytvořen vzorový config %s – dokonči nastavení v prohlížeči (Nastavení → Průvodce).\n", cfgPath)
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintf(stdout, "Config obsahuje chyby – oprav je v prohlížeči (Nastavení → YAML):\n%v\n", err)
		}
		c.Config = cfg
		c.Open = !*noOpen && (cfg == nil || cfg.Web.OpenBrowser)
	default:
		fmt.Fprintln(stderr, webUsage)
		return errUsage
	}
	if Web == nil {
		return errors.New("webové rozhraní zatím není k dispozici")
	}
	return Web(c, stdout)
}

// webPortEnv overrides web.port (the same name the web package documents).
const webPortEnv = "HOUSENKA_WEB_PORT"

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
	if err := writeExample(cfgPath); err != nil {
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
		text.Plural(len(cfg.Activities), "činnost", "činnosti", "činností"),
		text.Plural(len(cfg.Projects), "kód projektu", "kódy projektů", "kódů projektů"),
		text.Plural(len(cfg.Repos), "pravidlo", "pravidla", "pravidel"))
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
