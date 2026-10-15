package theme

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/just-tomdo/housenka/internal/config"
)

func TestPalettesDefineEveryColor(t *testing.T) {
	for _, name := range config.Themes {
		p := Get(name)
		for _, c := range config.ColorNames {
			if p.Colors[c] == nil {
				t.Errorf("theme %s has no color %q", name, c)
			}
		}
	}
}

func TestProjectColor(t *testing.T) {
	p := Get("tokyonight")
	if got := p.ProjectColor("SHOPDEV", "red"); got != p.Colors["red"] {
		t.Errorf("named color = %v", got)
	}
	if got := p.ProjectColor("SHOPDEV", "#123456"); got != lipgloss.Color("#123456") {
		t.Errorf("hex color = %v", got)
	}
	// Without a spec the color is stable and comes from the named set.
	derived := p.ProjectColor("SHOPDEV", "")
	if derived != p.ProjectColor("SHOPDEV", "") {
		t.Error("derived color is not stable")
	}
	found := false
	for _, c := range p.Colors {
		found = found || c == derived
	}
	if !found {
		t.Errorf("derived color %v is not a named color", derived)
	}
}

func TestTextOn(t *testing.T) {
	p := Get("tokyonight")
	if p.TextOn(lipgloss.Color("#f9e2af")) != p.Base {
		t.Error("light background needs dark text")
	}
	if p.TextOn(lipgloss.Color("#202040")) != p.Text {
		t.Error("dark background needs light text")
	}
}
