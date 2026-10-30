// Command housenka reports work into Bílý motýl, in the terminal (TUI) or
// in the browser (housenka web).
//
// Install with the script in install.sh, or: go install github.com/just-tomdo/housenka@latest
package main

import (
	_ "embed"
	"io"
	"os"
	"runtime/debug"

	"github.com/just-tomdo/housenka/internal/cli"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/opener"
	"github.com/just-tomdo/housenka/internal/tui"
	"github.com/just-tomdo/housenka/internal/web"
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
	web.ExampleConfig = exampleConfig
	cli.TUI = func(cfg *config.Config, _ string) error { return tui.Run(cfg) }
	cli.Web = runWeb
	cli.WebRunning = web.Running
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}

func runWeb(c cli.WebCommand, out io.Writer) error {
	o := web.Options{Config: c.Config, ConfigPath: c.ConfigPath, Port: c.Port}
	if c.Open {
		var cmd []string
		if c.Config != nil {
			cmd = c.Config.BM.OpenCommand
		}
		o.Open = opener.New(opener.Detect(), cmd).Open
	}
	switch c.Action {
	case "start":
		return web.Start(o, out)
	case "stop":
		return web.Stop(c.Port, c.Force, out)
	case "status":
		return web.Status(c.Port, out)
	}
	return web.Serve(o, out)
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
