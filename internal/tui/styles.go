package tui

import (
	"image/color"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"

	"github.com/just-tomdo/housenka/internal/tui/theme"
)

// styles are all lipgloss styles of the UI, derived from one palette.
type styles struct {
	p theme.Palette

	Title, TabActive, TabInactive, Period lipgloss.Style
	Panel, PanelFocused, Modal            lipgloss.Style
	Text, Subtext, Muted, Bold            lipgloss.Style
	Good, Bad, Warn, Err, Accent          lipgloss.Style
	DayHeader, Selected, SelectedBar      lipgloss.Style
	ModeNormal, ModeInsert, ModeOther     lipgloss.Style
	StatusBar, StatusSeg                  lipgloss.Style
	Label, LabelFocused, FieldHint        lipgloss.Style
	OptionActive, Option, OptionFocused   lipgloss.Style
	Toast, ToastErr                       lipgloss.Style
	Button, ButtonSave, ButtonCancel      lipgloss.Style
	Help                                  help.Styles
}

func newStyles(p theme.Palette) styles {
	s := lipgloss.NewStyle
	modeBlock := func(bg color.Color) lipgloss.Style {
		return s().Bold(true).Foreground(p.Base).Background(bg).Padding(0, 1)
	}
	return styles{
		p:           p,
		Title:       s().Bold(true).Foreground(p.Accent),
		TabActive:   s().Bold(true).Foreground(p.Base).Background(p.Accent).Padding(0, 1),
		TabInactive: s().Foreground(p.Muted).Padding(0, 1),
		Period:      s().Bold(true).Foreground(p.Text),

		Panel:        s().Border(lipgloss.NormalBorder()).BorderForeground(p.Overlay).Padding(0, 1),
		PanelFocused: s().Border(lipgloss.NormalBorder()).BorderForeground(p.Accent).Padding(0, 1),
		Modal:        s().Border(lipgloss.NormalBorder()).BorderForeground(p.Accent2).Padding(1, 2),

		Text:    s().Foreground(p.Text),
		Subtext: s().Foreground(p.Subtext),
		Muted:   s().Foreground(p.Muted),
		Bold:    s().Bold(true).Foreground(p.Text),
		Good:    s().Foreground(p.Green),
		Bad:     s().Foreground(p.Orange),
		Warn:    s().Foreground(p.Yellow),
		Err:     s().Foreground(p.Red),
		Accent:  s().Foreground(p.Accent),

		DayHeader:   s().Bold(true).Foreground(p.Accent2),
		Selected:    s().Background(p.Surface),
		SelectedBar: s().Foreground(p.Accent).Background(p.Surface),

		ModeNormal: modeBlock(p.Accent),
		ModeInsert: modeBlock(p.Green),
		ModeOther:  modeBlock(p.Accent2),
		StatusBar:  s().Foreground(p.Subtext).Background(p.Surface),
		StatusSeg:  s().Foreground(p.Text).Background(p.Overlay).Padding(0, 1),

		Label:         s().Foreground(p.Subtext).Width(10),
		LabelFocused:  s().Bold(true).Foreground(p.Accent).Width(10),
		FieldHint:     s().Foreground(p.Muted),
		OptionActive:  s().Bold(true).Foreground(p.Base).Background(p.Accent2).Padding(0, 1),
		OptionFocused: s().Bold(true).Foreground(p.Base).Background(p.Accent).Padding(0, 1),
		Option:        s().Foreground(p.Subtext).Padding(0, 1),

		Button:       s().Foreground(p.Text).Background(p.Overlay).Padding(0, 2),
		ButtonSave:   s().Bold(true).Foreground(p.Base).Background(p.Green).Padding(0, 2),
		ButtonCancel: s().Bold(true).Foreground(p.Base).Background(p.Red).Padding(0, 2),

		Toast:    s().Bold(true).Foreground(p.Base).Background(p.Green).Padding(0, 1),
		ToastErr: s().Bold(true).Foreground(p.Base).Background(p.Red).Padding(0, 1),

		Help: help.Styles{
			Ellipsis:       s().Foreground(p.Muted),
			ShortKey:       s().Foreground(p.Accent),
			ShortDesc:      s().Foreground(p.Subtext),
			ShortSeparator: s().Foreground(p.Overlay),
			FullKey:        s().Foreground(p.Accent),
			FullDesc:       s().Foreground(p.Subtext),
			FullSeparator:  s().Foreground(p.Overlay),
		},
	}
}

// badge renders a colored label, e.g. a project code.
func (st styles) badge(text string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(st.p.Base).Background(st.p.BadgeColor(text)).Padding(0, 1).Render(text)
}
