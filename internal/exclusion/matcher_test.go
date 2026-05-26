package exclusion_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/exclusion"
)

func TestMatcher_Match(t *testing.T) {
	m := exclusion.NewMatcher([]string{".env", "**/*.log", "**/build/**"})

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact match", ".env", true},
		{"glob star", "debug.log", true},
		{"nested glob", "src/app.log", true},
		{"dir match", "build/output.js", true},
		{"no match", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatcher_HasPatterns(t *testing.T) {
	empty := exclusion.NewMatcher(nil)
	if empty.HasPatterns() {
		t.Error("empty matcher should not have patterns")
	}

	withPatterns := exclusion.NewMatcher([]string{".env"})
	if !withPatterns.HasPatterns() {
		t.Error("matcher with patterns should report HasPatterns=true")
	}
}

func TestBuildMatcher_LoadsLocalIgnore(t *testing.T) {
	dir := t.TempDir()
	ignoreFile := filepath.Join(dir, ".abxignore")
	if err := os.WriteFile(ignoreFile, []byte("**/node_modules/**\n*.secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := exclusion.BuildMatcher(t.Context(), dir)
	if err != nil {
		t.Fatalf("BuildMatcher() error: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"node_modules/react/index.js", true},
		{"keys.secret", true},
		{"main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := m.Match(tt.path)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildMatcher_IncludesHardcoded(t *testing.T) {
	dir := t.TempDir()
	// No .abxignore — should still have hardcoded patterns

	m, err := exclusion.BuildMatcher(t.Context(), dir)
	if err != nil {
		t.Fatalf("BuildMatcher() error: %v", err)
	}

	if !m.Match(".env") {
		t.Error("hardcoded .env pattern should match")
	}
	if !m.Match(".ssh/id_rsa") {
		t.Error("hardcoded .ssh pattern should match")
	}
}
