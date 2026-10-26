// Package atomicfile replaces files so that readers never see a half-written
// version, and tells whether a file changed since it was last seen. Files
// are shared by several processes (the TUI and the web interface).
package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Write replaces the file at path with data: it writes a temporary file
// next to it and renames it over the original. A symlink is followed, so
// the link stays and its target is replaced, and the permissions of an
// existing file are kept (perm applies to a new file).
func Write(path string, data []byte, perm os.FileMode) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	} else if link, err := os.Readlink(path); err == nil {
		// A symlink to a file that does not exist yet: create the target.
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		path = link
	}
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*"+filepath.Ext(path))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return rename(tmp.Name(), path)
}

// rename retries on Windows, where replacing a file fails while another
// process has it open for reading.
func rename(from, to string) error {
	err := os.Rename(from, to)
	if runtime.GOOS != "windows" {
		return err
	}
	for i := 0; err != nil && i < 20; i++ {
		time.Sleep(time.Duration(10+10*i) * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}

// Stamp identifies a version of a file. The zero value means "missing".
type Stamp struct {
	modTime time.Time
	size    int64
	id      uint64 // inode where available: Write creates a new file every time
}

// StatStamp returns the stamp of the file at path.
func StatStamp(path string) (Stamp, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Stamp{}, nil
	}
	if err != nil {
		return Stamp{}, err
	}
	return Stamp{fi.ModTime(), fi.Size(), fileID(fi)}, nil
}
