package config

import (
	"os"
	"path/filepath"
)

// HomeEnv overrides the Housenka home directory (useful for tests or a second profile).
const HomeEnv = "HOUSENKA_HOME"

// HomeDir returns the directory holding everything Housenka stores:
// $HOUSENKA_HOME if set, otherwise ~/.housenka on every platform
// (on Windows %USERPROFILE%\.housenka).
func HomeDir() (string, error) {
	if dir := os.Getenv(HomeEnv); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".housenka"), nil
}

// DefaultPath returns the default config file location (~/.housenka/config.yaml).
func DefaultPath() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DataDir returns the directory for stored entries (~/.housenka/entries).
func DataDir() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "entries"), nil
}
