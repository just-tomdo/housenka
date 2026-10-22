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

var client = &http.Client{Timeout: 3 * time.Second}

// healthPID asks the server on port who it is: the PID of a Housenka
// server answering there.
func healthPID(port int) (int, bool) {
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/_/health")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100))
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(body)), "housenka")
	if resp.StatusCode != http.StatusOK || !ok {
		return 0, false
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(rest)) // 0 for older versions
	return pid, true
}

// portFree reports whether nothing listens on port.
func portFree(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

type serverStatus int

const (
	stopped      serverStatus = iota
	up                        // answers
	unresponsive              // its process lives and holds the port, but it does not answer
)

// lookup finds the server of this Housenka home. A state file is removed
// only when its server is certainly gone: the process does not exist or
// the port is free. port is where to look for a server whose state file
// is missing (0 = do not look).
func lookup(port int) (state, serverStatus) {
	st, ok := readState()
	if ok {
		if pid, answers := healthPID(st.Port); answers && (pid == 0 || pid == st.PID) {
			return st, up
		}
		if processAlive(st.PID) && !portFree(st.Port) {
			return st, unresponsive
		}
		removeState(st.Token)
	}
	// No state file (deleted meanwhile?): a running server rewrites it
	// within stateCheckEvery; until then it is found by its port.
	if port != 0 {
		if pid, answers := healthPID(port); answers && pid != 0 {
			return state{PID: pid, Port: port, URL: fmt.Sprintf("http://localhost:%d/", port)}, up
		}
	}
	return state{}, stopped
}

// running reports a server that answers.
func running() (state, bool) {
	st, s := lookup(0)
	return st, s == up
}

// stateCheckEvery is how often a running server makes sure its state file
// exists and is its own, so `web stop` can always find it.
var stateCheckEvery = 5 * time.Second

func keepState(ctx context.Context, st state) {
	t := time.NewTicker(stateCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur, ok := readState()
		if ok && cur.Token == st.Token {
			continue
		}
		if ok && cur.Token != st.Token {
			if pid, answers := healthPID(cur.Port); answers && pid != os.Getpid() {
				continue // another live server owns the file
			}
		}
		writeState(st)
	}
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
	switch st, status := lookup(o.port()); status {
	case up:
		return fmt.Errorf("Housenka už běží na %s (PID %d) – vypneš ji příkazem housenka web stop", st.URL, st.PID)
	case unresponsive:
		return fmt.Errorf("Housenka (PID %d) na portu %d neodpovídá – ukonči ji příkazem housenka web stop --force", st.PID, st.Port)
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
	go keepState(ctx, st)
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
	switch st, status := lookup(o.port()); status {
	case up:
		fmt.Fprintf(out, "Housenka už běží na %s (PID %d)\n", st.URL, st.PID)
		if o.Open != nil {
			o.Open(st.URL)
		}
		return nil
	case unresponsive:
		return fmt.Errorf("Housenka (PID %d) na portu %d neodpovídá – ukonči ji příkazem housenka web stop --force", st.PID, st.Port)
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
			// Do not leave a half-started server behind.
			terminate(cmd.Process.Pid)
			return fmt.Errorf("server neodpověděl do 10 s, ukončil jsem ho – podrobnosti jsou v %s", lp)
		case <-time.After(100 * time.Millisecond):
		}
		if st, ok := readState(); ok && st.PID == cmd.Process.Pid {
			if _, answers := healthPID(st.Port); !answers {
				continue
			}
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

// Stop shuts down the server of this Housenka home: politely through its
// shutdown endpoint, and when it does not end in time, or does not answer
// at all and force is set, by ending its process. port is where to look
// for a server whose state file is missing.
func Stop(port int, force bool, out io.Writer) error {
	st, status := lookup(port)
	switch status {
	case stopped:
		fmt.Fprintln(out, "Housenka na pozadí neběží.")
		return nil
	case unresponsive:
		if !force {
			return fmt.Errorf("Housenka (PID %d) na portu %d neodpovídá – ukonči ji příkazem housenka web stop --force", st.PID, st.Port)
		}
		return kill(st, out)
	}
	if st.Token == "" {
		// Found by its port only: the server rewrites its state file soon.
		for deadline := time.Now().Add(stateCheckEvery + 2*time.Second); time.Now().Before(deadline); {
			time.Sleep(100 * time.Millisecond)
			if cur, ok := readState(); ok && cur.PID == st.PID {
				st = cur
				break
			}
		}
	}
	if st.Token != "" {
		req, _ := http.NewRequest(http.MethodPost, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(st.Port))+"/_/shutdown", nil)
		req.Header.Set("X-Shutdown-Token", st.Token)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
		for range 50 {
			if _, answers := healthPID(st.Port); !answers {
				fmt.Fprintf(out, "Housenka na %s je vypnutá.\n", st.URL)
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// Still there: end the process, but only the one that answers as
	// this server (a reused PID must never be hit).
	if pid, answers := healthPID(st.Port); answers && pid == st.PID {
		return kill(st, out)
	}
	if _, answers := healthPID(st.Port); !answers {
		fmt.Fprintf(out, "Housenka na %s je vypnutá.\n", st.URL)
		return nil
	}
	return fmt.Errorf("server na portu %d se nepodařilo vypnout (PID %d)", st.Port, st.PID)
}

func kill(st state, out io.Writer) error {
	if err := terminate(st.PID); err != nil {
		return fmt.Errorf("proces %d se nepodařilo ukončit: %v", st.PID, err)
	}
	removeState(st.Token)
	fmt.Fprintf(out, "Housenka (PID %d) byla ukončena.\n", st.PID)
	return nil
}

// Status reports whether the server is running. port is where to look for
// a server whose state file is missing.
func Status(port int, out io.Writer) error {
	switch st, status := lookup(port); status {
	case up:
		fmt.Fprintf(out, "Housenka běží na %s (PID %d)\n", st.URL, st.PID)
	case unresponsive:
		fmt.Fprintf(out, "Housenka (PID %d) na portu %d běží, ale neodpovídá – ukončíš ji příkazem housenka web stop --force\n", st.PID, st.Port)
	default:
		fmt.Fprintln(out, "Housenka na pozadí neběží.")
	}
	return nil
}
