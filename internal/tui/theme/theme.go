// Package theme defines color palettes and the styles derived from them.
package theme

import (
	"hash/fnv"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette is a set of named colors. All built-in palettes are dark themes.
type Palette struct {
	Base    color.Color // app background, also text on colored blocks
	Surface color.Color // panels, inactive badges
	Overlay color.Color // borders, separators
	Text    color.Color
	Subtext color.Color
	Muted   color.Color
	Accent  color.Color // primary accent (mode block, active tab, focus)
	Accent2 color.Color // secondary accent
	Green   color.Color // productive work, success
	Orange  color.Color // unproductive work
	Red     color.Color // errors
	Yellow  color.Color // warnings
	Badges  []color.Color
}

var palettes = map[string]Palette{
	"tokyonight": {
		Base: c("#1a1b26"), Surface: c("#24283b"), Overlay: c("#414868"),
		Text: c("#c0caf5"), Subtext: c("#a9b1d6"), Muted: c("#565f89"),
		Accent: c("#7aa2f7"), Accent2: c("#bb9af7"),
		Green: c("#9ece6a"), Orange: c("#ff9e64"), Red: c("#f7768e"), Yellow: c("#e0af68"),
		Badges: []color.Color{c("#7aa2f7"), c("#bb9af7"), c("#7dcfff"), c("#1abc9c"), c("#e0af68"), c("#f7768e"), c("#73daca"), c("#2ac3de")},
	},
	"catppuccin": {
		Base: c("#1e1e2e"), Surface: c("#313244"), Overlay: c("#585b70"),
		Text: c("#cdd6f4"), Subtext: c("#a6adc8"), Muted: c("#6c7086"),
		Accent: c("#cba6f7"), Accent2: c("#89b4fa"),
		Green: c("#a6e3a1"), Orange: c("#fab387"), Red: c("#f38ba8"), Yellow: c("#f9e2af"),
		Badges: []color.Color{c("#89b4fa"), c("#cba6f7"), c("#89dceb"), c("#94e2d5"), c("#f9e2af"), c("#f5c2e7"), c("#b4befe"), c("#74c7ec")},
	},
	"gruvbox": {
		Base: c("#282828"), Surface: c("#3c3836"), Overlay: c("#665c54"),
		Text: c("#ebdbb2"), Subtext: c("#bdae93"), Muted: c("#928374"),
		Accent: c("#fabd2f"), Accent2: c("#83a598"),
		Green: c("#b8bb26"), Orange: c("#fe8019"), Red: c("#fb4934"), Yellow: c("#fabd2f"),
		Badges: []color.Color{c("#83a598"), c("#d3869b"), c("#8ec07c"), c("#fabd2f"), c("#fe8019"), c("#b8bb26"), c("#d5c4a1"), c("#458588")},
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

// BadgeColor returns a stable color for a key (e.g. a project code), so the
// same project always looks the same.
func (p Palette) BadgeColor(key string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(key))
	return p.Badges[h.Sum32()%uint32(len(p.Badges))]
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
