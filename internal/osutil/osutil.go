package osutil

import (
	"fmt"
	"os"
	"os/user"
)

// SystemHomeDir returns the OS account home directory for security decisions.
func SystemHomeDir() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolving current user: %w", err)
	}
	if current.HomeDir == "" {
		return "", fmt.Errorf("current user home directory is empty")
	}
	return current.HomeDir, nil
}

// HomeDir returns the user's home directory, preferring HOME for compatibility.
func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	current, err := user.Current()
	if err != nil {
		return ""
	}
	return current.HomeDir
}
