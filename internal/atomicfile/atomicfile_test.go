package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteKeepsSymlinkAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yaml")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("old"), 0o600)
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a file")
	}
	data, _ := os.ReadFile(target)
	fi, _ := os.Stat(target)
	if string(data) != "new" || fi.Mode().Perm() != 0o600 {
		t.Errorf("target = %q, mode %v", data, fi.Mode().Perm())
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}
}

func TestStampSeesSameSizeRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026.json")
	if st, _ := StatStamp(path); st != (Stamp{}) {
		t.Error("missing file has a stamp")
	}
	Write(path, []byte("aaaa"), 0o644)
	before, _ := StatStamp(path)
	Write(path, []byte("bbbb"), 0o644)
	after, _ := StatStamp(path)
	if runtime.GOOS != "windows" && before == after {
		t.Error("same-size rewrite not detected")
	}
}

func TestWriteCreatesTargetOfDanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(filepath.Join("dotfiles", "config.yaml"), link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a file")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "dotfiles", "config.yaml")); string(data) != "new" {
		t.Errorf("target = %q", data)
	}
}
