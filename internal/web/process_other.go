//go:build !unix && !windows

package web

import "os"

// processAlive cannot tell; a server that does not answer counts as gone.
func processAlive(int) bool { return false }

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
