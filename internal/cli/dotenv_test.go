package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestLoadDotEnv_DisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".abxenv"), []byte("OPENAI_API_KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "host-secret")

	got, err := cli.LoadDotEnv(dir, false)
	if err != nil {
		t.Fatalf("LoadDotEnv() error: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadDotEnv() = %v, want nil when trust disabled", got)
	}
}

func TestLoadDotEnv_TrustedAllowsOnlyHostAllowlistValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".abxenv"), []byte("OPENAI_API_KEY=file-secret\nANTHROPIC_API_KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "host-openai")
	t.Setenv("ANTHROPIC_API_KEY", "host-anthropic")

	got, err := cli.LoadDotEnv(dir, true)
	if err != nil {
		t.Fatalf("LoadDotEnv() error: %v", err)
	}
	want := []string{"OPENAI_API_KEY=host-openai", "ANTHROPIC_API_KEY=host-anthropic"}
	assertStringSlicesEqual(t, got, want)
}

func TestLoadDotEnv_BlocksABoxControlKeys(t *testing.T) {
	dir := t.TempDir()
	content := "HOST_UID\nHOST_GID\nSSH_AUTH_SOCK\nABX_SESSION_ID\nABX_WORKSPACE\nPATH\nHOME\nUSER\nSHELL\nPWD\nTERM\nDISPLAY\nOPENAI_API_KEY\n"
	if err := os.WriteFile(filepath.Join(dir, ".abxenv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "host-secret")

	got, err := cli.LoadDotEnv(dir, true)
	if err != nil {
		t.Fatalf("LoadDotEnv() error: %v", err)
	}
	want := []string{"OPENAI_API_KEY=host-secret"}
	assertStringSlicesEqual(t, got, want)
}

func TestLoadDotEnv(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		envSetup map[string]string
		want     []string
	}{
		{
			name:     "bare keys resolved from env",
			content:  "API_KEY\nMY_VAR\n",
			envSetup: map[string]string{"API_KEY": "sk-123", "MY_VAR": "hello"},
			want:     []string{"API_KEY=sk-123", "MY_VAR=hello"},
		},
		{
			name:     "key=value pairs resolve key from env",
			content:  "API_KEY=sk-123\nMY_VAR=hello\n",
			envSetup: map[string]string{"API_KEY": "sk-123", "MY_VAR": "hello"},
			want:     []string{"API_KEY=sk-123", "MY_VAR=hello"},
		},
		{
			name:     "missing env vars skipped",
			content:  "PRESENT_KEY\nMISSING_KEY\n",
			envSetup: map[string]string{"PRESENT_KEY": "val"},
			want:     []string{"PRESENT_KEY=val"},
		},
		{
			name:     "comments and blanks skipped",
			content:  "# comment\n\nAPI_KEY\n# another\n",
			envSetup: map[string]string{"API_KEY": "x"},
			want:     []string{"API_KEY=x"},
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
		{
			name:     "blocked keys skipped",
			content:  "PATH\nHOME\nAPI_KEY\n",
			envSetup: map[string]string{"API_KEY": "x"},
			want:     []string{"API_KEY=x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up env vars
			for k, v := range tt.envSetup {
				t.Setenv(k, v)
			}

			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dir, ".abxenv"), []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := cli.LoadDotEnv(dir, true)
			if err != nil {
				t.Fatalf("LoadDotEnv() error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, k := range got {
				if k != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, k, tt.want[i])
				}
			}
		})
	}
}

func TestLoadDotEnv_NoFile(t *testing.T) {
	dir := t.TempDir()
	got, err := cli.LoadDotEnv(dir)
	if err != nil {
		t.Fatalf("LoadDotEnv() error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing .abxenv, got %v", got)
	}
}

func TestLoadDotEnv_ReturnsReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".abxenv"), 0o755); err != nil {
		t.Fatalf("creating .abxenv directory fixture: %v", err)
	}

	_, err := cli.LoadDotEnv(dir, true)
	if err == nil {
		t.Fatal("expected read error")
	}
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q (full got %v)", i, got[i], want[i], got)
		}
	}
}
