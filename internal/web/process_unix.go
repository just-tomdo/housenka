//go:build unix

package web

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// processAlive reports whether a process with pid exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// terminate asks the process to end (SIGTERM, so it removes its state
// file) and kills it when it is still there after a few seconds.
func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	for range 30 {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return p.Kill()
}

// processStart identifies when pid started, or "" when it cannot be told.
// With the PID it tells a process apart from a later one with the same PID.
func processStart(pid int) string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return ""
		}
		// The command name may contain spaces: fields start after ")".
		_, rest, ok := strings.Cut(string(data), ") ")
		if f := strings.Fields(rest); ok && len(f) > 19 {
			return f[19] // starttime, field 22 of the whole line
		}
		return ""
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(out)), " ")
}
