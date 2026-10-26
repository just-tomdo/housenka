package web

import (
	"os"
	"path/filepath"
	"strings"
)

// sameProgram reports whether pid runs this program (Housenka), so a PID
// reused by another program after a crash is never taken for the server.
func sameProgram(pid int) bool {
	name := processProgram(pid)
	exe, err := os.Executable()
	if name == "" || err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(name, ".exe"), strings.TrimSuffix(filepath.Base(exe), ".exe"))
}
