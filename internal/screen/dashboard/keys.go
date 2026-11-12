package dashboard

import "github.com/charmbracelet/bubbles/key"

// keyMap is every binding on the dashboard. It satisfies bubbles' help.KeyMap:
// ShortHelp is the footer, FullHelp is the sheet behind "?".
type keyMap struct {
	Up, Down, PageUp, PageDown, Top, Bottom key.Binding
	Open                                    key.Binding // the selected container's dialog
	Next, Prev, Containers, Images, Pods    key.Binding
	Filter, Refresh, Help, Back, Quit       key.Binding
	move                                    key.Binding // footer label for Up+Down
}

func newKeys() keyMap {
	b := func(keys []string, k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(k, desc))
	}
	return keyMap{
		Up:         b([]string{"up", "k"}, "↑ k", "up"),
		Down:       b([]string{"down", "j"}, "↓ j", "down"),
		PageUp:     b([]string{"pgup", "ctrl+u"}, "pgup", "page up"),
		PageDown:   b([]string{"pgdown", "ctrl+d"}, "pgdn", "page down"),
		Top:        b([]string{"g", "home"}, "g", "first"),
		Bottom:     b([]string{"G", "end"}, "G", "last"),
		Open:       b([]string{"enter"}, "enter", "open"),
		Next:       b([]string{"tab", "right", "l"}, "tab", "section"),
		Prev:       b([]string{"shift+tab", "left", "h"}, "⇧tab", "back a section"),
		Containers: b([]string{"1"}, "1", "containers"),
		Images:     b([]string{"2"}, "2", "images"),
		Pods:       b([]string{"3"}, "3", "pods"),
		Filter:     b([]string{"/"}, "/", "filter"),
		Refresh:    b([]string{"r"}, "r", "refresh"),
		Help:       b([]string{"?"}, "?", "keys"),
		Back:       b([]string{"esc"}, "esc", "clear filter"),
		Quit:       b([]string{"q"}, "q", "quit"),
		move:       b([]string{"up", "down"}, "↑↓", "move"), // a label; Up and Down do the work
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.move, k.Open, k.Next, k.Filter, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.Open, k.Next, k.Prev, k.Containers, k.Images, k.Pods},
		{k.Filter, k.Back, k.Refresh, k.Help, k.Quit},
	}
}

// dialogKeys are the bindings while a container's dialog is open.
type dialogKeys struct {
	Start, Stop, Restart, Remove, Untag, GoTo key.Binding
	Up, Down, PageUp, PageDown, Top, Follow   key.Binding
	Close                                     key.Binding
	scroll                                    key.Binding // footer label for Up+Down
}

func newDialogKeys() dialogKeys {
	b := func(keys []string, k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(k, desc))
	}
	return dialogKeys{
		Start:    b([]string{"enter", "s"}, "enter", "start"),
		Stop:     b([]string{"x"}, "x", "stop"), // not enter: one enter too many shouldn't stop a container
		Restart:  b([]string{"r"}, "r", "restart"),
		Remove:   b([]string{"d", "delete"}, "d", "remove"),
		Untag:    b([]string{"u"}, "u", "untag"),
		GoTo:     b([]string{"c"}, "c", "go to container"),
		Up:       b([]string{"up", "k"}, "↑ k", "scroll up"),
		Down:     b([]string{"down", "j"}, "↓ j", "scroll down"),
		PageUp:   b([]string{"pgup", "ctrl+u"}, "pgup", "page up"),
		PageDown: b([]string{"pgdown", "ctrl+d"}, "pgdn", "page down"),
		Top:      b([]string{"g", "home"}, "g", "oldest"),
		Follow:   b([]string{"G", "end"}, "G", "follow"),
		Close:    b([]string{"esc", "q"}, "esc", "close"),
		scroll:   b([]string{"up", "down"}, "↑↓", "scroll"), // a label, like move
	}
}

func (k dialogKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.GoTo, k.Start, k.Stop, k.Restart, k.Remove, k.Untag, k.scroll, k.Follow, k.Close}
}

func (k dialogKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Start, k.Stop, k.Restart, k.Remove, k.Untag, k.GoTo, k.Close},
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Follow},
	}
}
