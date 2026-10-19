package configedit

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/just-tomdo/housenka/internal/config"
)

// BMFromLink reads the editor address and form field names from a link
// the BM editor itself produces for an entry with an issue. Its last six
// query parameters are, in this order: project, issue, hours, activity,
// date and description (the order bm.BuildURL keeps). Earlier parameters
// stay in the address.
func BMFromLink(link string) (baseURL string, f config.BMFields, err error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", f, errors.New("vlož celý odkaz začínající https://")
	}
	var names, params []string
	for _, p := range strings.Split(u.RawQuery, "&") {
		if p == "" {
			continue
		}
		name, _, _ := strings.Cut(p, "=")
		n, err := url.QueryUnescape(name)
		if err != nil {
			return "", f, fmt.Errorf("neplatný parametr %q", name)
		}
		names, params = append(names, n), append(params, p)
	}
	ordered := f.Ordered()
	if len(names) < len(ordered) {
		return "", f, fmt.Errorf("odkaz má jen %d parametrů, potřeba je %d (projekt, issue, hodiny, činnost, datum, popis) – vytvoř v BM výkaz s odkazem na issue a zkopíruj jeho odkaz", len(names), len(ordered))
	}
	tail := names[len(names)-len(ordered):]
	f = config.BMFields{Project: tail[0], Issue: tail[1], Hours: tail[2], Activity: tail[3], Date: tail[4], Description: tail[5]}
	u.RawQuery = strings.Join(params[:len(params)-len(ordered)], "&")
	u.Fragment = ""
	return u.String(), f, nil
}
