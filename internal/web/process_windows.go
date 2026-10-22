//go:build windows

package web

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const stillActive = 259 // STILL_ACTIVE exit code

// processAlive reports whether a process with pid exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}

// terminate ends the process; Windows has no polite signal for a
// detached process.
func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
