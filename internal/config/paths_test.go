package config

import (
	"path/filepath"
	"testing"
)

func TestPathsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows
	t.Setenv(HomeEnv, "")

	cfg, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".housenka", "config.yaml"); cfg != want {
		t.Errorf("DefaultPath = %q, want %q", cfg, want)
	}
	data, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".housenka", "entries"); data != want {
		t.Errorf("DataDir = %q, want %q", data, want)
	}
}

func TestPathsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnv, dir)
	cfg, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "config.yaml"); cfg != want {
		t.Errorf("DefaultPath = %q, want %q", cfg, want)
	}
}
