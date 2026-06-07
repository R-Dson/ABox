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
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtimetest"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncOut_WithMatcherSkipsExcludedArchiveEntryAndPreservesHostFile(t *testing.T) {
	destDir := t.TempDir()
	mustWriteFile(t, filepath.Join(destDir, ".env"), []byte("HOST_SECRET"), 0o600)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".env", Mode: 0o600, Size: int64(len("SANDBOX_SECRET"))}, []byte("SANDBOX_SECRET"))
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.OutWithOptions(t.Context(), stub, "test-vol", "/workspace", destDir, syncpkg.Options{
		Matcher: exclusion.NewMatcher([]string{".env"}),
	})
	if err != nil {
		t.Fatalf("OutWithOptions() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, ".env"))
	if err != nil {
		t.Fatalf("reading host .env: %v", err)
	}
	if string(data) != "HOST_SECRET" {
		t.Fatalf("host .env = %q, want HOST_SECRET", string(data))
	}
}

func TestSyncOut_WithMatcherDoesNotCreateExcludedPath(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".env.local", Mode: 0o600, Size: int64(len("SANDBOX_SECRET"))}, []byte("SANDBOX_SECRET"))
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.OutWithOptions(t.Context(), stub, "test-vol", "/workspace", destDir, syncpkg.Options{
		Matcher: exclusion.NewMatcher([]string{".env.*"}),
	})
	if err != nil {
		t.Fatalf("OutWithOptions() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("excluded file should not be created, stat error = %v", err)
	}
}

func TestSyncOut_RejectsSymlinkDestinationRoot(t *testing.T) {
	outsideDir := t.TempDir()
	destParent := t.TempDir()
	destDir := filepath.Join(destParent, "workspace")
	if err := os.Symlink(outsideDir, destDir); err != nil {
		t.Fatalf("creating root symlink: %v", err)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "file.txt", Mode: 0o644, Size: int64(len("changed"))}, []byte("changed"))
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
	if err == nil || !strings.Contains(err.Error(), "destination root is a symlink") {
		t.Fatalf("Out() error = %v, want destination root is a symlink", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file should not be created, stat error = %v", err)
	}
}

func TestSyncOut_RejectsSymlinkParentBeforeMkdirAll(t *testing.T) {
	destDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(destDir, "dir")); err != nil {
		t.Fatalf("creating parent symlink: %v", err)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "dir/file.txt", Mode: 0o644, Size: int64(len("changed"))}, []byte("changed"))
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
	if err == nil {
		t.Fatal("expected parent symlink error")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file should not be created, stat error = %v", err)
	}
}

func TestSyncOut_RejectsTraversalBeforeWrite(t *testing.T) {
	parentDir := t.TempDir()
	destDir := filepath.Join(parentDir, "workspace")
	mustMkdirAll(t, destDir)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "../escape.txt", Mode: 0o644, Size: int64(len("escape"))}, []byte("escape"))
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
	if err == nil || !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("Out() error = %v, want escape error", err)
	}
	if _, err := os.Stat(filepath.Join(parentDir, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape file should not be created, stat error = %v", err)
	}
}

func TestSyncOut_RemovesContainerDeletedTrackedFile(t *testing.T) {
	destDir := t.TempDir()
	oldPath := filepath.Join(destDir, "old.txt")
	mustWriteFile(t, oldPath, []byte("old"), 0o644)
	snapshot, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "workspace", Path: destDir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(emptyTar(t))), nil
		},
	}

	if err := syncpkg.OutWithOptions(t.Context(), stub, "test-vol", "/workspace", destDir, syncpkg.Options{
		Snapshot:      &snapshot.Roots[0],
		DeleteMissing: true,
	}); err != nil {
		t.Fatalf("OutWithOptions() error: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old.txt should be removed, stat error = %v", err)
	}
}

func TestSyncOut_DoesNotRemoveExcludedHostFileWhenMissingFromArchive(t *testing.T) {
	destDir := t.TempDir()
	envPath := filepath.Join(destDir, ".env")
	mustWriteFile(t, envPath, []byte("HOST_SECRET"), 0o600)
	matcher := exclusion.NewMatcher([]string{".env"})
	snapshot, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "workspace", Path: destDir, Matcher: matcher}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(emptyTar(t))), nil
		},
	}

	if err := syncpkg.OutWithOptions(t.Context(), stub, "test-vol", "/workspace", destDir, syncpkg.Options{
		Matcher:       matcher,
		Snapshot:      &snapshot.Roots[0],
		DeleteMissing: true,
	}); err != nil {
		t.Fatalf("OutWithOptions() error: %v", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if string(data) != "HOST_SECRET" {
		t.Fatalf(".env = %q, want HOST_SECRET", string(data))
	}
}

func TestSyncOut_DoesNotRemoveHostCreatedUntrackedFile(t *testing.T) {
	destDir := t.TempDir()
	snapshot, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "workspace", Path: destDir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	notesPath := filepath.Join(destDir, "notes.txt")
	mustWriteFile(t, notesPath, []byte("host-created"), 0o644)

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(emptyTar(t))), nil
		},
	}

	if err := syncpkg.OutWithOptions(t.Context(), stub, "test-vol", "/workspace", destDir, syncpkg.Options{
		Snapshot:      &snapshot.Roots[0],
		DeleteMissing: true,
	}); err != nil {
		t.Fatalf("OutWithOptions() error: %v", err)
	}
	data, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("reading host-created notes: %v", err)
	}
	if string(data) != "host-created" {
		t.Fatalf("notes.txt = %q, want host-created", string(data))
	}
}

func TestSyncOut_RecreatesSafeRelativeSymlink(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: "target.txt", Mode: 0o644, Size: int64(len("target"))}, []byte("target"))
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target.txt", Mode: 0o777}); err != nil {
		t.Fatalf("tar write symlink header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	if err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir); err != nil {
		t.Fatalf("Out() error: %v", err)
	}
	linkTarget, err := os.Readlink(filepath.Join(destDir, "link"))
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	if linkTarget != "target.txt" {
		t.Fatalf("link target = %q, want target.txt", linkTarget)
	}
}

func TestSyncOut_RejectsEscapingSymlink(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside.txt", Mode: 0o777}); err != nil {
		t.Fatalf("tar write symlink header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(tarBuf.Bytes())), nil
		},
	}

	err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Out() error = %v, want symlink rejection", err)
	}
	if _, err := os.Lstat(filepath.Join(destDir, "link")); !os.IsNotExist(err) {
		t.Fatalf("escaping symlink should not be created, stat error = %v", err)
	}
}

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

func TestSyncOut_SkipsInternalVolumeMarker(t *testing.T) {
	destDir := t.TempDir()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarWrite(t, tw, &tar.Header{Name: ".abx-volume-initialized", Mode: 0o644, Size: 0}, nil)
	mustTarWrite(t, tw, &tar.Header{Name: "kept.txt", Mode: 0o644, Size: int64(len("kept"))}, []byte("kept"))
	tw.Close()

	stub := &runtimetest.StubRuntime{
		CopyFromContainerFn: func(_ context.Context, _, _ string) (io.ReadCloser, error) {
			return io.NopCloser(&tarBuf), nil
		},
	}

	if err := syncpkg.Out(t.Context(), stub, "test-vol", "/workspace", destDir); err != nil {
		t.Fatalf("SyncOut() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, ".abx-volume-initialized")); !os.IsNotExist(err) {
		t.Fatalf("internal marker should not sync out, stat error = %v", err)
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

func emptyTar(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.Close(); err != nil {
		t.Fatalf("closing empty tar: %v", err)
	}
	return buf.Bytes()
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
