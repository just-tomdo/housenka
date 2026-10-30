// Package update replaces the housenka binary with the latest GitHub
// release, the same way install.sh installs it: the binary for this system
// is downloaded and checked against the release's checksums.txt.
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/just-tomdo/housenka/internal/atomicfile"
)

// LatestURL is the GitHub API endpoint of the latest release.
const LatestURL = "https://api.github.com/repos/just-tomdo/housenka/releases/latest"

// maxSize caps a download; the binaries have about 15 MB.
const maxSize = 200 << 20

// Updater finds and installs releases. Fields are replaceable in tests.
type Updater struct {
	LatestURL string
	Client    *http.Client
	UserAgent string
	GOOS      string
	GOARCH    string
}

// New returns the Updater for this system; version goes to the User-Agent.
func New(version string) Updater {
	return Updater{
		LatestURL: LatestURL,
		Client:    &http.Client{Timeout: 2 * time.Minute},
		UserAgent: "housenka/" + version,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}

// Release is a published version and its downloadable files.
type Release struct {
	Version string            // tag, e.g. v1.2.3
	assets  map[string]string // file name → download URL
}

// Asset is the name of the release binary for this system
// (scripts/build-release.sh).
func (u Updater) Asset() string {
	name := "housenka-" + u.GOOS + "-" + u.GOARCH
	if u.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Latest returns the latest release.
func (u Updater) Latest() (Release, error) {
	resp, err := u.get(u.LatestURL, "application/vnd.github+json")
	if err != nil {
		return Release{}, fmt.Errorf("nepodařilo se zjistit poslední verzi: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("nepodařilo se zjistit poslední verzi: %w", err)
	}
	if !IsRelease(body.TagName) {
		return Release{}, fmt.Errorf("poslední vydání má nečekané označení %q", body.TagName)
	}
	r := Release{Version: body.TagName, assets: map[string]string{}}
	for _, a := range body.Assets {
		r.assets[a.Name] = a.URL
	}
	return r, nil
}

// Install downloads the release binary for this system, checks it against
// checksums.txt and replaces the file exe with it. Nothing is replaced when
// any step fails.
func (u Updater) Install(r Release, exe string) error {
	asset := u.Asset()
	binURL, ok := r.assets[asset]
	sumsURL, ok2 := r.assets["checksums.txt"]
	if !ok || !ok2 {
		// A release drafted in the GitHub UI gets its binaries a few
		// minutes later from the release workflow.
		return fmt.Errorf("vydání %s zatím nemá %s nebo checksums.txt (pokud právě vzniklo, zkus to za pár minut)", r.Version, asset)
	}
	sums, err := u.download(sumsURL)
	if err != nil {
		return fmt.Errorf("stažení checksums.txt selhalo: %w", err)
	}
	want := checksum(sums, asset)
	if want == "" {
		return fmt.Errorf("%s chybí v checksums.txt", asset)
	}
	bin, err := u.download(binURL)
	if err != nil {
		return fmt.Errorf("stažení %s selhalo: %w", asset, err)
	}
	if got := sha256.Sum256(bin); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("kontrolní součet %s nesedí, nic jsem neinstaloval", asset)
	}
	if err := replace(exe, bin, u.GOOS); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("nemám právo zapisovat do %s: %w", filepath.Dir(exe), err)
		}
		return err
	}
	return nil
}

// replace writes the new binary over exe. Windows does not let a running
// program be overwritten, only renamed, so it is moved aside to exe.old
// first; that file is removed by the next update.
func replace(exe string, bin []byte, goos string) error {
	if goos != "windows" {
		return atomicfile.Write(exe, bin, 0o755)
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := atomicfile.Write(exe, bin, 0o755); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}

func (u Updater) get(url, accept string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", u.UserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errors.New("na GitHubu zatím není žádné vydání")
		case http.StatusForbidden, http.StatusTooManyRequests:
			return nil, errors.New("GitHub dočasně omezil počet dotazů, zkus to později")
		}
		return nil, fmt.Errorf("GitHub odpověděl %s", resp.Status)
	}
	return resp, nil
}

func (u Updater) download(url string) ([]byte, error) {
	resp, err := u.get(url, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSize {
		return nil, errors.New("soubor je nečekaně velký")
	}
	return b, nil
}

// checksum finds the SHA-256 of name in sha256sum output.
func checksum(sums []byte, name string) string {
	for line := range strings.Lines(string(sums)) {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

// IsRelease reports whether v is a release tag (vMAJOR.MINOR.PATCH), not a
// local build ("dev") or a Go pseudo-version.
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

// Newer reports whether release v is newer than release than.
func Newer(v, than string) bool {
	a, ok := parse(v)
	b, ok2 := parse(than)
	if !ok || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var n [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if !strings.HasPrefix(v, "v") || len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 || strings.HasPrefix(p, "+") {
			return n, false
		}
		n[i] = x
	}
	return n, true
}
