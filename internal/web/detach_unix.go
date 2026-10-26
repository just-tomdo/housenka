//go:build unix

package web

import (
	"os/exec"
	"syscall"
)

// detach runs cmd in its own session, so it survives closing the terminal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
