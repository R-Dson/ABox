package cli

import (
	"log/slog"
	"os"
	"path/filepath"
)

func setupLogging(verbose, jsonOutput bool) {
	// Placeholder — will be replaced by internal/logging in Task 1.6
	// For now, just set a default slog logger so the CLI compiles
	slog.SetDefault(slog.Default())

	if verbose {
		logDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "abx")
		_ = os.MkdirAll(logDir, 0o700)
	}
}
