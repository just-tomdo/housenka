package gitlaburl

import (
	"slices"
	"strings"
	"testing"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		in   string
		want RepoRef
	}{
		{"https://gitlab.example.com/clients/acme-web", RepoRef{Host: "gitlab.example.com", Path: "clients/acme-web"}},
		{" https://GitLab.Example.com/clients/acme-web/ ", RepoRef{Host: "gitlab.example.com", Path: "clients/acme-web"}},
		{"https://gitlab.example.com/clients/acme-web.git", RepoRef{Host: "gitlab.example.com", Path: "clients/acme-web"}},
		{"https://gitlab.example.com/interni/backend/api/-/issues/12#note_1", RepoRef{Host: "gitlab.example.com", Path: "interni/backend/api"}},
		{"https://gitlab.example.com/clients/acme-web/-/tree/main?ref_type=heads", RepoRef{Host: "gitlab.example.com", Path: "clients/acme-web"}},
		{"git@gitlab.example.com:clients/acme-web.git", RepoRef{Host: "gitlab.example.com", Path: "clients/acme-web"}},
		{"clients/acme-web", RepoRef{Path: "clients/acme-web"}},
		{"/clients/acme-web/", RepoRef{Path: "clients/acme-web"}},
		{"interni/*", RepoRef{Path: "interni", Wildcard: true}},
		{"https://gitlab.example.com/interni/*", RepoRef{Host: "gitlab.example.com", Path: "interni", Wildcard: true}},
	}
	for _, tt := range tests {
		got, err := ParseRepo(tt.in)
		if err != nil {
			t.Errorf("ParseRepo(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseRepo(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseRepoErrors(t *testing.T) {
	for in, want := range map[string]string{
		"":                                "zadej odkaz",
		"clients":                         "je jen skupina",
		"https://gitlab.example.com/acme": "je jen skupina",
		"clients//acme":                   "neplatná cesta",
		"clients/*/acme":                  "neplatná cesta",
		"https:///clients/acme":           "chybí doména",
		"git@:clients/acme":               "SSH adresa",
	} {
		_, err := ParseRepo(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseRepo(%q) error = %v, want %q", in, err, want)
		}
	}
}

func TestRepoPatterns(t *testing.T) {
	r := RepoRef{Path: "interni/backend/api"}
	if got, want := r.Patterns(), []string{"interni/backend/api", "interni/backend/*", "interni/*"}; !slices.Equal(got, want) {
		t.Errorf("Patterns = %v, want %v", got, want)
	}
	w := RepoRef{Path: "interni/backend", Wildcard: true}
	if got, want := w.Patterns(), []string{"interni/backend/*", "interni/*"}; !slices.Equal(got, want) {
		t.Errorf("wildcard Patterns = %v, want %v", got, want)
	}
	if w.Pattern() != "interni/backend/*" || r.Pattern() != "interni/backend/api" {
		t.Errorf("Pattern: %q %q", w.Pattern(), r.Pattern())
	}
}
