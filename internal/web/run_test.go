package web

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/just-tomdo/housenka/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting: %s", what)
}

func TestServeStatusStop(t *testing.T) {
	defer func(d time.Duration) { stateCheckEvery = d }(stateCheckEvery)
	stateCheckEvery = 50 * time.Millisecond
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	cfgPath := filepath.Join(home, "config.yaml")
	os.WriteFile(cfgPath, []byte(testYAML), 0o644)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)

	var out bytes.Buffer
	done := make(chan error, 1)
	opened := make(chan string, 1)
	go func() {
		done <- Serve(Options{Config: cfg, ConfigPath: cfgPath, Port: port, Open: func(u string) error { opened <- u; return nil }}, &out)
	}()
	var st state
	for i := 0; ; i++ {
		var ok bool
		if st, ok = running(); ok {
			break
		}
		if i > 100 {
			t.Fatalf("server did not start: %s", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st.Port != port || !strings.HasPrefix(st.URL, "http://housenka.localhost:") {
		t.Errorf("state = %+v", st)
	}
	if u := <-opened; u != st.URL {
		t.Errorf("opened %q", u)
	}

	var status bytes.Buffer
	Status(port, &status)
	if !strings.Contains(status.String(), "běží na "+st.URL) || !strings.Contains(status.String(), fmt.Sprintf("PID %d", os.Getpid())) {
		t.Errorf("status = %q", status.String())
	}

	// A deleted state file: status still finds the server by its port, and
	// the server writes the file again.
	os.Remove(filepath.Join(home, "web.json"))
	status.Reset()
	Status(port, &status)
	if !strings.Contains(status.String(), fmt.Sprintf("běží na http://localhost:%d/ (PID %d)", port, os.Getpid())) {
		t.Errorf("status without state file = %q", status.String())
	}
	waitFor(t, "state file rewritten", func() bool { _, ok := readState(); return ok })
	if err := Serve(Options{Config: cfg, ConfigPath: cfgPath, Port: port}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "už běží") {
		t.Errorf("second server: %v", err)
	}

	var stop bytes.Buffer
	if err := Stop(port, false, &stop); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
	if _, err := os.Stat(filepath.Join(home, "web.json")); !os.IsNotExist(err) {
		t.Errorf("state file left behind: %v", err)
	}
	status.Reset()
	Status(port, &status)
	if !strings.Contains(status.String(), "neběží") {
		t.Errorf("status after stop = %q", status.String())
	}
}

func TestStaleState(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	writeState(state{PID: 1, Port: freePort(t), URL: "http://x/", Token: "t"})
	var out bytes.Buffer
	if err := Stop(0, false, &out); err != nil || !strings.Contains(out.String(), "neběží") {
		t.Errorf("stop with stale state: %v %q", err, out.String())
	}
	if _, ok := readState(); ok {
		t.Error("stale state not removed")
	}
}

// helperEnv makes the test binary act as a misbehaving server (see TestMain).
const helperEnv = "HOUSENKA_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "deaf":
		// Answers health like Housenka but ignores the shutdown request.
		mux := http.NewServeMux()
		mux.HandleFunc("/_/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "housenka %d\n", os.Getpid()) })
		mux.HandleFunc("/_/shutdown", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
		http.ListenAndServe("127.0.0.1:"+os.Getenv("HELPER_PORT"), mux)
		os.Exit(0)
	case "hung":
		// Holds the port but never answers.
		ln, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("HELPER_PORT"))
		if err != nil {
			os.Exit(1)
		}
		for {
			c, err := ln.Accept()
			if err != nil {
				os.Exit(1)
			}
			defer c.Close()
		}
	}
	os.Exit(m.Run())
}

// startHelper runs the test binary as a misbehaving server and writes its
// state file as the real server would.
func startHelper(t *testing.T, mode string) (*exec.Cmd, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper processes are tested on Unix")
	}
	port := freePort(t)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, fmt.Sprintf("HELPER_PORT=%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }()
	t.Cleanup(func() { cmd.Process.Kill(); <-exited })
	waitFor(t, "helper listening", func() bool { return !portFree(port) })
	writeState(state{PID: cmd.Process.Pid, Port: port, URL: fmt.Sprintf("http://localhost:%d/", port), Token: "secret"})
	return cmd, port
}

func TestStopEndsServerIgnoringShutdown(t *testing.T) {
	t.Setenv(config.HomeEnv, t.TempDir())
	cmd, port := startHelper(t, "deaf")
	var out bytes.Buffer
	if err := Stop(port, false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), fmt.Sprintf("Housenka (PID %d) byla ukončena", cmd.Process.Pid)) {
		t.Errorf("out = %q", out.String())
	}
	waitFor(t, "process gone", func() bool { return portFree(port) })
	if _, ok := readState(); ok {
		t.Error("state file left behind")
	}
}

