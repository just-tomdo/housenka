//go:build !unix

package atomicfile

import "os"

// fileID is not available; NTFS keeps modification times in 100 ns steps.
func fileID(os.FileInfo) uint64 { return 0 }
