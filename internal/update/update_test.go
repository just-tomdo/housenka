package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHub serves a latest release with the given files.
func fakeGitHub(t *testing.T, tag string, files map[string]string) Updater {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	var assets []asset
	for name, body := range files {
		assets = append(assets, asset{name, srv.URL + "/download/" + name})
		mux.HandleFunc("/download/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		})
	}
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no user agent", http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": assets})
	})
	return Updater{
		LatestURL: srv.URL + "/latest",
		Client:    srv.Client(),
		UserAgent: "housenka/test",
		GOOS:      "linux",
		GOARCH:    "amd64",
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeExe(t *testing.T, body string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "housenka")
	if err := os.WriteFile(exe, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstall(t *testing.T) {
	u := fakeGitHub(t, "v1.2.0", map[string]string{
		"housenka-linux-amd64":       "new linux",
		"housenka-windows-amd64.exe": "new windows",
		"checksums.txt":              sum("new linux") + "  housenka-linux-amd64\n" + sum("new windows") + "  housenka-windows-amd64.exe\n",
	})
	r, err := u.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "v1.2.0" {
		t.Errorf("version = %q", r.Version)
	}
	exe := writeExe(t, "old")
	if err := u.Install(r, exe); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new linux" {
		t.Errorf("binary = %q", got)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v", fi.Mode())
	}
}

func TestInstallWindowsMovesRunningBinaryAside(t *testing.T) {
	u := fakeGitHub(t, "v1.2.0", map[string]string{
		"housenka-windows-amd64.exe": "new",
		"checksums.txt":              sum("new") + " *housenka-windows-amd64.exe\n",
	})
	u.GOOS = "windows"
	r, err := u.Latest()
	if err != nil {
		t.Fatal(err)
	}
	exe := writeExe(t, "old")
	if err := os.WriteFile(exe+".old", []byte("older"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(r, exe); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new" {
		t.Errorf("binary = %q", got)
	}
	if got := readFile(t, exe+".old"); got != "old" {
		t.Errorf("moved aside = %q, want the replaced binary", got)
	}
}

func TestInstallKeepsBinaryOnFailure(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"wrong checksum": {
			"housenka-linux-amd64": "tampered",
			"checksums.txt":        sum("new") + "  housenka-linux-amd64\n",
		},
		"not in checksums": {
			"housenka-linux-amd64": "new",
			"checksums.txt":        sum("new") + "  housenka-darwin-arm64\n",
		},
		"no binaries yet": {},
	} {
		t.Run(name, func(t *testing.T) {
			u := fakeGitHub(t, "v1.2.0", files)
			r, err := u.Latest()
			if err != nil {
				t.Fatal(err)
			}
			exe := writeExe(t, "old")
			if err := u.Install(r, exe); err == nil {
				t.Fatal("Install succeeded")
			}
			if got := readFile(t, exe); got != "old" {
				t.Errorf("binary = %q, want it untouched", got)
			}
		})
	}
}

func TestLatestErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	u := Updater{LatestURL: srv.URL, Client: srv.Client(), UserAgent: "x"}
	if _, err := u.Latest(); err == nil || !strings.Contains(err.Error(), "žádné vydání") {
		t.Errorf("404: err = %v", err)
	}
	u = fakeGitHub(t, "nightly", nil)
	if _, err := u.Latest(); err == nil {
		t.Error("a tag that is not a version was accepted")
	}
}

func TestVersions(t *testing.T) {
	for _, v := range []string{"v1.0.0", "v0.1.12"} {
		if !IsRelease(v) {
			t.Errorf("IsRelease(%q) = false", v)
		}
	}
	for _, v := range []string{"dev", "", "1.0.0", "v1.0", "v1.0.0-rc1", "v0.0.0-20261026124943-857d1da6f080+dirty", "v1.+0.0"} {
		if IsRelease(v) {
			t.Errorf("IsRelease(%q) = true", v)
		}
	}
	for _, c := range []struct {
		v, than string
		want    bool
	}{
		{"v1.1.0", "v1.0.0", true},
		{"v1.0.10", "v1.0.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.1.0", false},
		{"v1.1.0", "dev", false},
	} {
		if got := Newer(c.v, c.than); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.v, c.than, got)
		}
	}
}
