// Package gitlab is a small client of the GitLab REST API (v4): the issues
// assigned to the token's user, and when an issue was assigned to them.
package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client calls one GitLab instance with a personal access token (scope read_api).
type Client struct {
	base  string // https://gitlab.example.com/api/v4
	host  string
	token string
	http  *http.Client
}

// New returns a client of the GitLab at host ("gitlab.example.com").
func New(host, token string) *Client {
	return NewWithURL("https://"+host, token)
}

// NewWithURL returns a client of the GitLab at baseURL (used by tests).
func NewWithURL(baseURL, token string) *Client {
	c := &Client{base: strings.TrimRight(baseURL, "/") + "/api/v4", token: token}
	if u, err := url.Parse(baseURL); err == nil {
		c.host = strings.ToLower(u.Hostname())
	}
	c.http = &http.Client{Timeout: 30 * time.Second, CheckRedirect: keepToken}
	return c
}

// keepToken refuses a redirect to another host or from https to http: Go
// would send the PRIVATE-TOKEN header along (it drops only Authorization
// and cookies), e.g. to an SSO proxy in front of a company GitLab.
func keepToken(req *http.Request, via []*http.Request) error {
	first := via[0].URL
	if !strings.EqualFold(req.URL.Host, first.Host) || first.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("GitLab přesměroval požadavek na %s://%s – token se tam neposílá, zkontroluj host v issues.yaml", req.URL.Scheme, req.URL.Host)
	}
	if len(via) >= 10 {
		return errors.New("GitLab přesměrovává dokola")
	}
	return nil
}

// Host returns the host name of the GitLab ("gitlab.example.com").
func (c *Client) Host() string { return c.host }

// User is a GitLab user.
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Issue is the part of a GitLab issue housenka uses.
type Issue struct {
	IID       int       `json:"iid"`
	Title     string    `json:"title"`
	State     string    `json:"state"` // opened | closed
	WebURL    string    `json:"web_url"`
	Labels    []string  `json:"labels"`
	DueDate   string    `json:"due_date"` // "2026-10-12" or ""
	Assignees []User    `json:"assignees"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AssignedTo reports whether username is among the assignees.
func (i Issue) AssignedTo(username string) bool {
	for _, u := range i.Assignees {
		if strings.EqualFold(u.Username, username) {
			return true
		}
	}
	return false
}

var (
	// ErrUnauthorized means the token is missing, wrong or expired (401).
	ErrUnauthorized = errors.New("GitLab odmítl token – je neplatný nebo vypršel")
	// ErrForbidden means the token may not do this (403): it lacks the
	// read_api scope, or the user lost access to a project.
	ErrForbidden = errors.New("GitLab odepřel přístup – token nemá oprávnění read_api, nebo k projektu už nemáš přístup")
	// ErrNotFound means the object does not exist or the user cannot see it.
	ErrNotFound = errors.New("GitLab objekt nenašel")
)

// CurrentUser returns the user the token belongs to.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var u User
	_, err := c.get(ctx, "/user", nil, &u)
	return u, err
}

// maxPages stops a runaway pagination (100 issues a page).
const maxPages = 50

// AssignedIssues returns all open issues assigned to the token's user.
func (c *Client) AssignedIssues(ctx context.Context) ([]Issue, error) {
	q := url.Values{"scope": {"assigned_to_me"}, "state": {"opened"}, "per_page": {"100"}}
	var all []Issue
	for page := 1; page > 0 && page <= maxPages; {
		q.Set("page", strconv.Itoa(page))
		var batch []Issue
		h, err := c.get(ctx, "/issues", q, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		page, _ = strconv.Atoi(h.Get("X-Next-Page")) // empty on the last page
	}
	return all, nil
}

// Issue returns one issue of a project ("clients/shop-web").
func (c *Client) Issue(ctx context.Context, projectPath string, iid int) (Issue, error) {
	var is Issue
	_, err := c.get(ctx, fmt.Sprintf("/projects/%s/issues/%d", url.PathEscape(projectPath), iid), nil, &is)
	return is, err
}

type note struct {
	Body      string    `json:"body"`
	System    bool      `json:"system"`
	CreatedAt time.Time `json:"created_at"`
}

// AssignedAt returns when the issue was last assigned to username, read
// from its system notes ("assigned to @username"); zero when not found.
func (c *Client) AssignedAt(ctx context.Context, projectPath string, iid int, username string) (time.Time, error) {
	q := url.Values{"activity_filter": {"only_activity"}, "sort": {"desc"}, "order_by": {"created_at"}, "per_page": {"100"}}
	var notes []note
	if _, err := c.get(ctx, fmt.Sprintf("/projects/%s/issues/%d/notes", url.PathEscape(projectPath), iid), q, &notes); err != nil {
		return time.Time{}, err
	}
	for _, n := range notes {
		if n.System && AssignsTo(n.Body, username) {
			return n.CreatedAt, nil
		}
	}
	return time.Time{}, nil
}

var assignedRe = regexp.MustCompile(`(?i)(?:^|\s)assigned to ((?:@[\w.-]+(?:,\s*(?:and\s+)?|\s+and\s+)?)+)`)

// AssignsTo reports whether a system note body such as "assigned to @a,
// @b, and @c and unassigned @d" assigns the issue to username.
func AssignsTo(body, username string) bool {
	for _, m := range assignedRe.FindAllStringSubmatch(body, -1) {
		for _, f := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' }) {
			if strings.EqualFold(strings.TrimPrefix(f, "@"), username) {
				return true
			}
		}
	}
	return false
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) (http.Header, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitLab není dostupný: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusForbidden:
		return nil, ErrForbidden
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("GitLab vrátil %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("nečitelná odpověď GitLabu: %w", err)
	}
	return resp.Header, nil
}
