package tui

import "charm.land/bubbles/v2/key"

// keyMap holds every binding of the list view. Vim sequences (gg, dd, yy, :q)
// are handled in handleListKey; their bindings here exist for the help only.
type keyMap struct {
	Down, Up, Top, Bottom, HalfDown, HalfUp key.Binding
	PrevPeriod, NextPeriod, Today           key.Binding
	Day, Week, Month, CycleKind             key.Binding
	Add, Edit, Copy, Yank, Paste, Delete    key.Binding
	OpenBM, CopyURL, ToggleSent, Status     key.Binding
	Filter, Help, Quit                      key.Binding
}

func newKeyMap() keyMap {
	b := func(keys []string, help, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(help, desc))
	}
	return keyMap{
		Down:       b([]string{"j", "down"}, "j/↓", "dolů"),
		Up:         b([]string{"k", "up"}, "k/↑", "nahoru"),
		Top:        b([]string{"home"}, "gg/home", "na začátek"),
		Bottom:     b([]string{"G", "end"}, "G/end", "na konec"),
		HalfDown:   b([]string{"ctrl+d", "pgdown"}, "ctrl+d/pgdn", "o stránku dolů"),
		HalfUp:     b([]string{"ctrl+u", "pgup"}, "ctrl+u/pgup", "o stránku nahoru"),
		PrevPeriod: b([]string{"h", "left"}, "h/←", "předchozí"),
		NextPeriod: b([]string{"l", "right"}, "l/→", "další"),
		Today:      b([]string{"t"}, "t", "dnes"),
		Day:        b([]string{"1"}, "1", "den"),
		Week:       b([]string{"2"}, "2", "týden"),
		Month:      b([]string{"3"}, "3", "měsíc"),
		CycleKind:  b([]string{"tab"}, "tab", "den/týden/měsíc"),
		Add:        b([]string{"a", "o", "insert"}, "a/ins", "přidat"),
		Edit:       b([]string{"e", "enter"}, "e/enter", "upravit"),
		Copy:       b([]string{"c"}, "c", "kopie na dnes"),
		Yank:       b([]string{"y"}, "yy", "zapamatovat"),
		Paste:      b([]string{"p"}, "p", "vložit kopii"),
		Delete:     b([]string{"delete"}, "dd/del", "smazat"),
		OpenBM:     b([]string{"b"}, "b", "otevřít v BM"),
		CopyURL:    b([]string{"Y"}, "Y", "kopírovat odkaz BM"),
		ToggleSent: b([]string{"s", "space"}, "s/space", "odesláno"),
		Status:     b([]string{"S"}, "S", "týdenní souhrn"),
		Filter:     b([]string{"/"}, "/", "filtr"),
		Help:       b([]string{"?", "f1"}, "?", "nápověda"),
		Quit:       b([]string{"q", "ctrl+c"}, "q/:q", "konec"),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.PrevPeriod, k.CycleKind, k.Add, k.Edit, k.OpenBM, k.ToggleSent, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp},
		{k.PrevPeriod, k.NextPeriod, k.Today, k.Day, k.Week, k.Month, k.CycleKind},
		{k.Add, k.Edit, k.Copy, k.Yank, k.Paste, k.Delete},
		{k.OpenBM, k.CopyURL, k.ToggleSent, k.Status, k.Filter, k.Help, k.Quit},
	}
}

// formKeyMap holds bindings of the entry form.
type formKeyMap struct {
	Next, Prev, Save, Cancel, Option, Newline, Leave, Press, Switch key.Binding

	inDesc   bool // focus is in the multi-line description
	inButton bool // focus is on the Save / Cancel buttons
}

func newFormKeyMap() formKeyMap {
	return formKeyMap{
		Next:    key.NewBinding(key.WithKeys("tab", "enter"), key.WithHelp("tab/enter", "další pole")),
		Prev:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "předchozí pole")),
		Save:    key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "uložit")),
		Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "zrušit")),
		Option:  key.NewBinding(key.WithKeys("j", "k", "up", "down", "left", "right"), key.WithHelp("j/k ←/→", "výběr")),
		Newline: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "nový řádek")),
		Leave:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "další pole")),
		Press:   key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "stisknout")),
		Switch:  key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←/→", "jiné tlačítko")),
	}
}

func (k formKeyMap) ShortHelp() []key.Binding {
	switch {
	case k.inButton:
		return []key.Binding{k.Press, k.Switch, k.Prev, k.Cancel}
	case k.inDesc:
		return []key.Binding{k.Newline, k.Leave, k.Prev, k.Save, k.Cancel}
	}
	return []key.Binding{k.Next, k.Prev, k.Option, k.Save, k.Cancel}
}

func (k formKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
