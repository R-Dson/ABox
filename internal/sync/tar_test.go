package sync_test

import (
	"archive/tar"
	"bytes"
	"io"
	"path/filepath"
	"testing"

	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestTarDir_SingleFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "hello.txt"), []byte("world"), 0o644)

	buf := new(bytes.Buffer)
	err := syncpkg.TarDir(dir, buf)
	if err != nil {
		t.Fatalf("TarDir() error: %v", err)
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

func TestTarDir_NestedStructure(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "src", "lib"))
	mustWriteFile(t, filepath.Join(dir, "src", "lib", "util.go"), []byte("package lib"), 0o644)
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	buf := new(bytes.Buffer)
	err := syncpkg.TarDir(dir, buf)
	if err != nil {
		t.Fatalf("TarDir() error: %v", err)
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

func TestTarDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	buf := new(bytes.Buffer)
	err := syncpkg.TarDir(dir, buf)
	if err != nil {
		t.Fatalf("TarDir() error: %v", err)
	}

	tr := tar.NewReader(buf)
	_, err = tr.Next()
	if err != io.EOF {
		t.Errorf("empty dir should produce empty tar, got %v", err)
	}
}

func TestTarDir_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "script.sh"), []byte("#!/bin/sh"), 0o755)

	buf := new(bytes.Buffer)
	err := syncpkg.TarDir(dir, buf)
	if err != nil {
		t.Fatalf("TarDir() error: %v", err)
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
