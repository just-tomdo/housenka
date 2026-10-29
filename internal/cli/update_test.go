package cli

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

	"github.com/just-tomdo/housenka/internal/update"
)

// fakeRelease serves release tag with a binary for this system, replaces
// the running binary by a file in a temporary directory and returns it.
func fakeRelease(t *testing.T, version, tag string) string {
	t.Helper()
	u := update.New("test")
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	h := sha256.Sum256([]byte("new"))
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("new")) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hex.EncodeToString(h[:]) + "  " + u.Asset() + "\n"))
	})
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": []map[string]string{
			{"name": u.Asset(), "browser_download_url": srv.URL + "/bin"},
			{"name": "checksums.txt", "browser_download_url": srv.URL + "/sums"},
		}})
	})
	u.LatestURL, u.Client = srv.URL+"/latest", srv.Client()

	exe := filepath.Join(t.TempDir(), "housenka")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersion, oldUpdater, oldExe, oldWeb := Version, newUpdater, executable, WebRunning
	t.Cleanup(func() { Version, newUpdater, executable, WebRunning = oldVersion, oldUpdater, oldExe, oldWeb })
	Version = version
	newUpdater = func() update.Updater { return u }
	executable = func() (string, error) { return exe, nil }
	WebRunning = nil
	return exe
}

func binary(t *testing.T, exe string) string {
	t.Helper()
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdate(t *testing.T) {
	exe := fakeRelease(t, "v1.0.0", "v1.1.0")
	WebRunning = func() (string, bool) { return "http://localhost:8765/", true }
	code, out, errOut := run_(t, "update")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "v1.0.0 → v1.1.0") || !strings.Contains(out, "restartuj") {
		t.Errorf("out = %q", out)
	}
	if got := binary(t, exe); got != "new" {
		t.Errorf("binary = %q", got)
	}
}

func TestUpdateUpToDate(t *testing.T) {
	exe := fakeRelease(t, "v1.1.0", "v1.1.0")
	code, out, _ := run_(t, "update")
	if code != 0 || !strings.Contains(out, "nejnovější verzi v1.1.0") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if got := binary(t, exe); got != "old" {
		t.Errorf("binary = %q, want it untouched", got)
	}
}

func TestUpdateCheck(t *testing.T) {
	exe := fakeRelease(t, "v1.0.0", "v1.1.0")
	code, out, _ := run_(t, "update", "--check")
	if code != 0 || !strings.Contains(out, "Je k dispozici v1.1.0, máš v1.0.0") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if got := binary(t, exe); got != "old" {
		t.Errorf("binary = %q, want it untouched", got)
	}
}

func TestUpdateLocalBuildNeedsForce(t *testing.T) {
	exe := fakeRelease(t, "v0.0.0-20261026124943-857d1da6f080+dirty", "v1.1.0")
	code, _, errOut := run_(t, "update")
	if code != 1 || !strings.Contains(errOut, exe) || !strings.Contains(errOut, "--force") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	if got := binary(t, exe); got != "old" {
		t.Fatalf("binary = %q, want it untouched", got)
	}
	if code, _, errOut := run_(t, "update", "--force"); code != 0 {
		t.Fatalf("--force: code=%d err=%q", code, errOut)
	}
	if got := binary(t, exe); got != "new" {
		t.Errorf("binary = %q", got)
	}
}

func TestUpdateUsage(t *testing.T) {
	fakeRelease(t, "v1.0.0", "v1.1.0")
	if code, _, _ := run_(t, "update", "now"); code != 2 {
		t.Errorf("code = %d", code)
	}
}
