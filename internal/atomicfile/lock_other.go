//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly) && !windows

package atomicfile

import "os"

// Platforms without flock (Solaris, AIX …) rely on the in-process mutex
// and the version checks of the callers only.
func lockFile(*os.File) error { return nil }

func tryLockFile(*os.File) (bool, error) { return false, nil }

func unlockFile(*os.File) {}
