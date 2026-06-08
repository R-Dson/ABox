package logging_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/logging"
)

func TestSetup_VerboseCreatesLogFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := logging.Setup(true, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(home, ".local", "state", "abx", "abx.log")
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("verbose log file not created at %s: %v", logFile, err)
	}
	if info.Mode().Perm()&0o777 > 0o600 {
		t.Errorf("log file permissions %o, want 0600 or stricter", info.Mode().Perm())
	}
}

func TestSetup_NonVerboseNoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := logging.Setup(false, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(home, ".local", "state", "abx", "abx.log")
	if _, err := os.Stat(logFile); err == nil {
		t.Error("log file should not be created when verbose=false")
	}
}

func TestLogging_JSONOnlyWhenFlagTrue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "stderr.log")
	stderr, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderr
	defer func() { os.Stderr = oldStderr }()

	if err := logging.Setup(false, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
	slog.Info("hello")
	if err := stderr.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		t.Fatalf("stderr log is JSON without json flag: %q", data)
	}
}

func TestSetup_JSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := logging.Setup(false, true); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
}

func TestSetup_VerboseAndJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := logging.Setup(true, true); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	logFile := filepath.Join(home, ".local", "state", "abx", "abx.log")
	if _, err := os.Stat(logFile); err != nil {
		t.Error("log file should exist when verbose=true even with json=true")
	}
}
