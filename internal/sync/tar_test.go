package sync_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestTarFiltered_SingleFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "hello.txt"), []byte("world"), 0o644)

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar header: %v", err)
	}

	if header.Name != "hello.txt" {
		t.Errorf("tar entry name = %q, want %q", header.Name, "hello.txt")
	}

	content, _ := io.ReadAll(tr)
	if string(content) != "world" {
		t.Errorf("tar entry content = %q, want %q", string(content), "world")
	}

	// Verify EOF
	_, err = tr.Next()
	if err != io.EOF {
		t.Errorf("expected EOF after single entry, got %v", err)
	}
}

func TestTarFiltered_FileSource(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".aider.conf.yml")
	mustWriteFile(t, filePath, []byte("model: test"), 0o600)

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(filePath, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar header: %v", err)
	}
	if header.Name != ".aider.conf.yml" {
		t.Errorf("tar entry name = %q, want .aider.conf.yml", header.Name)
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "model: test" {
		t.Errorf("tar entry content = %q, want model: test", string(content))
	}
}

func TestTarFiltered_NestedStructure(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "src", "lib"))
	mustWriteFile(t, filepath.Join(dir, "src", "lib", "util.go"), []byte("package lib"), 0o644)
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	entries := map[string]string{}
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

	if entries["main.go"] != "package main" {
		t.Errorf("main.go content = %q, want %q", entries["main.go"], "package main")
	}
	if entries["src/lib/util.go"] != "package lib" {
		t.Errorf("src/lib/util.go content = %q, want %q", entries["src/lib/util.go"], "package lib")
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 file entries, got %d: %v", len(entries), entries)
	}
}

func TestTarFiltered_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	_, err = tr.Next()
	if err != io.EOF {
		t.Errorf("empty dir should produce empty tar, got %v", err)
	}
}

func TestTarFiltered_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "script.sh"), []byte("#!/bin/sh"), 0o755)

	buf := new(bytes.Buffer)
	err := syncpkg.TarFiltered(dir, buf, nil)
	if err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	h, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar: %v", err)
	}

	if h.Mode != 0o755 {
		t.Errorf("permissions = %04o, want 0755", h.Mode)
	}
}

func TestTarFiltered_ArchivesSymlinkWithoutDereferencing(t *testing.T) {
	workspaceDir := t.TempDir()
	secretDir := t.TempDir()
	secretPath := filepath.Join(secretDir, "id_rsa")
	mustWriteFile(t, secretPath, []byte("SECRET"), 0o600)

	linkPath := filepath.Join(workspaceDir, "safe-link")
	if err := os.Symlink(secretPath, linkPath); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	buf := new(bytes.Buffer)
	if err := syncpkg.TarFiltered(workspaceDir, buf, nil); err != nil {
		t.Fatalf("TarFiltered() error: %v", err)
	}

	tr := tar.NewReader(buf)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar header: %v", err)
	}
	if header.Typeflag != tar.TypeSymlink {
		t.Fatalf("tar entry type = %v, want symlink", header.Typeflag)
	}
	if header.Linkname != secretPath {
		t.Fatalf("tar entry link target = %q, want %q", header.Linkname, secretPath)
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("reading tar content: %v", err)
	}
	if string(content) == "SECRET" {
		t.Fatal("symlink target content was archived")
	}
}
