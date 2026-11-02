package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeGitLab(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id": 7, "username": "jnovak", "name": "Jan Novák"}`)
	})
	mux.HandleFunc("GET /api/v4/issues", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("scope") != "assigned_to_me" || q.Get("state") != "opened" {
			t.Errorf("query %v", q)
		}
		switch q.Get("page") {
		case "1":
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"iid": 1, "title": "První", "state": "opened", "web_url": "https://gitlab.example.com/clients/shop-web/-/issues/1", "labels": ["Bug"], "due_date": "2026-11-20"}]`)
		case "2":
			w.Header().Set("X-Next-Page", "")
			fmt.Fprint(w, `[{"iid": 2, "title": "Druhé", "state": "opened", "web_url": "https://gitlab.example.com/clients/shop-web/-/work_items/2", "due_date": null}]`)
		default:
			t.Errorf("page %q", q.Get("page"))
		}
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues/{iid}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("project") != "clients/shop-web" {
			t.Errorf("project %q", r.PathValue("project"))
		}
		if r.PathValue("iid") == "404" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"iid": 3, "state": "closed", "web_url": "https://gitlab.example.com/clients/shop-web/-/issues/3", "assignees": [{"username": "jnovak"}]}`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues/{iid}/notes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("activity_filter") != "only_activity" {
			t.Errorf("notes query %v", r.URL.Query())
		}
		fmt.Fprint(w, `[
			{"body": "changed the description", "system": true, "created_at": "2026-10-20T10:00:00Z"},
			{"body": "assigned to @jnovak and unassigned @other", "system": true, "created_at": "2026-10-15T08:30:00Z"},
			{"body": "assigned to @jnovak", "system": true, "created_at": "2026-10-01T08:00:00Z"}
		]`)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient(t *testing.T) {
	srv := fakeGitLab(t)
	c := NewWithURL(srv.URL, "secret")
	ctx := context.Background()

	u, err := c.CurrentUser(ctx)
	if err != nil || u.Username != "jnovak" {
		t.Fatalf("user %+v %v", u, err)
	}
	list, err := c.AssignedIssues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Title != "První" || list[0].DueDate != "2026-11-20" || list[1].IID != 2 {
		t.Errorf("issues %+v", list)
	}
	is, err := c.Issue(ctx, "clients/shop-web", 3)
	if err != nil || is.State != "closed" || !is.AssignedTo("JNovak") || is.AssignedTo("other") {
		t.Errorf("issue %+v %v", is, err)
	}
	if _, err := c.Issue(ctx, "clients/shop-web", 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("404: %v", err)
	}
	at, err := c.AssignedAt(ctx, "clients/shop-web", 1, "jnovak")
	if err != nil || !at.Equal(time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("assigned at %v %v", at, err)
	}

	if _, err := NewWithURL(srv.URL, "wrong").CurrentUser(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("bad token: %v", err)
	}
}

func TestAssignsTo(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"assigned to @jnovak", true},
		{"assigned to @other and @jnovak", true},
		{"assigned to @a, @b and @jnovak", true},
		{"assigned to @jnovak and unassigned @other", true},
		{"assigned to @other and unassigned @jnovak", false},
		{"unassigned @jnovak", false},
		{"assigned to @jnovak2", false},
		{"mentioned in issue #4", false},
	}
	for _, tt := range tests {
		if got := AssignsTo(tt.body, "jnovak"); got != tt.want {
			t.Errorf("AssignsTo(%q) = %v", tt.body, got)
		}
	}
}

func TestToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv(TokenEnv, "")
	if _, err := Token(home); !errors.Is(err, ErrNoToken) {
		t.Errorf("no token: %v", err)
	}
	if err := SaveToken(home, " glpat-abc \n"); err != nil {
		t.Fatal(err)
	}
	if tok, err := Token(home); err != nil || tok != "glpat-abc" {
		t.Errorf("token %q %v", tok, err)
	}
	info, _ := os.Stat(filepath.Join(home, "gitlab-token"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v", info.Mode().Perm())
	}
	t.Setenv(TokenEnv, "from-env")
	if tok, _ := Token(home); tok != "from-env" || !TokenFromEnv() {
		t.Errorf("env must win, got %q", tok)
	}
	if err := SaveToken(home, "a b"); err == nil {
		t.Error("token with a space accepted")
	}
	if err := SaveToken(home, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "gitlab-token")); !os.IsNotExist(err) {
		t.Error("empty token must remove the file")
	}
}
