// Command housenka is a TUI for reporting work into Bílý motýl.
//
// Install with: go install github.com/just-tomdo/housenka@latest
package main

import (
	_ "embed"
	"os"

	"github.com/just-tomdo/housenka/internal/cli"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/tui"
)

// exampleConfig is the commented example configuration written by `housenka config init`.
//
//go:embed config.example.yaml
var exampleConfig []byte

func main() {
	cli.ExampleConfig = exampleConfig
	cli.TUI = func(cfg *config.Config, _ string) error { return tui.Run(cfg) }
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
