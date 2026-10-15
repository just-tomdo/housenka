// Package config loads and validates the Housenka YAML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/hours"
)

// Themes lists the built-in UI themes.
var Themes = []string{"tokyonight", "catppuccin", "gruvbox", "ayu-dark"}

// ColorNames lists the named project colors; every theme defines its own
// shade of each.
var ColorNames = []string{"red", "orange", "yellow", "green", "teal", "cyan", "blue", "purple", "pink"}

// Config is the whole configuration file.
type Config struct {
	GitLabHosts []string            `yaml:"gitlab_hosts"`
	BM          BM                  `yaml:"bm"`
	UI          UI                  `yaml:"ui"`
	Time        Time                `yaml:"time"`
	Activities  map[string]Activity `yaml:"activities"`
	Projects    map[string]Project  `yaml:"projects"`
	Repos       map[string][]string `yaml:"repos"`
}

// BM configures links into Bílý motýl.
type BM struct {
	BaseURL     string   `yaml:"base_url"`
	Fields      BMFields `yaml:"fields"`
	OpenCommand []string `yaml:"open_command"` // empty = autodetect
}

// BMFields are the names of the editor form fields (URL query parameters)
// that get pre-filled. They are configured rather than built in because they
// are specific to the reporting system.
type BMFields struct {
	Project     string `yaml:"project"`
	Issue       string `yaml:"issue"`
	Hours       string `yaml:"hours"`
	Activity    string `yaml:"activity"`
	Date        string `yaml:"date"`
	Description string `yaml:"description"`
}

// Ordered returns (config key, field name) pairs in the order the editor expects.
func (f BMFields) Ordered() [][2]string {
	return [][2]string{
		{"project", f.Project}, {"issue", f.Issue}, {"hours", f.Hours},
		{"activity", f.Activity}, {"date", f.Date}, {"description", f.Description},
	}
}

// UI configures the look of the TUI.
type UI struct {
	Theme       string `yaml:"theme"`
	NerdFont    bool   `yaml:"nerd_font"`
	Caterpillar bool   `yaml:"caterpillar"`
	Background  bool   `yaml:"background"` // paint the theme background instead of the terminal's
	DateFormat  string `yaml:"date_format"`
}

// DateLayout returns the parsed ui.date_format, or the default when invalid
// (Validate reports that).
func (u UI) DateLayout() civil.Layout {
	if l, err := civil.ParseLayout(u.DateFormat); err == nil {
		return l
	}
	return civil.MustLayout(civil.DefaultLayout)
}

// Time holds time limits; all values are in quarter-hours.
type Time struct {
	DailyGoal    Hours `yaml:"daily_goal_hours"`
	MaxEntry     Hours `yaml:"max_entry_hours"`
	DailyWarning Hours `yaml:"daily_warning_hours"`
}

// Activity is a BM activity type ("činnost").
type Activity struct {
	Name       string `yaml:"name"`
	Productive *bool  `yaml:"productive"` // required; pointer detects omission
}

// IsProductive reports whether the activity counts as productive work.
func (a Activity) IsProductive() bool {
	return a.Productive != nil && *a.Productive
}

// IssuePolicy says whether a project requires an issue link.
type IssuePolicy string

const (
	IssueRequired IssuePolicy = "required"
	IssueOptional IssuePolicy = "optional"
)

// Project is a BM project code.
type Project struct {
	Name            string      `yaml:"name"`
	Activities      []string    `yaml:"activities"`       // empty = all activities
	DefaultActivity string      `yaml:"default_activity"` // optional
	Issue           IssuePolicy `yaml:"issue"`            // empty = required
	Color           string      `yaml:"color"`            // name from ColorNames or #rrggbb; empty = derived from the code
}

// IssueOptional reports whether entries for this project may omit the issue link.
func (p Project) IssueOptional() bool {
	return p.Issue == IssueOptional
}

// Hours is a duration in quarter-hours that decodes from YAML values like 8, 7.5 or "7:30".
type Hours int

// UnmarshalYAML implements yaml.Unmarshaler.
func (h *Hours) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("řádek %d: očekáván počet hodin", n.Line)
	}
	q, err := hours.Parse(n.Value)
	if err != nil {
		return fmt.Errorf("řádek %d: %v", n.Line, err)
	}
	*h = Hours(q)
	return nil
}

// Default returns a configuration with defaults for every optional value.
func Default() Config {
	return Config{
		UI: UI{Theme: "tokyonight", NerdFont: false, Caterpillar: true, Background: true, DateFormat: civil.DefaultLayout},
		Time: Time{
			DailyGoal:    8 * hours.PerHour,
			MaxEntry:     12 * hours.PerHour,
			DailyWarning: 8 * hours.PerHour,
		},
	}
}

// Load reads, decodes and validates the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config %s neexistuje – vytvoř ho příkazem `housenka config init`", path)
		}
		return nil, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes and validates configuration from YAML data.
// Unknown keys are rejected so that typos do not silently disable rules.
func Parse(data []byte) (*Config, error) {
	cfg := Default()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config je prázdný")
		}
		return nil, translateYAMLError(err)
	}
	if loc := unquotedHexRe.FindIndex(data); loc != nil {
		// YAML reads `color: #abc123` as an empty value with a comment.
		line := bytes.Count(data[:loc[0]], []byte("\n")) + 1
		return nil, fmt.Errorf("řádek %d: barvu zapiš v uvozovkách, např. color: \"#e06c75\" (bez nich ji YAML čte jako komentář)", line)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var unquotedHexRe = regexp.MustCompile(`(?m)^\s*color:\s*#`)

var unknownFieldRe = regexp.MustCompile(`^line (\d+): field (\S+) not found in type \S+$`)

// translateYAMLError turns decoder errors into Problems; unknown-key errors
// (usually typos) are reworded in Czech, others are kept as they are.
func translateYAMLError(err error) error {
	var te *yaml.TypeError
	if !errors.As(err, &te) {
		return fmt.Errorf("chyba syntaxe YAML: %w", err)
	}
	var p Problems
	for _, msg := range te.Errors {
		if m := unknownFieldRe.FindStringSubmatch(msg); m != nil {
			p = append(p, fmt.Sprintf("řádek %s: neznámý klíč „%s“ (překlep?)", m[1], m[2]))
		} else {
			p = append(p, msg)
		}
	}
	return p
}

// ActivityCodes returns all activity codes, sorted.
func (c *Config) ActivityCodes() []string {
	return sortedKeys(c.Activities)
}

// ProjectCodes returns all project codes, sorted.
func (c *Config) ProjectCodes() []string {
	return sortedKeys(c.Projects)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
