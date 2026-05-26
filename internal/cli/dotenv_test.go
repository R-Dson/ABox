package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

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

			got := cli.LoadDotEnv(dir)
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
	got := cli.LoadDotEnv(dir)
	if got != nil {
		t.Errorf("expected nil for missing .abxenv, got %v", got)
	}
}