func TestUnresponsiveServer(t *testing.T) {
	t.Setenv(config.HomeEnv, t.TempDir())
	defer func(c *http.Client) { client = c }(client)
	client = &http.Client{Timeout: 200 * time.Millisecond}
	cmd, port := startHelper(t, "hung")

	var out bytes.Buffer
	Status(port, &out)
	if !strings.Contains(out.String(), "neodpovídá") {
		t.Errorf("status = %q", out.String())
	}
	if _, ok := readState(); !ok {
		t.Fatal("state of a living but silent server was removed")
	}
	if err := Stop(port, false, &out); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("stop without force: %v", err)
	}
	out.Reset()
	if err := Stop(port, true, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), fmt.Sprintf("PID %d", cmd.Process.Pid)) {
		t.Errorf("out = %q", out.String())
	}
	waitFor(t, "process gone", func() bool { return portFree(port) })
}

func TestServerOfAnotherHomeIsNotOurs(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	cfgPath := filepath.Join(home, "config.yaml")
	os.WriteFile(cfgPath, []byte(testYAML), 0o644)
	cfg, _ := config.Load(cfgPath)
	port := freePort(t)
	done := make(chan error, 1)
	go func() { done <- Serve(Options{Config: cfg, ConfigPath: cfgPath, Port: port}, &bytes.Buffer{}) }()
	waitFor(t, "server up", func() bool { _, ok := running(); return ok })

	// Another home (a demo next to the real data) on the same port.
	t.Setenv(config.HomeEnv, t.TempDir())
	var out bytes.Buffer
	Status(port, &out)
	if !strings.Contains(out.String(), "neběží") {
		t.Errorf("status of another home = %q", out.String())
	}
	out.Reset()
	if err := Stop(port, true, &out); err != nil || !strings.Contains(out.String(), "neběží") {
		t.Errorf("stop of another home: %v %q", err, out.String())
	}

	t.Setenv(config.HomeEnv, home)
	if !answers(port) {
		t.Fatal("the other home stopped our server")
	}
	if err := Stop(port, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	<-done
}

// startSleeper runs an unrelated program whose PID a stale state file may
// point to after a crash.
func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep not available:", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

func TestReusedPIDIsNeverHit(t *testing.T) {
	for _, mode := range []string{"hung", "deaf"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(config.HomeEnv, t.TempDir())
			defer func(c *http.Client) { client = c }(client)
			client = &http.Client{Timeout: 200 * time.Millisecond}
			_, port := startHelper(t, mode)
			sleeper := startSleeper(t)
			// The state file is stale: its PID now belongs to sleep, and the
			// port is held by another server.
			writeState(state{PID: sleeper.Process.Pid, Port: port, URL: "http://localhost/", Token: "old"})

			var out bytes.Buffer
			if err := Stop(port, true, &out); err != nil || !strings.Contains(out.String(), "neběží") {
				t.Errorf("stop: %v %q", err, out.String())
			}
			if !processAlive(sleeper.Process.Pid) {
				t.Error("an unrelated process was killed")
			}
			if _, ok := readState(); ok {
				t.Error("stale state file kept")
			}
		})
	}
}
