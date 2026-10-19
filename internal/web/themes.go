package web

import (
	"cmp"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// A theme is one CSS file in static/themes/<id>.css: a header comment
// "/* name: Gruvbox světlý; scheme: light */" followed by a ":root { … }"
// block that sets the variables in themeVars. Adding a file adds a theme.

// themeVars are the CSS variables every theme must define.
var themeVars = []string{
	"base", "surface", "overlay", "text", "subtext", "muted", "accent", "accent2",
	"green", "orange", "red", "yellow",
	"c-red", "c-orange", "c-yellow", "c-green", "c-teal", "c-cyan", "c-blue", "c-purple", "c-pink",
}

// Auto follows the system: autoLight or autoDark by prefers-color-scheme.
const (
	themeAuto = "auto"
	autoLight = "housenka-light"
	autoDark  = "housenka-dark"
)

type theme struct {
	ID     string
	Name   string
	Scheme string // "light" or "dark"
	body   string // declarations inside :root { … }
}

type themeSet struct {
	list []theme // light themes first, then by name
	css  string  // every theme scoped to [data-theme="id"], plus auto
}

var (
	themeHeaderRe = regexp.MustCompile(`^/\*\s*name:\s*([^;]+?)\s*;\s*scheme:\s*(light|dark)\s*\*/`)
	themeBodyRe   = regexp.MustCompile(`(?s):root\s*\{(.*)\}\s*$`)
	themeVarRe    = regexp.MustCompile(`--([a-z0-9-]+)\s*:`)
)

// loadThemes reads and checks all themes in dir of fsys.
func loadThemes(fsys fs.FS, dir string) (*themeSet, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.css"))
	if err != nil {
		return nil, err
	}
	ts := &themeSet{}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		t, err := parseTheme(strings.TrimSuffix(path.Base(f), ".css"), string(data))
		if err != nil {
			return nil, err
		}
		ts.list = append(ts.list, t)
	}
	slices.SortFunc(ts.list, func(a, b theme) int {
		return cmp.Or(cmp.Compare(b.Scheme, a.Scheme), cmp.Compare(a.Name, b.Name))
	})
	light, okL := ts.get(autoLight)
	dark, okD := ts.get(autoDark)
	if !okL || !okD {
		return nil, fmt.Errorf("chybí výchozí témata %s a %s", autoLight, autoDark)
	}

	var b strings.Builder
	for _, t := range ts.list {
		fmt.Fprintf(&b, "[data-theme=%q] {\n  color-scheme: %s;%s}\n", t.ID, t.Scheme, t.body)
	}
	fmt.Fprintf(&b, "[data-theme=%q] {\n  color-scheme: light;%s}\n", themeAuto, light.body)
	fmt.Fprintf(&b, "@media (prefers-color-scheme: dark) {\n[data-theme=%q] {\n  color-scheme: dark;%s}\n}\n", themeAuto, dark.body)
	ts.css = b.String()
	return ts, nil
}

func parseTheme(id, css string) (theme, error) {
	css = strings.TrimSpace(css)
	head := themeHeaderRe.FindStringSubmatch(css)
	if head == nil {
		return theme{}, fmt.Errorf("téma %s: chybí hlavička /* name: …; scheme: light|dark */", id)
	}
	body := themeBodyRe.FindStringSubmatch(css)
	if body == nil {
		return theme{}, fmt.Errorf("téma %s: chybí blok :root { … }", id)
	}
	defined := map[string]bool{}
	for _, m := range themeVarRe.FindAllStringSubmatch(body[1], -1) {
		defined[m[1]] = true
	}
	for _, v := range themeVars {
		if !defined[v] {
			return theme{}, fmt.Errorf("téma %s: chybí proměnná --%s", id, v)
		}
	}
	return theme{ID: id, Name: head[1], Scheme: head[2], body: body[1]}, nil
}

func (ts *themeSet) get(id string) (theme, bool) {
	i := slices.IndexFunc(ts.list, func(t theme) bool { return t.ID == id })
	if i < 0 {
		return theme{}, false
	}
	return ts.list[i], true
}

// valid reports whether id names a theme or is "auto".
func (ts *themeSet) valid(id string) bool {
	_, ok := ts.get(id)
	return ok || id == themeAuto
}

// ids lists all choices, "auto" first.
func (ts *themeSet) ids() []string {
	out := []string{themeAuto}
	for _, t := range ts.list {
		out = append(out, t.ID)
	}
	return out
}
