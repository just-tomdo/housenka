package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/just-tomdo/housenka/internal/civil"
	"github.com/just-tomdo/housenka/internal/config"
	"github.com/just-tomdo/housenka/internal/store"
)

// PortEnv overrides web.port from the config.
const PortEnv = "HOUSENKA_WEB_PORT"

// state describes a running server; it is written to <home>/web.json so
// that `housenka web stop` and `status` can find it.
type state struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

func statePath() (string, error) {
	dir, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "web.json"), nil
}

func logPath() (string, error) {
	dir, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "web.log"), nil
}

func readState() (state, bool) {
	p, err := statePath()
	if err != nil {
		return state{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return state{}, false
	}
	var st state
	if json.Unmarshal(data, &st) != nil || st.Port == 0 {
		return state{}, false
	}
	return st, true
}

func writeState(st state) error {
	p, err := statePath()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// removeState deletes the state file if it still belongs to the server with token.
func removeState(token string) {
	if st, ok := readState(); ok && st.Token == token {
		if p, err := statePath(); err == nil {
			os.Remove(p)
		}
	}
}

var client = &http.Client{Timeout: 2 * time.Second}

// alive reports whether a Housenka server answers on port.
func alive(port int) bool {
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/_/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode == http.StatusOK && strings.HasPrefix(string(body), "housenka")
}

// running returns the state of a server that is up; a stale state file is removed.
func running() (state, bool) {
	st, ok := readState()
	if !ok {
		return st, false
	}
	if alive(st.Port) {
		return st, true
	}
	removeState(st.Token)
	return st, false
}

// Options configure starting the server.
type Options struct {
	Config     *config.Config // nil when the file is invalid; the web offers fixing it
	ConfigPath string
	Port       int                  // 0 = web.port from the config
	Open       func(u string) error // opens the browser; nil = do not open
}

func (o Options) port() int {
	switch {
	case o.Port != 0:
		return o.Port
	case o.Config != nil:
		return o.Config.Web.Port
	}
	return config.DefaultWebPort
}

// Serve runs the server in the foreground until Ctrl+C, SIGTERM or
// `housenka web stop`.
func Serve(o Options, out io.Writer) error {
	if st, ok := running(); ok {
		return fmt.Errorf("Housenka už běží na %s (PID %d) – vypneš ji příkazem housenka web stop", st.URL, st.PID)
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	s, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	port := o.port()
	srv, err := New(o.Config, o.ConfigPath, s, civil.Today, port)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		if port < 1024 && errors.Is(err, syscall.EACCES) {
			return fmt.Errorf("port %d smí na tomto systému otevřít jen správce (%v) – povol ho podle README (Adresa bez portu), nebo použij port nad 1023", port, err)
		}
		return fmt.Errorf("port %d nejde použít (%v) – nastav jiný v configu (web.port), proměnnou %s nebo --port", port, err, PortEnv)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	srv.shutdown = stop
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		// Running requests (a save, the answer to `web stop`) may finish.
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()

	st := state{PID: os.Getpid(), Port: port, URL: srv.URL(), Token: srv.token}
	if err := writeState(st); err != nil {
		return err
	}
	defer removeState(st.Token)
	if o.Config != nil && !srv.themes.valid(o.Config.Web.Theme) {
		fmt.Fprintf(out, "Varování: neznámé téma web.theme %q, použije se auto (možnosti: %s)\n", o.Config.Web.Theme, strings.Join(srv.themes.ids(), ", "))
	}
	fmt.Fprintf(out, "Housenka běží na %s – Ctrl+C ji ukončí\n", st.URL)
	if o.Open != nil {
		if err := o.Open(st.URL); err != nil {
			fmt.Fprintf(out, "Prohlížeč se nepodařilo otevřít (%v) – otevři adresu ručně.\n", err)
		}
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-stopped
	fmt.Fprintln(out, "Housenka ukončena.")
	return nil
}

// Start runs the server in the background (a detached copy of this
// program running `web`) and returns once it answers.
func Start(o Options, out io.Writer) error {
	if st, ok := running(); ok {
		fmt.Fprintf(out, "Housenka už běží na %s (PID %d)\n", st.URL, st.PID)
		if o.Open != nil {
			o.Open(st.URL)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	lp, err := logPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "--- %s spuštění na pozadí\n", time.Now().Format(time.DateTime))

	cmd := exec.Command(exe, "--config", o.ConfigPath, "web", "--no-open", "--port", strconv.Itoa(o.port()))
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-exited:
			return fmt.Errorf("server se nespustil – podrobnosti jsou v %s:\n%s", lp, tail(lp, 5))
		case <-deadline:
			return fmt.Errorf("server neodpovídá – podrobnosti jsou v %s", lp)
		case <-time.After(100 * time.Millisecond):
		}
		if st, ok := readState(); ok && st.PID == cmd.Process.Pid && alive(st.Port) {
			fmt.Fprintf(out, "Housenka běží na pozadí na %s (PID %d)\nVypneš ji příkazem housenka web stop.\n", st.URL, st.PID)
			if o.Open != nil {
				if err := o.Open(st.URL); err != nil {
					fmt.Fprintf(out, "Prohlížeč se nepodařilo otevřít (%v) – otevři adresu ručně.\n", err)
				}
			}
			cmd.Process.Release()
			return nil
		}
	}
}

// tail returns the last n lines of a file.
func tail(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	return "  " + strings.Join(lines[max(len(lines)-n, 0):], "\n  ")
}

// Stop shuts down the server running in the background (or in another terminal).
func Stop(out io.Writer) error {
	st, ok := running()
	if !ok {
		fmt.Fprintln(out, "Housenka na pozadí neběží.")
		return nil
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(st.Port))+"/_/shutdown", nil)
	req.Header.Set("X-Shutdown-Token", st.Token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("server se nepodařilo vypnout: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server odmítl vypnutí (%s)", resp.Status)
	}
	for range 50 {
		if !alive(st.Port) {
			fmt.Fprintf(out, "Housenka na %s je vypnutá.\n", st.URL)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server na %s se nevypnul (PID %d)", st.URL, st.PID)
}

// Status reports whether the server is running.
func Status(out io.Writer) error {
	if st, ok := running(); ok {
		fmt.Fprintf(out, "Housenka běží na %s (PID %d)\n", st.URL, st.PID)
		return nil
	}
	fmt.Fprintln(out, "Housenka na pozadí neběží.")
	return nil
}
