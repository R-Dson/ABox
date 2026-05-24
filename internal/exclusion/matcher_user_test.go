package exclusion_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/exclusion"
)

func TestMatcher_UserFacingPatterns(t *testing.T) {
	// These are patterns a user would actually write in .abxignore
	m := exclusion.NewMatcher([]string{".env", "*.log", "build/", "node_modules/"})

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact file", ".env", true},
		{"glob extension", "debug.log", true},
		{"nested glob extension", "src/app.log", true},
		{"trailing slash dir", "build/output.js", true},
		{"trailing slash nested", "build/js/bundle.min.js", true},
		{"node_modules dir", "node_modules/react/index.js", true},
		{"no match source", "main.go", false},
		{"no match src dir", "src/lib/util.rs", false},
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

func TestBuildMatcher_LoadsLocalIgnore_UserPatterns(t *testing.T) {
	dir := t.TempDir()
	ignoreFile := filepath.Join(dir, ".abxignore")
	// User writes natural patterns
	if err := os.WriteFile(ignoreFile, []byte("node_modules/\n*.secret\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := exclusion.BuildMatcher(t.Context(), dir, "")
	if err != nil {
		t.Fatalf("BuildMatcher() error: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"node_modules/react/index.js", true},
		{"keys.secret", true},
		{"build/output.js", true},
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
