package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/config"
)

// Stdin is read when `config edit` asks whether to reopen a broken config.
var Stdin io.Reader = os.Stdin

// runEditor runs the editor attached to the terminal; replaced in tests.
var runEditor = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// fallbackEditors are tried in order when neither $VISUAL nor $EDITOR is set.
var fallbackEditors = []string{"nano", "nvim", "vim", "vi"}

// editorCommand returns the editor and its arguments: $VISUAL, $EDITOR
// (both may contain arguments, e.g. "code --wait"), or the first installed
// fallback editor.
func editorCommand(getenv func(string) string, lookPath func(string) (string, error)) ([]string, error) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(getenv(env)); len(f) > 0 {
			return f, nil
		}
	}
	candidates := fallbackEditors
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "notepad")
	}
	for _, name := range candidates {
		if _, err := lookPath(name); err == nil {
			return []string{name}, nil
		}
	}
	return nil, errors.New("nenašel jsem žádný editor – nastav proměnnou EDITOR (např. export EDITOR=nvim)")
}

// configEdit opens the config in the editor and checks it afterwards.
// A config with errors can be reopened right away.
func configEdit(stdout, stderr io.Writer, cfgPath string) error {
	return editFile(stdout, stderr, editTarget{
		path:    cfgPath,
		name:    "Config",
		create:  writeExample,
		check:   func(p string) error { _, err := config.Load(p); return err },
		report:  func() error { return configCheck(stdout, cfgPath) },
		refused: "config obsahuje chyby, Housenka se s ním nespustí",
	})
}

// editTarget is a configuration file `… edit` opens.
type editTarget struct {
	path    string
	name    string                  // "Config", shown in messages
	create  func(path string) error // writes the example when the file is missing
	check   func(path string) error // loads and validates the file
	report  func() error            // prints the result of a valid, changed file
	refused string                  // the error when the user gives up on a broken file
}

// editFile opens the file in the editor and checks it afterwards; a file
// with errors can be reopened right away.
func editFile(stdout, stderr io.Writer, t editTarget) error {
	editor, err := editorCommand(os.Getenv, exec.LookPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(t.path); errors.Is(err, os.ErrNotExist) {
		if err := t.create(t.path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s neexistoval, vytvořil jsem ho ze vzoru.\n", t.name, t.path)
	}
	in := bufio.NewReader(Stdin)
	for {
		before, err := os.ReadFile(t.path)
		if err != nil {
			return err
		}
		if err := runEditor(editor[0], append(editor[1:], t.path)...); err != nil {
			return fmt.Errorf("editor %s: %w", editor[0], err)
		}
		after, err := os.ReadFile(t.path)
		if err != nil {
			return err
		}
		loadErr := t.check(t.path)
		switch {
		case loadErr == nil && bytes.Equal(before, after):
			fmt.Fprintf(stdout, "%s beze změny.\n", t.name)
			return nil
		case loadErr == nil:
			return t.report()
		}
		fmt.Fprintf(stderr, "%v\n\nOtevřít %s znovu? [A/n] ", loadErr, strings.ToLower(t.name))
		answer, err := in.ReadString('\n')
		if err != nil && answer == "" {
			fmt.Fprintln(stderr) // no input (e.g. not a terminal): do not loop
		} else {
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "", "a", "ano", "y", "yes":
				continue
			}
		}
		return errors.New(t.refused)
	}
}

// writeExample writes the example config to path.
func writeExample(path string) error {
	if len(ExampleConfig) == 0 {
		return errors.New("vzorový config není k dispozici")
	}
	return atomicfile.Write(path, ExampleConfig, 0o644)
}
