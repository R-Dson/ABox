package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestLoadDotEnv(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "bare keys",
			content: "API_KEY\nMY_VAR\n",
			want:    []string{"API_KEY", "MY_VAR"},
		},
		{
			name:    "key=value pairs",
			content: "API_KEY=sk-123\nMY_VAR=hello\n",
			want:    []string{"API_KEY", "MY_VAR"},
		},
		{
			name:    "mixed formats",
			content: "API_KEY\nMY_VAR=hello\n",
			want:    []string{"API_KEY", "MY_VAR"},
		},
		{
			name:    "comments and blanks skipped",
			content: "# comment\n\nAPI_KEY\n# another\n",
			want:    []string{"API_KEY"},
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
