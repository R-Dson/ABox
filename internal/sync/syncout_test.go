package sync_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncer_SyncOut_ExtractsTarToHost(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "modified.go", Mode: 0o644, Size: int64(len("package main"))}, []byte("package main"))
	tw.Close()

	mock := &syncOutCapturingRuntime{
		onCopyFromContainer: func(_ string, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	s := syncpkg.NewSyncer(mock)
	err := s.SyncOut(t.Context(), "test-vol", "/workspace", destDir)
	if err != nil {
		t.Fatalf("SyncOut() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "modified.go"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "package main" {
		t.Errorf("extracted content = %q, want %q", string(data), "package main")
	}
}

func TestSyncer_SyncOut_SkipsIfDestMissing(t *testing.T) {
	mock := &syncOutCapturingRuntime{}
	s := syncpkg.NewSyncer(mock)

	err := s.SyncOut(t.Context(), "test-vol", "/workspace", "/nonexistent/dest")
	if err != nil {
		t.Fatalf("SyncOut with missing dest should not error: %v", err)
	}
}

func TestSyncer_SyncOut_ExtractsNestedFiles(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "src/lib/util.rs", Mode: 0o644, Size: 5}, []byte("fn x "))
	mustTarWrite(t, tw, &tar.Header{Name: "main.rs", Mode: 0o644, Size: 4}, []byte("fn m"))
	tw.Close()

	mock := &syncOutCapturingRuntime{
		onCopyFromContainer: func(_ string, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	s := syncpkg.NewSyncer(mock)
	err := s.SyncOut(t.Context(), "vol", "/workspace", destDir)
	if err != nil {
		t.Fatalf("SyncOut() error: %v", err)
	}

	utilData, _ := os.ReadFile(filepath.Join(destDir, "src/lib/util.rs"))
	if string(utilData) != "fn x " {
		t.Errorf("src/lib/util.rs = %q, want %q", string(utilData), "fn x ")
	}

	mainData, _ := os.ReadFile(filepath.Join(destDir, "main.rs"))
	if string(mainData) != "fn m" {
		t.Errorf("main.rs = %q, want %q", string(mainData), "fn m")
	}
}

// syncOutCapturingRuntime captures CopyFromContainer calls.
type syncOutCapturingRuntime struct {
	syncStubRuntime
	onCopyFromContainer func(containerID string, src string) (io.ReadCloser, error)
}

func (s *syncOutCapturingRuntime) CopyFromContainer(_ context.Context, _ string, src string) (io.ReadCloser, error) {
	if s.onCopyFromContainer != nil {
		return s.onCopyFromContainer("", src)
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func mustTarWrite(t *testing.T, tw *tar.Writer, h *tar.Header, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("tar write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write data: %v", err)
	}
}
