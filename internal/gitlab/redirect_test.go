package gitlab

import (
	"net/http"
	"testing"
)

func TestKeepTokenRules(t *testing.T) {
	tests := []struct {
		from, to string
		ok       bool
	}{
		{"https://gitlab.example.com/api/v4/user", "https://gitlab.example.com/api/v4/user/", true},
		{"https://gitlab.example.com/api/v4/user", "https://GitLab.example.com:443/api/v4/user", true},
		{"http://gitlab.example.com/api/v4/user", "https://gitlab.example.com/api/v4/user", true},
		{"https://gitlab.example.com/api/v4/user", "http://gitlab.example.com/api/v4/user", false},
		{"https://gitlab.example.com/api/v4/user", "https://sso.example.com/login", false},
		{"https://gitlab.example.com/api/v4/user", "https://gitlab.example.com:8443/api/v4/user", false},
	}
	for _, tt := range tests {
		from, _ := http.NewRequest(http.MethodGet, tt.from, nil)
		to, _ := http.NewRequest(http.MethodGet, tt.to, nil)
		if err := keepToken(to, []*http.Request{from}); (err == nil) != tt.ok {
			t.Errorf("%s → %s: %v", tt.from, tt.to, err)
		}
	}
}
