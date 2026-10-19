//go:build !unix && !windows

package store

import "os"

// Platforms without file locking rely on the in-process mutex only.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
