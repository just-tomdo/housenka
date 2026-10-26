package configedit

import (
	"os"
	"strings"
	"testing"
)

func readExample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRoundTripExample(t *testing.T) {
	data := readExample(t)
	d, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(data) {
		if p := os.Getenv("CONFIGEDIT_OUT"); p != "" {
			os.WriteFile(p, out, 0o644)
		}
		t.Error("round trip changed the example (CONFIGEDIT_OUT=file saves the output)")
	}
}

// Real-world shaped configs keep their exact layout when nothing changes.
func TestRoundTripExamples(t *testing.T) {
	for _, path := range []string{"../../examples/demo/config.yaml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		out, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != string(data) {
			if p := os.Getenv("CONFIGEDIT_OUT"); p != "" {
				os.WriteFile(p, out, 0o644)
			}
			t.Errorf("%s changed by a round trip", path)
		}
	}
}

func TestRoundTripKeepsHandLayout(t *testing.T) {
	src := strings.Join([]string{
		"gitlab_hosts: [gitlab.example.com]",
		"",
		"web: {}",
		"",
		"projects:",
		"  # clients",
		"  SHOPDEV: { name: Shop }",
		"  SHOPSUP: { name: Podpora }",
		"",
		"  # internal work",
		"  INTERNI: { name: Interní, issue: optional }",
		"",
	}, "\n")
	for _, data := range []string{src, strings.ReplaceAll(src, "\n", "\r\n")} {
		d, err := Parse([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		out, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != data {
			t.Errorf("round trip changed the file:\n%q\nwant\n%q", out, data)
		}
	}
}

func TestRoundTripKeepsUnusualLayouts(t *testing.T) {
	for name, src := range map[string]string{
		"banners":                   "# ------\n# A\n# ------\ngitlab_hosts: [a.example.com]\n\n# ------\n# B\n# ------\nweb: { port: 1 }\n",
		"compact":                   "gitlab_hosts: [a.example.com]\nbm:\n  base_url: x\nui:\n  theme: y\n",
		"commented key at column 0": "bm:\n  base_url: x\n#  open_command: [wslview]\nui:\n  theme: y\n",
		"repeated lines":            "projects:\n  A:\n    name: A\n    issue: optional\n\n  B:\n    name: B\n    issue: optional\n",
	} {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(name, err)
		}
		out, _ := d.Bytes()
		if string(out) != src {
			t.Errorf("%s: round trip changed the file:\n%q\nwant\n%q", name, out, src)
		}
	}
}
