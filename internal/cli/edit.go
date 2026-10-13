package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	editor, err := editorCommand(os.Getenv, exec.LookPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		if err := writeExample(cfgPath); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Config %s neexistoval, vytvořil jsem ho ze vzoru.\n", cfgPath)
	}
	in := bufio.NewReader(Stdin)
	for {
		before, err := os.ReadFile(cfgPath)
		if err != nil {
			return err
		}
		if err := runEditor(editor[0], append(editor[1:], cfgPath)...); err != nil {
			return fmt.Errorf("editor %s: %w", editor[0], err)
		}
		after, err := os.ReadFile(cfgPath)
		if err != nil {
			return err
		}
		_, loadErr := config.Load(cfgPath)
		switch {
		case loadErr == nil && bytes.Equal(before, after):
			fmt.Fprintln(stdout, "Config beze změny.")
			return nil
		case loadErr == nil:
			return configCheck(stdout, cfgPath)
		}
		fmt.Fprintf(stderr, "%v\n\nOtevřít config znovu? [A/n] ", loadErr)
		answer, err := in.ReadString('\n')
		if err != nil && answer == "" {
			fmt.Fprintln(stderr) // no input (e.g. not a terminal): do not loop
		} else {
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "", "a", "ano", "y", "yes":
				continue
			}
		}
		return errors.New("config obsahuje chyby, Housenka se s ním nespustí")
	}
}

// writeExample writes the example config to path.
func writeExample(path string) error {
	if len(ExampleConfig) == 0 {
		return errors.New("vzorový config není k dispozici")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, ExampleConfig, 0o644)
}
