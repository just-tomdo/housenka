package configedit

import (
	"os"
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
