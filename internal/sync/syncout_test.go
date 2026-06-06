package sync_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/runtimetest"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncOut_ExtractsTarToHost(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "modified.go", Mode: 0o644, Size: int64(len("package main"))}, []byte("package main"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
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

func TestSyncOut_CopiesDirectoryContents(t *testing.T) {
	destDir := t.TempDir()
	var gotSrc string
	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, src string) (io.ReadCloser, error) {
			gotSrc = src
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			if err := tw.Close(); err != nil {
				return nil, fmt.Errorf("closing tar writer: %w", err)
			}
			return io.NopCloser(&tarBuf), nil
		},
	}

	if err := syncpkg.Out(t.Context(), stub, "test-vol", "/data", destDir); err != nil {
		t.Fatalf("SyncOut() error: %v", err)
	}
	if gotSrc != "/data/." {
		t.Fatalf("CopyFromContainer src = %q, want /data/.", gotSrc)
	}
}

func TestSyncOut_ExtractsTarToHostFile(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	mustWriteFile(t, destFile, []byte("old"), 0o600)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".aider.conf.yml", Mode: 0o600, Size: int64(len("new"))}, []byte("new"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/data", destFile)
	if err != nil {
		t.Fatalf("SyncOut() error: %v", err)
	}

	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("extracted content = %q, want new", string(data))
	}
}

func TestSyncOutFile_CreatesMissingHostFile(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".aider.conf.yml", Mode: 0o600, Size: int64(len("created"))}, []byte("created"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile)
	if err != nil {
		t.Fatalf("OutFile() error: %v", err)
	}

	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "created" {
		t.Errorf("extracted content = %q, want created", string(data))
	}
}

func TestSyncOutFile_CopiesExpectedFilePath(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	var gotSrc string
	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, src string) (io.ReadCloser, error) {
			gotSrc = src
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			mustTarWrite(t, tw, &tar.Header{Name: ".aider.conf.yml", Mode: 0o600, Size: int64(len("new"))}, []byte("new"))
			if err := tw.Close(); err != nil {
				return nil, fmt.Errorf("closing tar writer: %w", err)
			}
			return io.NopCloser(&tarBuf), nil
		},
	}

	if err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile); err != nil {
		t.Fatalf("OutFile() error: %v", err)
	}
	if gotSrc != "/data/.aider.conf.yml" {
		t.Fatalf("CopyFromContainer src = %q, want /data/.aider.conf.yml", gotSrc)
	}
}

func TestSyncOutFile_RejectsUnexpectedArchiveEntry(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	mustWriteFile(t, destFile, []byte("old"), 0o600)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "wrong.conf", Mode: 0o600, Size: int64(len("wrong"))}, []byte("wrong"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile)
	if err == nil {
		t.Fatal("expected unexpected archive entry error")
	}
	data, readErr := os.ReadFile(destFile)
	if readErr != nil {
		t.Fatalf("reading destination file: %v", readErr)
	}
	if string(data) != "old" {
		t.Fatalf("destination content = %q, want old", string(data))
	}
}

func TestSyncOutFile_ErrorsWhenArchiveHasNoFile(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile)
	if err == nil {
		t.Fatal("expected error for archive with no regular file")
	}
}

func TestSyncOutFile_DoesNotCreateFileOnCopyFailure(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return nil, errors.New("copy failed")
		},
	}

	err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile)
	if err == nil {
		t.Fatal("expected copy error")
	}
	if _, statErr := os.Stat(destFile); !os.IsNotExist(statErr) {
		t.Fatalf("destination file should not exist after copy failure, stat error = %v", statErr)
	}
}

func TestSyncOutFile_RejectsHostSymlinkOverwrite(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), ".aider.conf.yml")
	outsideFile := filepath.Join(t.TempDir(), "outside.conf")
	mustWriteFile(t, outsideFile, []byte("original"), 0o600)
	if err := os.Symlink(outsideFile, destFile); err != nil {
		t.Fatalf("creating host symlink: %v", err)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".aider.conf.yml", Mode: 0o600, Size: int64(len("changed"))}, []byte("changed"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.OutFile(t.Context(), stub, "test-vol", "/data", destFile)
	if err == nil {
		t.Fatal("expected symlink overwrite to be rejected")
	}
	data, readErr := os.ReadFile(outsideFile)
	if readErr != nil {
		t.Fatalf("reading outside file: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("outside file content = %q, want original", string(data))
	}
}

func TestSyncOut_SkipsIfDestMissing(t *testing.T) {
	err := syncpkg.Out(t.Context(), &runtimetest.StubRuntime{}, "test-vol", "/workspace", "/nonexistent/dest")
	if err != nil {
		t.Fatalf("SyncOut with missing dest should not error: %v", err)
	}
}

func TestSyncOut_RejectsHostSymlinkOverwrite(t *testing.T) {
	destDir := t.TempDir()
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	mustWriteFile(t, outsideFile, []byte("original"), 0o600)
	if err := os.Symlink(outsideFile, filepath.Join(destDir, "modified.go")); err != nil {
		t.Fatalf("creating host symlink: %v", err)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "modified.go", Mode: 0o644, Size: int64(len("changed"))}, []byte("changed"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
	if err == nil {
		t.Fatal("expected symlink overwrite to be rejected")
	}
	data, readErr := os.ReadFile(outsideFile)
	if readErr != nil {
		t.Fatalf("reading outside file: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("outside file content = %q, want original", string(data))
	}
}

func TestSyncOut_ExtractsNestedFiles(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "src/lib/util.rs", Mode: 0o644, Size: 5}, []byte("fn x "))
	mustTarWrite(t, tw, &tar.Header{Name: "main.rs", Mode: 0o644, Size: 4}, []byte("fn m"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "vol", "/workspace", destDir)
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

func mustTarWrite(t *testing.T, tw *tar.Writer, h *tar.Header, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("tar write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write data: %v", err)
	}
}
