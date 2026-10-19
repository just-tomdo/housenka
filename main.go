// Command housenka is a TUI for reporting work into Bílý motýl.
//
// Install with the script in install.sh, or: go install github.com/just-tomdo/housenka@latest
package main

import (
	_ "embed"
	"os"
	"runtime/debug"

	"github.com/just-tomdo/housenka/internal/cli"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/tui"
)

// exampleConfig is the commented example configuration written by `housenka config init`.
//
//go:embed config.example.yaml
var exampleConfig []byte

// version is set by scripts/build-release.sh (-ldflags "-X main.version=v1.2.3").
var version = "dev"

func main() {
	cli.Version = buildVersion()
	cli.ExampleConfig = exampleConfig
	cli.TUI = func(cfg *config.Config, _ string) error { return tui.Run(cfg) }
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}

// buildVersion falls back to the module version for `go install …@v1.2.3`.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}
