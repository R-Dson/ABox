package sync_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/r-dson/abox/internal/runtime"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestNewSyncer(t *testing.T) {
	mock := &syncStubRuntime{}
	s := syncpkg.NewSyncer(mock)
	if s == nil {
		t.Fatal("NewSyncer() returned nil")
	}
}

func TestSyncer_SyncIn_SkipsNonexistentDir(t *testing.T) {
	mock := &syncStubRuntime{}
	s := syncpkg.NewSyncer(mock)

	err := s.SyncIn(t.Context(), "/nonexistent/dir", "test-vol", "/data")
	if err != nil {
		t.Fatalf("SyncIn with nonexistent dir should not error: %v", err)
	}
}

func TestSyncer_SyncIn(t *testing.T) {
	created := false
	mock := &syncStubRuntime{
		onContainerCreate: func(runtime.ContainerSpec) (string, error) {
			created = true
			return "sync-c-1", nil
		},
		onContainerStart:  func(string) error { return nil },
		onContainerWait:   func(string) (int64, error) { return 0, nil },
		onContainerRemove: func(string, bool) error { return nil },
	}

	s := syncpkg.NewSyncer(mock)
	srcDir := t.TempDir()

	err := s.SyncIn(t.Context(), srcDir, "test-vol", "/data")
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}
	if !created {
		t.Error("expected a sync container to be created")
	}
}

// syncStubRuntime satisfies runtime.ContainerRuntime for sync tests.
type syncStubRuntime struct {
	onContainerCreate func(runtime.ContainerSpec) (string, error)
	onContainerStart  func(string) error
	onContainerWait   func(string) (int64, error)
	onContainerRemove func(string, bool) error
}

func (s *syncStubRuntime) VolumeCreate(context.Context, string, map[string]string) error {
	return nil
}
func (s *syncStubRuntime) VolumeRemove(context.Context, string, bool) error { return nil }
func (s *syncStubRuntime) NetworkCreate(context.Context, string, bool) (string, error) {
	return "", nil
}
func (s *syncStubRuntime) NetworkRemove(context.Context, string) error { return nil }
func (s *syncStubRuntime) ContainerCreate(_ context.Context, spec runtime.ContainerSpec) (string, error) {
	if s.onContainerCreate != nil {
		return s.onContainerCreate(spec)
	}
	return "c-0", nil
}
func (s *syncStubRuntime) ContainerStart(_ context.Context, id string) error {
	if s.onContainerStart != nil {
		return s.onContainerStart(id)
	}
	return nil
}
func (s *syncStubRuntime) ContainerWait(_ context.Context, id string) (int64, error) {
	if s.onContainerWait != nil {
		return s.onContainerWait(id)
	}
	return 0, nil
}
func (s *syncStubRuntime) ContainerRemove(_ context.Context, id string, force bool) error {
	if s.onContainerRemove != nil {
		return s.onContainerRemove(id, force)
	}
	return nil
}
func (s *syncStubRuntime) ContainerAttach(context.Context, string) (io.ReadWriteCloser, error) {
	return nil, nil
}
func (s *syncStubRuntime) ContainerExec(context.Context, string, []string) (int64, error) {
	return 0, nil
}
func (s *syncStubRuntime) CopyToContainer(context.Context, string, string, io.Reader) error {
	return nil
}
func (s *syncStubRuntime) CopyFromContainer(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s *syncStubRuntime) ImagePull(context.Context, string, io.Writer) error { return nil }
func (s *syncStubRuntime) ImageExists(context.Context, string) (bool, error)  { return false, nil }
func (s *syncStubRuntime) Ping(context.Context) error                         { return nil }
