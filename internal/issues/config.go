package issues

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/config"
)

// ExampleConfig is the commented template of issues.yaml written by
// `housenka issues init`, set by the main package.
var ExampleConfig []byte

// Config is the extension's own file, issues.yaml next to config.yaml.
// The extension is enabled when the file exists.
type Config struct {
	Host        string   `yaml:"host"`         // GitLab to read issues from; empty = the first of gitlab_hosts
	SyncMinutes int      `yaml:"sync_minutes"` // how often the web server reads GitLab
	StaleDays   int      `yaml:"stale_days"`   // an issue untouched this long is marked
	MoveTo      string   `yaml:"move_to"`      // where an issue moved to another status goes: end (default) or start
	Statuses    []Status `yaml:"statuses"`     // board columns, in this order
}

// Status is the user's own state of an issue: one column of the board.
type Status struct {
	Code  string `yaml:"code"`
	Name  string `yaml:"name"`
	Color string `yaml:"color"` // name from config.ColorNames or #rrggbb; empty = derived from the code
	Watch *bool  `yaml:"watch"` // false = issues in this status are never marked as forgotten
}

// Watched reports whether forgotten issues in this status are marked.
func (s Status) Watched() bool {
	return s.Watch == nil || *s.Watch
}

// Label returns how the status is shown: its name, or the code.
func (s Status) Label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Code
}

// DefaultConfig returns the values of keys left out of issues.yaml.
func DefaultConfig() Config {
	return Config{SyncMinutes: 15, StaleDays: 14}
}

// ConfigPath returns the path of issues.yaml in the housenka home.
func ConfigPath() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "issues.yaml"), nil
}

// DataDir returns the directory of issues.json in the housenka home.
func DataDir() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "issues"), nil
}

// LoadConfig reads issues.yaml. A missing file is reported as an error
// matching os.ErrNotExist: the extension is off.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Enabled reports whether err from LoadConfig means anything other than
// "the extension is off" (the file is missing).
func Enabled(err error) bool {
	return !errors.Is(err, os.ErrNotExist)
}

// ParseConfig decodes and validates issues.yaml.
func ParseConfig(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := config.DecodeYAML(data, &cfg); errors.Is(err, config.ErrEmpty) {
		return nil, errors.New("issues.yaml je prázdný – vzor vytvoří housenka issues init --force")
	} else if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var statusCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate checks the configuration and returns config.Problems or nil.
func (c *Config) Validate() error {
	var p config.Problems
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	if c.Host != "" && strings.ContainsAny(c.Host, "/: ") {
		add("host: zadej jen doménu, např. gitlab.example.com (ne %q)", c.Host)
	}
	if c.SyncMinutes < 1 {
		add("sync_minutes: musí být aspoň 1")
	}
	if c.StaleDays < 1 {
		add("stale_days: musí být aspoň 1")
	}
	switch c.MoveTo {
	case "", "end", "start":
	default:
		add("move_to: %q – zadej end (na konec sloupce) nebo start (na začátek)", c.MoveTo)
	}
	if len(c.Statuses) == 0 {
		add("statuses: zadej aspoň jeden stav (sloupec nástěnky)")
	}
	seen := map[string]bool{}
	for i, s := range c.Statuses {
		switch {
		case s.Code == "":
			add("statuses[%d]: chybí code", i+1)
		case !statusCodeRe.MatchString(s.Code):
			add("statuses[%d]: kód %q smí obsahovat jen malá písmena, číslice, - a _", i+1, s.Code)
		case seen[s.Code]:
			add("statuses: kód %q je tam dvakrát", s.Code)
		}
		seen[s.Code] = true
		if s.Color != "" && !config.ValidColor(s.Color) {
			add("statuses.%s.color: %s", s.Code, config.ColorProblem(s.Color))
		}
	}
	if len(p) > 0 {
		return p
	}
	return nil
}

// GitLabHost returns the GitLab host to read issues from.
func (c *Config) GitLabHost(main *config.Config) string {
	if c.Host != "" {
		return strings.ToLower(c.Host)
	}
	if main != nil && len(main.GitLabHosts) > 0 {
		return strings.ToLower(main.GitLabHosts[0])
	}
	return ""
}

// Warnings lists findings that do not stop the extension from working.
func (c *Config) Warnings(main *config.Config) []string {
	host := c.GitLabHost(main)
	switch {
	case host == "":
		return []string{"issues: chybí host a v configu nejsou gitlab_hosts – není odkud issue načíst"}
	case main != nil && !slices.ContainsFunc(main.GitLabHosts, func(h string) bool { return strings.EqualFold(h, host) }):
		return []string{fmt.Sprintf("issues: GitLab %s není v gitlab_hosts – issue se načtou, ale vykázat je nepůjde, dokud ho tam nepřidáš", host)}
	}
	return nil
}

// MoveToStart reports whether an issue moved to another status goes to
// the start of its column (move_to: start) rather than the end.
func (c *Config) MoveToStart() bool { return c.MoveTo == "start" }

// StatusIndex returns the column of a status code, or -1.
func (c *Config) StatusIndex(code string) int {
	if code == "" {
		return -1
	}
	return slices.IndexFunc(c.Statuses, func(s Status) bool { return s.Code == code })
}

// Status returns the status with the code.
func (c *Config) Status(code string) (Status, bool) {
	if i := c.StatusIndex(code); i >= 0 {
		return c.Statuses[i], true
	}
	return Status{}, false
}

// ConfigFile is issues.yaml, read again whenever it changed (the TUI, the
// web interface and a text editor may all change it).
type ConfigFile struct {
	Path string

	mu    sync.Mutex
	cfg   *Config
	err   error
	stamp atomicfile.Stamp
	read  bool
}

// Get returns the configuration, or nil and no error while the file is
// missing (the extension is off). An invalid file is an error.
func (c *ConfigFile) Get() (*Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Path == "" {
		return nil, nil
	}
	st, err := atomicfile.StatStamp(c.Path)
	if err != nil {
		return nil, err
	}
	if c.read && st == c.stamp {
		return c.cfg, c.err
	}
	c.read, c.stamp, c.cfg, c.err = true, st, nil, nil
	if st != (atomicfile.Stamp{}) {
		c.cfg, c.err = LoadConfig(c.Path)
		if !Enabled(c.err) {
			c.err = nil // removed meanwhile
		}
	}
	return c.cfg, c.err
}
