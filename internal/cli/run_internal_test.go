package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/config"
)

func TestValidateSessionConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *SessionConfig
		wantErr string
	}{
		{name: "valid empty defaults", cfg: &SessionConfig{}},
		{name: "no internet rejects exclude URL", cfg: &SessionConfig{NoInternet: true, ExcludeURL: "https://example.com"}, wantErr: "exclude-url"},
		{name: "invalid memory limit", cfg: &SessionConfig{MemoryLimit: "nope"}, wantErr: "memory limit"},
		{name: "negative CPU limit", cfg: &SessionConfig{CPULimit: -1}, wantErr: "cpu limit"},
		{name: "invalid pull policy", cfg: &SessionConfig{PullPolicy: "sometimes"}, wantErr: "pull policy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSessionConfig(tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSessionConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateSessionConfig() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestEnsureEditorDataDirsCreatesDirectoryBackedPaths(t *testing.T) {
	home := t.TempDir()
	profile := config.EditorProfile{CmdName: "opencode", ConfigPath: ".config/opencode"}

	if err := ensureEditorDataDirs(profile, home); err != nil {
		t.Fatalf("ensureEditorDataDirs() error = %v", err)
	}

	for _, dir := range []string{
		filepath.Join(home, ".config", "opencode"),
		filepath.Join(home, ".cache", "opencode"),
		filepath.Join(home, ".local", "state", "opencode"),
		filepath.Join(home, ".local", "share", "opencode"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
}

func TestEnsureEditorDataDirsDoesNotCreateFileBackedConfig(t *testing.T) {
	home := t.TempDir()
	profile := config.EditorProfile{CmdName: "aider", ConfigPath: ".aider.conf.yml", ConfigIsFile: true}

	if err := ensureEditorDataDirs(profile, home); err != nil {
		t.Fatalf("ensureEditorDataDirs() error = %v", err)
	}

	configPath := filepath.Join(home, ".aider.conf.yml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("file-backed config path should not be created, stat error = %v", err)
	}
}

func TestShouldAllocateTTY(t *testing.T) {
	tests := []struct {
		name             string
		hasTerminalInput bool
		shell            bool
		forceInteractive bool
		want             bool
	}{
		{name: "terminal input", hasTerminalInput: true, want: true},
		{name: "shell forces tty", shell: true, want: true},
		{name: "force-it forces tty", forceInteractive: true, want: true},
		{name: "non-interactive", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldAllocateTTY(tc.hasTerminalInput, tc.shell, tc.forceInteractive)
			if got != tc.want {
				t.Errorf("shouldAllocateTTY() = %v, want %v", got, tc.want)
			}
		})
	}
}
