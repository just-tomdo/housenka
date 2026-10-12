// Package bm builds links that open the Bílý motýl work-report editor
// with a pre-filled form.
package bm

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/hours"
)

// BuildURL returns the editor link for e: cfg.BaseURL with the form fields
// named in cfg.Fields. Parameters keep the order the BM UI itself produces;
// the issue parameter is omitted when e has no issue.
func BuildURL(cfg config.BM, e entry.Entry) (string, error) {
	baseURL, f := cfg.BaseURL, cfg.Fields
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("neplatná bm.base_url: %v", err)
	}

	params := [][2]string{{f.Project, e.ProjectCode}}
	if issue := strings.TrimSpace(e.IssueURL); issue != "" {
		params = append(params, [2]string{f.Issue, issue})
	}
	params = append(params,
		[2]string{f.Hours, hours.Format(e.Quarters)},
		[2]string{f.Activity, e.Activity},
		[2]string{f.Date, e.Date.Czech()},
		[2]string{f.Description, strings.TrimSpace(e.Description)},
	)

	var q strings.Builder
	q.WriteString(u.RawQuery)
	for _, p := range params {
		if q.Len() > 0 {
			q.WriteByte('&')
		}
		q.WriteString(url.QueryEscape(p[0]))
		q.WriteByte('=')
		q.WriteString(url.QueryEscape(p[1]))
	}
	u.RawQuery = q.String()
	return u.String(), nil
}
