package sync_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"testing"

	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncer_SyncIn_StreamsTarToContainer(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/app.go", []byte("package main"), 0o644)

	var capturedContent []byte
	mock := &tarCapturingRuntime{
		onCopyToContainer: func(_ string, _ string, content io.Reader) error {
			data, _ := io.ReadAll(content)
			capturedContent = data
			return nil
		},
	}

	s := syncpkg.NewSyncer(mock)
	err := s.SyncIn(t.Context(), dir, "test-vol", "/data")
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	// Verify captured data is a valid tar with our file
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

func TestSyncer_SyncIn_UsesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/file.txt", []byte("data"), 0o644)

	var execCmds [][]string
	mock := &tarCapturingRuntime{
		onContainerExec: func(_ string, cmd []string) (int64, error) {
			execCmds = append(execCmds, cmd)
			return 0, nil
		},
	}

	s := syncpkg.NewSyncer(mock)
	err := s.SyncIn(t.Context(), dir, "test-vol", "/workspace")
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	if len(execCmds) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(execCmds))
	}
	want := []string{"mv", "-T", "/workspace.abx-tmp", "/workspace"}
	if len(execCmds[0]) != len(want) {
		t.Fatalf("exec cmd = %v, want %v", execCmds[0], want)
	}
	for i, v := range execCmds[0] {
		if v != want[i] {
			t.Errorf("exec[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestSyncer_SyncIn_StreamsToStagingPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir+"/x.go", []byte("x"), 0o644)

	var capturedDst string
	mock := &tarCapturingRuntime{
		onCopyToContainer: func(_ string, dst string, _ io.Reader) error {
			capturedDst = dst
			return nil
		},
	}

	s := syncpkg.NewSyncer(mock)
	err := s.SyncIn(t.Context(), dir, "vol", "/project")
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	if capturedDst != "/project.abx-tmp" {
		t.Errorf("CopyToContainer dst = %q, want %q", capturedDst, "/project.abx-tmp")
	}
}

// tarCapturingRuntime captures CopyToContainer and ContainerExec calls.
type tarCapturingRuntime struct {
	syncStubRuntime
	onCopyToContainer func(containerID string, dst string, content io.Reader) error
	onContainerExec   func(containerID string, cmd []string) (int64, error)
}

func (t *tarCapturingRuntime) CopyToContainer(_ context.Context, _ string, dst string, content io.Reader) error {
	if t.onCopyToContainer != nil {
		return t.onCopyToContainer("", dst, content)
	}
	return nil
}

func (t *tarCapturingRuntime) ContainerExec(_ context.Context, _ string, cmd []string) (int64, error) {
	if t.onContainerExec != nil {
		return t.onContainerExec("", cmd)
	}
	return 0, nil
}
