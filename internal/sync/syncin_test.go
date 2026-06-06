package sync_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/runtimetest"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncIn_StreamsTarToContainer(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/app.go", []byte("package main"), 0o644)

	var capturedContent []byte
	stub := &runtimetest.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _, _ string, content io.Reader) error {
			data, _ := io.ReadAll(content)
			capturedContent = data
			return nil
		},
	}

	err := syncpkg.In(t.Context(), stub, dir, "test-vol", "/data", nil)
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(capturedContent))
	h, err := tr.Next()
	if err != nil {
		t.Fatalf("reading captured tar: %v", err)
	}
	if h.Name != "app.go" {
		t.Errorf("tar entry = %q, want %q", h.Name, "app.go")
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "package main" {
		t.Errorf("tar content = %q, want %q", string(content), "package main")
	}
}

func TestSyncIn_ReplacesContentsFromStagingDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/file.txt", []byte("data"), 0o644)

	var execCmds [][]string
	stub := &runtimetest.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _, _ string, content io.Reader) error {
			if _, err := io.Copy(io.Discard, content); err != nil {
				return fmt.Errorf("draining tar content: %w", err)
			}
			return nil
		},
		ContainerExecFn: func(_ context.Context, _ string, cmd []string) (int64, error) {
			execCmds = append(execCmds, append([]string(nil), cmd...))
			return 0, nil
		},
	}

	err := syncpkg.In(t.Context(), stub, dir, "test-vol", "/workspace", nil)
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	if len(execCmds) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(execCmds))
	}
	want := []string{"sh", "-c", fmt.Sprintf("find /workspace -mindepth 1 -maxdepth 1 ! -name .abx-tmp -exec rm -rf {} + && cp -a /workspace/.abx-tmp/. /workspace/ && rm -rf /workspace/.abx-tmp && chown -R %d:%d /workspace", os.Getuid(), os.Getgid())}
	if len(execCmds[1]) != len(want) {
		t.Fatalf("exec cmd = %v, want %v", execCmds[1], want)
	}
	for i, v := range execCmds[1] {
		if v != want[i] {
			t.Errorf("exec[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestSyncIn_ChownsDestinationForEditorUser(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/file.txt", []byte("data"), 0o644)

	var execCmds [][]string
	stub := &runtimetest.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _, _ string, content io.Reader) error {
			if _, err := io.Copy(io.Discard, content); err != nil {
				return fmt.Errorf("draining tar content: %w", err)
			}
			return nil
		},
		ContainerExecFn: func(_ context.Context, _ string, cmd []string) (int64, error) {
			execCmds = append(execCmds, append([]string(nil), cmd...))
			return 0, nil
		},
	}

	if err := syncpkg.In(t.Context(), stub, dir, "test-vol", "/workspace", nil); err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	if len(execCmds) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(execCmds))
	}
	if !strings.Contains(execCmds[1][2], "chown -R ") {
		t.Fatalf("replace command should chown destination, got %q", execCmds[1][2])
	}
	if !strings.Contains(execCmds[1][2], " /workspace") {
		t.Fatalf("replace command should chown workspace, got %q", execCmds[1][2])
	}
}

func TestSyncIn_StreamsToStagingPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/x.go", []byte("x"), 0o644)

	var capturedDst string
	stub := &runtimetest.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _, dst string, content io.Reader) error {
			capturedDst = dst
			if _, err := io.Copy(io.Discard, content); err != nil {
				return fmt.Errorf("draining tar content: %w", err)
			}
			return nil
		},
	}

	err := syncpkg.In(t.Context(), stub, dir, "vol", "/project", nil)
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	if capturedDst != "/project/.abx-tmp" {
		t.Errorf("CopyToContainer dst = %q, want %q", capturedDst, "/project/.abx-tmp")
	}
}
