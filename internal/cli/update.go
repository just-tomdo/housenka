package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/just-tomdo/housenka/internal/update"
)

// newUpdater and executable are replaced in tests.
var (
	newUpdater = func() update.Updater { return update.New(Version) }
	executable = func() (string, error) {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(exe)
	}
)

const updateUsage = "použití: housenka update [--check] [--force]"

func runUpdate(stdout, stderr io.Writer, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, updateUsage) }
	check := fs.Bool("check", false, "jen zjistit, jestli je nová verze")
	force := fs.Bool("force", false, "nahradit i binárku sestavenou ze zdrojových kódů")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errUsage
	}
	exe, err := executable()
	if err != nil {
		return fmt.Errorf("nevím, kde je spuštěná binárka: %w", err)
	}
	u := newUpdater()
	rel, err := u.Latest()
	if err != nil {
		return err
	}
	release := update.IsRelease(Version)
	switch {
	case release && !update.Newer(rel.Version, Version):
		fmt.Fprintf(stdout, "Máš nejnovější verzi %s (%s).\n", Version, exe)
		return nil
	case *check:
		fmt.Fprintf(stdout, "Je k dispozici %s, máš %s (%s).\nAktualizuješ ji: housenka update\n", rel.Version, Version, exe)
		return nil
	case !release && !*force:
		// A local build (go install, go build) is updated from the sources;
		// often it is another copy than the installed release, earlier in PATH.
		return fmt.Errorf("%s je sestavená ze zdrojových kódů (verze %s).\n"+
			"Aktualizuj ji přes go install, nebo ji nahraď vydanou verzí %s: housenka update --force", exe, Version, rel.Version)
	}
	fmt.Fprintf(stdout, "Stahuji %s %s…\n", u.Asset(), rel.Version)
	if err := u.Install(rel, exe); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Aktualizováno: %s → %s (%s)\n", Version, rel.Version, exe)
	if WebRunning != nil {
		if url, ok := WebRunning(); ok {
			fmt.Fprintf(stdout, "Webové rozhraní na %s běží ve staré verzi, restartuj ho: housenka web stop a housenka web start\n", url)
		}
	}
	return nil
}
