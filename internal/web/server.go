// Package web is the browser interface of Housenka: a local HTTP server
// rendering HTML with html/template and updating parts of the page with
// HTMX. It offers the same features as the TUI.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/entry"
	"github.com/just-tomdo/housenka/internal/rules"
)

//go:embed templates static
var assets embed.FS

// Store is the persistence the web interface needs.
type Store interface {
	Range(from, to civil.Date) ([]entry.Entry, error)
	All() ([]entry.Entry, error)
	Save(e entry.Entry) (entry.Entry, error)
	Delete(id string) error
}

// Server serves the web interface. Create it with New.
type Server struct {
	cfgPath string
	store   Store
	clock   func() civil.Date
	port    int
	hosts   []string // allowed Host names, fixed at start like the port
	tmpl    *template.Template
	themes  *themeSet
	static  http.Handler

	// token authorizes POST /_/shutdown; it is kept in the state file.
	token    string
	shutdown func()

	mu       sync.Mutex
	cfg      *config.Config
	rules    *rules.Rules
	cfgStamp atomicfile.Stamp
	cfgErr   error // the config file changed but is invalid; the previous one is used
}

// ExampleConfig is the commented example configuration, set by the main
// package; the setup guide compares the configuration with it.
var ExampleConfig []byte

// New creates the server. cfg is the loaded configuration, or nil when it
// is missing or invalid (then cfgPath is read for the error and only the
// text editor of the configuration is offered until it is fixed). cfgPath
// is watched for changes; port is the port the server listens on (for the
// Host check).
func New(cfg *config.Config, cfgPath string, s Store, clock func() civil.Date, port int) (*Server, error) {
	themes, err := loadThemes(assets, "static/themes")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	srv := &Server{
		cfgPath: cfgPath, store: s, clock: clock, port: port, tmpl: tmpl, themes: themes,
		static: http.FileServerFS(static), token: randomToken(),
		cfg: cfg, cfgStamp: statConfig(cfgPath),
	}
	if cfg == nil {
		srv.cfg, srv.cfgErr = config.Load(cfgPath)
	}
	srv.hosts = []string{"127.0.0.1", "localhost"}
	if srv.cfg != nil {
		srv.rules = rules.New(srv.cfg)
		srv.hosts = append(srv.hosts, srv.cfg.Web.Hostnames...)
	}
	return srv, nil
}

func randomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func statConfig(path string) atomicfile.Stamp {
	st, _ := atomicfile.StatStamp(path)
	return st
}

// config returns the current configuration, re-reading the file when it
// changed. An invalid file keeps the previous configuration and reports
// the error; the configuration is nil when no valid one was loaded yet.
func (s *Server) config() (*config.Config, *rules.Rules, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := statConfig(s.cfgPath); s.cfgPath != "" && st != s.cfgStamp {
		s.cfgStamp = st
		cfg, err := config.Load(s.cfgPath)
		s.cfgErr = err
		if err == nil {
			s.cfg, s.rules = cfg, rules.New(cfg)
		}
	}
	return s.cfg, s.rules, s.cfgErr
}

// Handler returns the HTTP handler with all routes and security checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.cacheStatic(s.static)))
	mux.HandleFunc("GET /themes.css", s.themesCSS)
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /view", s.view)
	mux.HandleFunc("GET /entries/new", s.newEntry)
	mux.HandleFunc("GET /entries/{id}/edit", s.editEntry)
	mux.HandleFunc("GET /form/refresh", s.refreshForm)
	mux.HandleFunc("POST /entries", s.saveEntry)
	mux.HandleFunc("POST /entries/{id}", s.saveEntry)
	mux.HandleFunc("POST /entries/{id}/sent", s.toggleSent)
	mux.HandleFunc("DELETE /entries/{id}", s.deleteEntry)
	mux.HandleFunc("GET /issue", s.issueSummary)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /help", s.help)
	mux.HandleFunc("GET /_/health", s.health)
	mux.HandleFunc("POST /_/shutdown", s.shutdownHandler)
	s.settingsRoutes(mux)
	return s.checkHost(s.checkCSRF(s.requireConfig(mux)))
}

// requireConfig sends every page to the configuration text editor while
// no valid configuration is loaded.
func (s *Server) requireConfig(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if cfg, _, _ := s.config(); cfg != nil || p == "/settings/yaml" || p == "/themes.css" ||
			strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/_/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("HX-Request") != "" {
			w.Header().Set("HX-Redirect", "/settings/yaml")
			return
		}
		http.Redirect(w, r, "/settings/yaml", http.StatusSeeOther)
	})
}

func (s *Server) cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// hostnames are the names the browser may use for the server. They are
// read from the config at start: like the port, a changed web.hostnames
// applies after a restart, so saving it cannot lock out the open page.
func (s *Server) hostnames() []string { return s.hosts }

// URL returns the address to open in the browser: the first configured
// host name, or localhost.
func (s *Server) URL() string {
	host := "localhost"
	if len(s.hosts) > 2 {
		host = s.hosts[2]
	}
	if s.port == 80 {
		return "http://" + host + "/"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.port)) + "/"
}

// checkHost rejects requests for other host names, which protects against
// DNS rebinding: a foreign page whose domain resolves to 127.0.0.1 still
// sends its own name in the Host header.
func (s *Server) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowedHost(r.Host) {
			http.Error(w, "nepovolený hostitel "+r.Host+" – přidej ho do web.hostnames v configu", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowedHost(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil && s.port == 80 {
		host, port, err = hostport, "80", nil // browsers omit the default port
	}
	if err != nil || port != strconv.Itoa(s.port) {
		return false
	}
	return slices.ContainsFunc(s.hostnames(), func(h string) bool { return strings.EqualFold(h, host) })
}

const csrfCookie = "housenka_csrf"

// checkCSRF protects changing requests: they must come from a page of this
// server (Origin, which browsers send with every POST and DELETE) and carry
// the token from the cookie in X-CSRF-Token (double submit; HTMX sends it
// from hx-headers).
func (s *Server) checkCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.URL.Path == "/_/shutdown" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || u.Scheme != "http" || !s.allowedHost(u.Host) {
			http.Error(w, "požadavek z cizí stránky", http.StatusForbidden)
			return
		}
		c, err := r.Cookie(csrfCookie)
		if err != nil || c.Value == "" ||
			subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
			http.Error(w, "neplatný bezpečnostní token – načti stránku znovu", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfToken returns the request's token, issuing a new cookie when missing.
func csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) == 32 {
		return c.Value
	}
	t := randomToken()
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return t
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("housenka\n"))
}

func (s *Server) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	if s.shutdown == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Shutdown-Token")), []byte(s.token)) != 1 {
		http.Error(w, "neplatný token", http.StatusForbidden)
		return
	}
	w.Write([]byte("ok\n"))
	go s.shutdown()
}

func (s *Server) themesCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(s.themes.css))
}
