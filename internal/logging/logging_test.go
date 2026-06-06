package logging_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/logging"
)

func TestSetup_VerboseCreatesLogFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	if err := logging.Setup(true, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(tmpHome, ".local", "state", "abx", "abx.log")
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

	if err := logging.Setup(false, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(tmpHome, ".local", "state", "abx", "abx.log")
	if _, err := os.Stat(logFile); err == nil {
		t.Error("log file should not be created when verbose=false")
	}
}

func TestSetup_JSONOutput(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	if err := logging.Setup(false, true); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
}

func TestSetup_VerboseAndJSON(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	if err := logging.Setup(true, true); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(tmpHome, ".local", "state", "abx", "abx.log")
	if _, err := os.Stat(logFile); err != nil {
		t.Error("log file should exist when verbose=true even with json=true")
	}
}
