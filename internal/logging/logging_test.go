package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetup_DefaultLogPath(t *testing.T) {
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", os.Getenv("HOMEOLD"))

	// Verbose should create the log file
	Setup(true, false)

	logDir := filepath.Join(tmpHome, ".local", "state", "abx")
	logFile := filepath.Join(logDir, "abx.log")

	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("verbose log file not created at %s: %v", logFile, err)
	}
	if info.Mode().Perm()&0o777 > 0o600 {
		t.Errorf("log file permissions %o, want 0600 or stricter", info.Mode().Perm())
	}
}

func TestSetup_NonVerboseNoFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	Setup(false, false)

	logFile := filepath.Join(tmpHome, ".local", "state", "abx", "abx.log")
	if _, err := os.Stat(logFile); err == nil {
		t.Error("log file should not be created when verbose=false")
	}
}
