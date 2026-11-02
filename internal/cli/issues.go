package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/gitlab"
	"github.com/just-tomdo/housenka/internal/issues"
	"github.com/just-tomdo/housenka/internal/text"
)

const issuesUsage = "použití: housenka issues [init [--force]|edit|sync|token]"

// issuesCmd runs `housenka issues …`, the optional overview of GitLab issues.
func issuesCmd(stdout, stderr io.Writer, cfgPath string, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	path, err := issues.ConfigPath()
	if err != nil {
		return err
	}
	switch sub {
	case "":
		return issuesStatus(stdout, path)
	case "init":
		return issuesInit(stdout, stderr, path, args)
	case "edit":
		return editFile(stdout, stderr, editTarget{
			path:    path,
			name:    "Soubor",
			create:  writeIssuesExample,
			check:   func(p string) error { _, err := issues.LoadConfig(p); return err },
			report:  func() error { return issuesCheck(stdout, cfgPath, path) },
			refused: "issues.yaml obsahuje chyby, přehled issues bez něj nepoběží",
		})
	case "sync":
		if len(args) > 0 {
			break
		}
		return issuesSync(stdout, cfgPath, path)
	case "token":
		if len(args) > 0 {
			break
		}
		return issuesToken(stdout, stderr, cfgPath, path)
	}
	fmt.Fprintln(stderr, issuesUsage)
	return errUsage
}

func issuesInit(stdout, stderr io.Writer, path string, args []string) error {
	fs := flag.NewFlagSet("issues init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "přepsat existující issues.yaml")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s už existuje (přepsat: housenka issues init --force)", path)
	}
	if err := writeIssuesExample(path); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Vytvořen %s – přehled issues je zapnutý.\nDalší krok: housenka issues token (token GitLabu s oprávněním read_api), pak housenka issues sync.\n", path)
	return nil
}

func writeIssuesExample(path string) error {
	if len(issues.ExampleConfig) == 0 {
		return errors.New("vzorový issues.yaml není k dispozici")
	}
	return atomicfile.Write(path, issues.ExampleConfig, 0o644)
}

// loadIssues returns the extension's configuration and the main one (nil
// when broken: only gitlab_hosts is needed from it).
func loadIssues(cfgPath, path string) (*issues.Config, *config.Config, error) {
	cfg, err := issues.LoadConfig(path)
	if !issues.Enabled(err) {
		return nil, nil, errors.New("přehled issues je vypnutý – zapni ho příkazem `housenka issues init`")
	}
	if err != nil {
		return nil, nil, err
	}
	main, _ := config.Load(cfgPath)
	return cfg, main, nil
}

// issuesCheck reports a valid issues.yaml (also part of `config check`).
func issuesCheck(stdout io.Writer, cfgPath, path string) error {
	cfg, main, err := loadIssues(cfgPath, path)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Přehled issues %s je v pořádku: GitLab %s, %s.\n", path, cfg.GitLabHost(main),
		text.Plural(len(cfg.Statuses), "stav", "stavy", "stavů"))
	if w := cfg.Warnings(main); len(w) > 0 {
		fmt.Fprintln(stdout, "\nVarování:")
		for _, line := range w {
			fmt.Fprintln(stdout, "  -", line)
		}
	}
	return nil
}

func issuesStatus(stdout io.Writer, path string) error {
	cfg, err := issues.LoadConfig(path)
	if !issues.Enabled(err) {
		fmt.Fprintln(stdout, "Přehled issues je vypnutý. Zapneš ho příkazem `housenka issues init`.")
		return nil
	}
	if err != nil {
		return err
	}
	st, err := openIssueStore()
	if err != nil {
		return err
	}
	f, err := st.Load()
	if err != nil {
		return err
	}
	b := issues.Arrange(issues.Cards(f, cfg, nil, nil, civil.Today()), cfg)
	fmt.Fprintf(stdout, "Nová: %d\n", len(b.New))
	for _, col := range b.Columns {
		fmt.Fprintf(stdout, "%s: %d\n", col.Status.Label(), len(col.Cards))
	}
	fmt.Fprintf(stdout, "Skrytá: %d\n", len(b.Hidden))
	switch {
	case f.SyncError != "":
		fmt.Fprintf(stdout, "\nPoslední načtení selhalo: %s\n", f.SyncError)
	case f.SyncedAt.IsZero():
		fmt.Fprintln(stdout, "\nZ GitLabu se zatím nic nenačetlo: housenka issues sync")
	default:
		fmt.Fprintf(stdout, "\nNačteno z GitLabu %s.\n", f.SyncedAt.Local().Format("2. 1. 2006 15:04"))
	}
	return nil
}

func openIssueStore() (*issues.Store, error) {
	dir, err := issues.DataDir()
	if err != nil {
		return nil, err
	}
	return issues.OpenStore(dir)
}

func issuesSync(stdout io.Writer, cfgPath, path string) error {
	cfg, main, err := loadIssues(cfgPath, path)
	if err != nil {
		return err
	}
	src, err := issues.Connect(cfg, main)
	if err != nil {
		return err
	}
	st, err := openIssueStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), issues.SyncTimeout)
	defer cancel()
	res, err := issues.Sync(ctx, src, st)
	if err != nil {
		return err
	}
	f, err := st.Load()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Načteno z GitLabu: %s. Čeká na zařazení: %d.\n", res, len(f.New(cfg)))
	return nil
}

// issuesToken asks for the GitLab token, checks it and saves it.
func issuesToken(stdout, stderr io.Writer, cfgPath, path string) error {
	cfg, main, err := loadIssues(cfgPath, path)
	if err != nil {
		return err
	}
	host := cfg.GitLabHost(main)
	if gitlab.TokenFromEnv() {
		fmt.Fprintf(stderr, "Pozor: je nastavená proměnná %s, ta má přednost před uloženým tokenem.\n", gitlab.TokenEnv)
	}
	fmt.Fprintf(stdout, "Token GitLabu %s (Preferences → Access tokens, oprávnění read_api): ", host)
	token, err := readSecret()
	fmt.Fprintln(stdout)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token nebyl zadán")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	user, err := issues.Dial(host, token).CurrentUser(ctx)
	switch {
	case errors.Is(err, gitlab.ErrUnauthorized) || errors.Is(err, gitlab.ErrForbidden):
		return err
	case err != nil:
		fmt.Fprintf(stderr, "Token jsem nemohl ověřit (%v), ukládám ho i tak.\n", err)
	default:
		fmt.Fprintf(stdout, "Token patří uživateli %s (@%s).\n", user.Name, user.Username)
	}
	home, err := config.HomeDir()
	if err != nil {
		return err
	}
	if err := gitlab.SaveToken(home, token); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Uloženo do %s. Načti issue: housenka issues sync\n", gitlab.TokenPath(home))
	return nil
}

// readSecret reads a line from Stdin, without echo on a terminal.
func readSecret() (string, error) {
	if f, ok := Stdin.(*os.File); ok && term.IsTerminal(f.Fd()) {
		b, err := term.ReadPassword(f.Fd())
		return string(b), err
	}
	line, err := bufio.NewReader(Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("token nebyl zadán")
	}
	return line, nil
}
