package cli

import (
	"strings"
	"testing"
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
