package opener

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin []byte
}

// fakeSystem has the given tools installed and records executed commands.
func fakeSystem(goos string, wsl bool, env map[string]string, tools ...string) (System, *[]call) {
	var calls []call
	return System{
		GOOS:   goos,
		WSL:    wsl,
		Getenv: func(k string) string { return env[k] },
		LookPath: func(name string) (string, error) {
			if slices.Contains(tools, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(name string, args []string, stdin []byte) error {
			calls = append(calls, call{name, args, stdin})
			return nil
		},
	}, &calls
}

const url = "https://bm.example/editor?a=1&b=2"

func TestOpen(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		wsl   bool
		tools []string
		want  []string
	}{
		{"linux", "linux", false, []string{"xdg-open"}, []string{"xdg-open", url}},
		{"mac", "darwin", false, []string{"open"}, []string{"open", url}},
		{"windows", "windows", false, []string{"rundll32"}, []string{"rundll32", "url.dll,FileProtocolHandler", url}},
		{"wsl with wslview", "linux", true, []string{"wslview", "rundll32.exe", "xdg-open"}, []string{"wslview", url}},
		{"wsl without wslview", "linux", true, []string{"rundll32.exe", "xdg-open"}, []string{"rundll32.exe", "url.dll,FileProtocolHandler", url}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys, calls := fakeSystem(tt.goos, tt.wsl, nil, tt.tools...)
			if err := New(sys, nil).Open(url); err != nil {
				t.Fatal(err)
			}
			if len(*calls) != 1 {
				t.Fatalf("calls = %+v", *calls)
			}
			got := append([]string{(*calls)[0].name}, (*calls)[0].args...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ran %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenOverride(t *testing.T) {
	sys, calls := fakeSystem("linux", false, nil)
	if err := New(sys, []string{"firefox", "--new-tab"}).Open(url); err != nil {
		t.Fatal(err)
	}
	c := (*calls)[0]
	if c.name != "firefox" || !slices.Equal(c.args, []string{"--new-tab", url}) {
		t.Errorf("ran %+v", c)
	}
}

func TestOpenNothingFound(t *testing.T) {
	sys, _ := fakeSystem("linux", false, nil)
	if err := New(sys, nil).Open(url); err == nil || !strings.Contains(err.Error(), "open_command") {
		t.Errorf("err = %v", err)
	}
}

func TestCopy(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		wsl   bool
		env   map[string]string
		tools []string
		want  string
	}{
		{"x11", "linux", false, nil, []string{"xclip", "wl-copy"}, "xclip"},
		{"wayland", "linux", false, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"xclip", "wl-copy"}, "wl-copy"},
		{"xsel only", "linux", false, nil, []string{"xsel"}, "xsel"},
		{"mac", "darwin", false, nil, []string{"pbcopy"}, "pbcopy"},
		{"wsl", "linux", true, nil, []string{"clip.exe", "xclip"}, "clip.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys, calls := fakeSystem(tt.goos, tt.wsl, tt.env, tt.tools...)
			if err := New(sys, nil).Copy("ahoj"); err != nil {
				t.Fatal(err)
			}
			if (*calls)[0].name != tt.want {
				t.Errorf("used %s, want %s", (*calls)[0].name, tt.want)
			}
		})
	}
}

func TestCopyClipExeUsesUTF16(t *testing.T) {
	sys, calls := fakeSystem("linux", true, nil, "clip.exe")
	if err := New(sys, nil).Copy("ř"); err != nil {
		t.Fatal(err)
	}
	// BOM FF FE, then U+0159 little-endian.
	if want := []byte{0xFF, 0xFE, 0x59, 0x01}; !bytes.Equal((*calls)[0].stdin, want) {
		t.Errorf("stdin = % x, want % x", (*calls)[0].stdin, want)
	}
}

func TestCopyNoTool(t *testing.T) {
	sys, _ := fakeSystem("linux", false, nil)
	if err := New(sys, nil).Copy("x"); !errors.Is(err, ErrNoClipboard) {
		t.Errorf("err = %v", err)
	}
}
