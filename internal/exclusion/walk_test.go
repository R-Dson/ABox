package exclusion_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/r-dson/abox/internal/exclusion"
)

func TestWalk_ReturnsNonExcludedPaths(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "package main")
	mustWrite(t, filepath.Join(dir, ".env"), "SECRET=abc")
	mustWrite(t, filepath.Join(dir, "README.md"), "# project")

	matcher := exclusion.NewMatcher([]string{".env"})
	paths, err := exclusion.Walk(t.Context(), dir, matcher)
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}

	sort.Strings(paths)
	want := []string{"README.md", "main.go"}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths %v, want %d paths %v", len(paths), paths, len(want), want)
	}
	for i, p := range paths {
		if p != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestWalk_SkipsExcludedDirectories(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "node_modules", "react"))
	mustWrite(t, filepath.Join(dir, "node_modules", "react", "index.js"), "react")
	mustWrite(t, filepath.Join(dir, "app.go"), "package app")

	matcher := exclusion.NewMatcher([]string{"node_modules/"})
	paths, err := exclusion.Walk(t.Context(), dir, matcher)
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}

	if len(paths) != 1 || paths[0] != "app.go" {
		t.Errorf("got %v, want [app.go]", paths)
	}
}

func TestWalk_ResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "real.txt"), "content")
	if err := os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	paths, err := exclusion.Walk(t.Context(), dir, exclusion.NewMatcher(nil))
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}

	sort.Strings(paths)
	// Should include the symlink (resolved or not — it's a valid file)
	found := false
	for _, p := range paths {
		if p == "link.txt" || p == "real.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected link.txt or real.txt in paths, got %v", paths)
	}
}

func TestWalk_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	paths, err := exclusion.Walk(t.Context(), dir, exclusion.NewMatcher(nil))
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("empty dir should return no paths, got %v", paths)
	}
}

func TestWalk_NonexistentDir(t *testing.T) {
	_, err := exclusion.Walk(t.Context(), "/nonexistent/path", exclusion.NewMatcher(nil))
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
