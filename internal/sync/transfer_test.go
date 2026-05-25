package sync_test

import (
	"context"
	"testing"

	"github.com/r-dson/abox/internal/runtime"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestNewSyncer(t *testing.T) {
	s := syncpkg.NewSyncer(&runtime.StubRuntime{})
	if s == nil {
		t.Fatal("NewSyncer() returned nil")
	}
}

func TestSyncer_SyncIn_SkipsNonexistentDir(t *testing.T) {
	s := syncpkg.NewSyncer(&runtime.StubRuntime{})

	err := s.SyncIn(t.Context(), "/nonexistent/dir", "test-vol", "/data")
	if err != nil {
		t.Fatalf("SyncIn with nonexistent dir should not error: %v", err)
	}
}

func TestSyncer_SyncIn(t *testing.T) {
	created := false
	stub := &runtime.StubRuntime{
		ContainerCreateFn: func(_ context.Context, _ runtime.ContainerSpec) (string, error) {
			created = true
			return "sync-c-1", nil
		},
	}

	s := syncpkg.NewSyncer(stub)
	srcDir := t.TempDir()

	err := s.SyncIn(t.Context(), srcDir, "test-vol", "/data")
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}
	if !created {
		t.Error("expected a sync container to be created")
	}
}
