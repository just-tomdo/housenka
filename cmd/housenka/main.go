// Command housenka is a TUI for reporting work into Bílý motýl.
package main

import (
	"os"

	"github.com/just-tomdo/housenka/internal/cli"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/tui"
)

func main() {
	cli.TUI = func(cfg *config.Config, _ string) error { return tui.Run(cfg) }
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
