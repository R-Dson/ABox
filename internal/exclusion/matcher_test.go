package exclusion_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestBuildMatcher_InvalidPatternFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".abxignore"), []byte("[bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := exclusion.BuildMatcher(t.Context(), dir)
	if err == nil || !strings.Contains(err.Error(), "invalid exclusion pattern") {
		t.Fatalf("BuildMatcher() error = %v, want invalid exclusion pattern", err)
	}
}

func TestBuildMatcher_ReturnsLocalIgnoreReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".abxignore"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := exclusion.BuildMatcher(t.Context(), dir)
	if err == nil {
		t.Fatal("expected error when .abxignore cannot be read as a file")
	}
}

func TestBuildMatcherWithRemote_RejectsHTTPURL(t *testing.T) {
	_, err := exclusion.BuildMatcherWithRemote(t.Context(), t.TempDir(), "http://example.com/.abxignore")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("BuildMatcherWithRemote() error = %v, want HTTPS rejection", err)
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

func TestBuildMatcherWithRemote_RejectsOversizedRemoteIgnore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("pattern\n", 200_000)))
	}))
	defer server.Close()

	_, err := exclusion.BuildMatcherWithRemote(t.Context(), t.TempDir(), server.URL)
	if err == nil {
		t.Fatal("expected oversized remote ignore error")
	}
}
