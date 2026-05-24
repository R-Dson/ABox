package sync_test

import (
	"archive/tar"
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/exclusion"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestTarFiltered_ExcludesSecretFiles(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	mustWriteFile(t, filepath.Join(dir, ".env"), []byte("SECRET=abc"), 0o644)
	mustWriteFile(t, filepath.Join(dir, "keys.pem"), []byte("cert data"), 0o644)

	matcher := exclusion.NewMatcher([]string{".env", "*.pem"})

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, matcher)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	entries := readTarEntries(t, buf)
	if _, exists := entries["main.go"]; !exists {
		t.Error("main.go should be included")
	}
	if _, exists := entries[".env"]; exists {
		t.Error(".env should be excluded")
	}
	if _, exists := entries["keys.pem"]; exists {
		t.Error("keys.pem should be excluded")
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(entries), entries)
	}
}

func TestTarFiltered_ExcludesNestedSecrets(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "src"))
	mustMkdirAll(t, filepath.Join(dir, ".ssh"))
	mustWriteFile(t, filepath.Join(dir, "src", "app.go"), []byte("package app"), 0o644)
	mustWriteFile(t, filepath.Join(dir, ".ssh", "id_rsa"), []byte("private key"), 0o600)
	mustWriteFile(t, filepath.Join(dir, "config.yaml"), []byte("app: test"), 0o644)

	// Hardcoded patterns include .ssh — test that
	matcher := exclusion.NewMatcher(exclusion.HardcodedPatterns())

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, matcher)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	entries := readTarEntries(t, buf)
	if _, exists := entries["src/app.go"]; !exists {
		t.Error("src/app.go should be included")
	}
	if _, exists := entries["config.yaml"]; !exists {
		t.Error("config.yaml should be included")
	}
	if _, exists := entries[".ssh/id_rsa"]; exists {
		t.Error(".ssh/id_rsa should be excluded by hardcoded patterns")
	}
}

func TestTarFiltered_SkipsExcludedDirectories(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "node_modules", "react"))
	mustWriteFile(t, filepath.Join(dir, "node_modules", "react", "index.js"), []byte("react"), 0o644)
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	matcher := exclusion.NewMatcher([]string{"node_modules/"})

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, matcher)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	entries := readTarEntries(t, buf)
	if _, exists := entries["main.go"]; !exists {
		t.Error("main.go should be included")
	}
	if _, exists := entries["node_modules/react/index.js"]; exists {
		t.Error("node_modules content should be excluded")
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(entries), entries)
	}
}

func TestTarFiltered_NilMatcherIncludesAll(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	mustWriteFile(t, filepath.Join(dir, ".env"), []byte("secret"), 0o644)

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	entries := readTarEntries(t, buf)
	if len(entries) != 2 {
		t.Errorf("nil matcher should include all files, got %d: %v", len(entries), entries)
	}
}

func TestTarFiltered_EmptyDirProducesEmptyTar(t *testing.T) {
	dir := t.TempDir()

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, exclusion.NewMatcher(nil))
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	_, err = tr.Next()
	if err != io.EOF {
		t.Errorf("empty dir should produce empty tar, got %v", err)
	}
}

// readTarEntries reads all file entries from a tar buffer into a name→content map.
func readTarEntries(t *testing.T, buf *bytes.Buffer) map[string]string {
	t.Helper()
	entries := map[string]string{}
	tr := tar.NewReader(buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		content, _ := io.ReadAll(tr)
		entries[h.Name] = string(content)
	}
	return entries
}
