// Package opener opens URLs in the browser and copies text to the clipboard
// using whatever the current OS (Linux, macOS, Windows, WSL) provides.
package opener

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

// ErrNoClipboard means no clipboard tool was found; callers may fall back
// to the terminal clipboard (OSC 52).
var ErrNoClipboard = errors.New("nenalezen nástroj pro schránku (wl-copy, xclip, xsel, pbcopy, clip.exe)")

// System describes the environment. Fields are replaceable in tests.
type System struct {
	GOOS     string
	WSL      bool
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Run      func(name string, args []string, stdin []byte) error
}

// Detect returns the System for the running process.
func Detect() System {
	return System{
		GOOS:     runtime.GOOS,
		WSL:      isWSL(),
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Run:      run,
	}
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

func run(name string, args []string, stdin []byte) error {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	// Output is discarded so that it cannot corrupt the TUI.
	return cmd.Run()
}

// Opener opens URLs and copies text.
type Opener struct {
	sys     System
	openCmd []string // user override from config; the URL is appended
}

// New returns an Opener. openCmd overrides browser detection when non-empty.
func New(sys System, openCmd []string) *Opener {
	return &Opener{sys: sys, openCmd: openCmd}
}

// Open opens url in the default browser.
func (o *Opener) Open(url string) error {
	if len(o.openCmd) > 0 {
		return o.sys.Run(o.openCmd[0], append(o.openCmd[1:], url), nil)
	}
	for _, c := range o.browserCandidates() {
		if _, err := o.sys.LookPath(c[0]); err == nil {
			return o.sys.Run(c[0], append(c[1:], url), nil)
		}
	}
	return errors.New("nenalezen příkaz pro otevření prohlížeče – nastav bm.open_command v configu")
}

// rundll32 avoids cmd.exe, which would interpret '&' in the URL.
func (o *Opener) browserCandidates() [][]string {
	switch {
	case o.sys.GOOS == "darwin":
		return [][]string{{"open"}}
	case o.sys.GOOS == "windows":
		return [][]string{{"rundll32", "url.dll,FileProtocolHandler"}}
	case o.sys.WSL:
		return [][]string{{"wslview"}, {"rundll32.exe", "url.dll,FileProtocolHandler"}, {"xdg-open"}}
	default:
		return [][]string{{"xdg-open"}}
	}
}

// Copy puts text on the system clipboard.
func (o *Opener) Copy(text string) error {
	for _, c := range o.clipboardCandidates() {
		if _, err := o.sys.LookPath(c.cmd[0]); err != nil {
			continue
		}
		input := []byte(text)
		if c.utf16 {
			input = utf16LE(text)
		}
		return o.sys.Run(c.cmd[0], c.cmd[1:], input)
	}
	return ErrNoClipboard
}

type clipTool struct {
	cmd   []string
	utf16 bool // clip.exe reads UTF-16LE with BOM correctly, UTF-8 would garble diacritics
}

func (o *Opener) clipboardCandidates() []clipTool {
	linux := []clipTool{
		{cmd: []string{"xclip", "-selection", "clipboard"}},
		{cmd: []string{"xsel", "--clipboard", "--input"}},
	}
	if o.sys.Getenv("WAYLAND_DISPLAY") != "" {
		linux = append([]clipTool{{cmd: []string{"wl-copy"}}}, linux...)
	}
	switch {
	case o.sys.GOOS == "darwin":
		return []clipTool{{cmd: []string{"pbcopy"}}}
	case o.sys.GOOS == "windows":
		return []clipTool{{cmd: []string{"clip"}, utf16: true}}
	case o.sys.WSL:
		return append([]clipTool{{cmd: []string{"clip.exe"}, utf16: true}}, linux...)
	default:
		return linux
	}
}

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(u))
	binary.LittleEndian.PutUint16(b, 0xFEFF)
	for _, r := range u {
		b = binary.LittleEndian.AppendUint16(b, r)
	}
	return b
}
