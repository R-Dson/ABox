package osutil

import (
	"os"
	"os/user"
)

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
