//go:build unix

package web

import (
	"errors"
	"os"
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
