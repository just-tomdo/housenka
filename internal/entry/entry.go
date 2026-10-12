// Package entry defines a single work-report entry.
package entry

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
)

// Entry is one line of a work report.
type Entry struct {
	ID          string     `json:"id"`
	Date        civil.Date `json:"date"`
	IssueURL    string     `json:"issue_url,omitempty"` // empty only for projects with issue: optional
	ProjectCode string     `json:"project"`
	Activity    string     `json:"activity"`
	Quarters    int        `json:"quarters"` // duration in quarter-hours (2.75 h = 11)
	Description string     `json:"description"`
	Sent        bool       `json:"sent"` // user confirmed the entry was submitted to BM
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// NewID returns a short random identifier.
func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b)
}
