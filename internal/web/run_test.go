package web

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
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

func TestServeStatusStop(t *testing.T) {
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
	Status(&status)
	if !strings.Contains(status.String(), "běží na "+st.URL) {
		t.Errorf("status = %q", status.String())
	}
	if err := Serve(Options{Config: cfg, ConfigPath: cfgPath, Port: port}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "už běží") {
		t.Errorf("second server: %v", err)
	}

	var stop bytes.Buffer
	if err := Stop(&stop); err != nil {
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
	Status(&status)
	if !strings.Contains(status.String(), "neběží") {
		t.Errorf("status after stop = %q", status.String())
	}
}

func TestStaleState(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	writeState(state{PID: 1, Port: freePort(t), URL: "http://x/", Token: "t"})
	var out bytes.Buffer
	if err := Stop(&out); err != nil || !strings.Contains(out.String(), "neběží") {
		t.Errorf("stop with stale state: %v %q", err, out.String())
	}
	if _, ok := readState(); ok {
		t.Error("stale state not removed")
	}
}
