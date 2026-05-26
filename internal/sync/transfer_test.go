package sync_test

import (
	"context"
	"testing"

	"github.com/r-dson/abox/internal/runtime"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncIn_SkipsNonexistentDir(t *testing.T) {
	err := syncpkg.In(t.Context(), &runtime.StubRuntime{}, "/nonexistent/dir", "test-vol", "/data", nil)
	if err != nil {
		t.Fatalf("SyncIn with nonexistent dir should not error: %v", err)
	}
}

func TestSyncIn(t *testing.T) {
	created := false
	stub := &runtime.StubRuntime{
		ContainerCreateFn: func(_ context.Context, _ runtime.ContainerSpec) (string, error) {
			created = true
			return "sync-c-1", nil
		},
	}

	srcDir := t.TempDir()
	err := syncpkg.In(t.Context(), stub, srcDir, "test-vol", "/data", nil)
	if err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}
	if !created {
		t.Error("expected a sync container to be created")
	}
}
