//go:build !unix && !windows

package web

import "os/exec"

func detach(*exec.Cmd) {}
