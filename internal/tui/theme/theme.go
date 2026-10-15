// Package theme defines color palettes and the styles derived from them.
package theme

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/just-tomdo/housenka/internal/config"
)

// Palette is a set of named colors. All built-in palettes are dark themes.
type Palette struct {
	Base    color.Color // app background, also text on colored blocks
	Surface color.Color // panels, inactive badges
	Overlay color.Color // borders, separators
	Text    color.Color
	Subtext color.Color
	Muted   color.Color
	Accent  color.Color            // primary accent (mode block, active tab, focus)
	Accent2 color.Color            // secondary accent
	Green   color.Color            // productive work, success
	Orange  color.Color            // unproductive work
	Red     color.Color            // errors
	Yellow  color.Color            // warnings
	Colors  map[string]color.Color // named project colors, see config.ColorNames
}

var palettes = map[string]Palette{
	"tokyonight": {
		Base: c("#1a1b26"), Surface: c("#24283b"), Overlay: c("#414868"),
		Text: c("#c0caf5"), Subtext: c("#a9b1d6"), Muted: c("#565f89"),
		Accent: c("#7aa2f7"), Accent2: c("#bb9af7"),
		Green: c("#9ece6a"), Orange: c("#ff9e64"), Red: c("#f7768e"), Yellow: c("#e0af68"),
		Colors: map[string]color.Color{
			"red": c("#f7768e"), "orange": c("#ff9e64"), "yellow": c("#e0af68"), "green": c("#9ece6a"), "teal": c("#1abc9c"),
			"cyan": c("#7dcfff"), "blue": c("#7aa2f7"), "purple": c("#bb9af7"), "pink": c("#ff9ed8"),
		},
	},
	"catppuccin": {
		Base: c("#1e1e2e"), Surface: c("#313244"), Overlay: c("#585b70"),
		Text: c("#cdd6f4"), Subtext: c("#a6adc8"), Muted: c("#6c7086"),
		Accent: c("#cba6f7"), Accent2: c("#89b4fa"),
		Green: c("#a6e3a1"), Orange: c("#fab387"), Red: c("#f38ba8"), Yellow: c("#f9e2af"),
		Colors: map[string]color.Color{
			"red": c("#f38ba8"), "orange": c("#fab387"), "yellow": c("#f9e2af"), "green": c("#a6e3a1"), "teal": c("#94e2d5"),
			"cyan": c("#89dceb"), "blue": c("#89b4fa"), "purple": c("#cba6f7"), "pink": c("#f5c2e7"),
		},
	},
	"gruvbox": {
		Base: c("#282828"), Surface: c("#3c3836"), Overlay: c("#665c54"),
		Text: c("#ebdbb2"), Subtext: c("#bdae93"), Muted: c("#928374"),
		Accent: c("#fabd2f"), Accent2: c("#83a598"),
		Green: c("#b8bb26"), Orange: c("#fe8019"), Red: c("#fb4934"), Yellow: c("#fabd2f"),
		Colors: map[string]color.Color{
			"red": c("#fb4934"), "orange": c("#fe8019"), "yellow": c("#fabd2f"), "green": c("#b8bb26"), "teal": c("#8ec07c"),
			"cyan": c("#7fbbb3"), "blue": c("#83a598"), "purple": c("#d3869b"), "pink": c("#f2a5c0"),
		},
	},
}

func c(hex string) color.Color { return lipgloss.Color(hex) }

// Get returns the palette with the given name, falling back to tokyonight.
func Get(name string) Palette {
	if p, ok := palettes[name]; ok {
		return p
	}
	return palettes["tokyonight"]
}

// ProjectColor returns the color of a project: spec is a color name from
// config.ColorNames, "#rrggbb", or empty for a stable color derived from
// the code, so the same project always looks the same.
func (p Palette) ProjectColor(code, spec string) color.Color {
	switch {
	case strings.HasPrefix(spec, "#"):
		return lipgloss.Color(spec)
	case p.Colors[spec] != nil:
		return p.Colors[spec]
	}
	h := fnv.New32a()
	h.Write([]byte(code))
	return p.Colors[config.ColorNames[h.Sum32()%uint32(len(config.ColorNames))]]
}

// TextOn returns Base or Text, whichever is readable on bg.
func (p Palette) TextOn(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA() // 0–65535
	if 0.2126*float64(r)+0.7152*float64(g)+0.0722*float64(b) > 0.45*65535 {
		return p.Base
	}
	return p.Text
}

// Icons are status symbols, either Nerd Font glyphs or plain Unicode.
type Icons struct {
	Sent, Pending, Invalid, Warning, Issue, NoIssue string
}

// GetIcons returns the icon set.
func GetIcons(nerdFont bool) Icons {
	if nerdFont {
		return Icons{Sent: "", Pending: "", Invalid: "", Warning: "", Issue: "", NoIssue: ""}
	}
	return Icons{Sent: "✓", Pending: "•", Invalid: "⚠", Warning: "!", Issue: "#", NoIssue: "–"}
}
