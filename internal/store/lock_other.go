//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly) && !windows

package store

import "os"

// Platforms without flock (Solaris, AIX …) rely on the in-process mutex
// and the entry version check only.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
