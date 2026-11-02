package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A redirect to another host must not take the token along (Go copies
// custom headers such as PRIVATE-TOKEN on redirects).
func TestRedirectKeepsToken(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("PRIVATE-TOKEN")
		w.Write([]byte(`{"username": "x"}`))
	}))
	defer other.Close()
	gl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			http.Redirect(w, r, other.URL+"/login", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/api/v4/user", http.StatusFound) // same host: followed
	}))
	defer gl.Close()

	_, err := NewWithURL(gl.URL, "secret").CurrentUser(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token se tam neposílá") {
		t.Errorf("redirect to another host: %v", err)
	}
	if leaked != "" {
		t.Errorf("the other host got the token %q", leaked)
	}
	if got := NewWithURL(gl.URL, "secret").Host(); got != "127.0.0.1" {
		t.Errorf("Host() = %q", got)
	}
}

func TestForbiddenIsNotUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := NewWithURL(srv.URL, "secret").Issue(context.Background(), "clients/shop-web", 1)
	if !errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnauthorized) {
		t.Errorf("403 = %v", err)
	}
}

func TestAssignsToSeveral(t *testing.T) {
	for _, body := range []string{"assigned to @a, @b, and @jnovak", "assigned to @a, @jnovak, and @b"} {
		if !AssignsTo(body, "jnovak") {
			t.Errorf("AssignsTo(%q) = false", body)
		}
	}
	if AssignsTo("assigned to @a, @b, and @c and unassigned @jnovak", "jnovak") {
		t.Error("unassigned counted as assigned")
	}
}

func TestSaveTokenTightensMode(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "gitlab-token")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(home, "glpat-new"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}
