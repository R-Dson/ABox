package sync_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func TestSyncIn_StagesInsideMountedVolume(t *testing.T) {
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644)

	var execCmd []string
	stub := &runtime.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _ string, _ string, content io.Reader) error {
			if _, err := io.Copy(io.Discard, content); err != nil {
				return fmt.Errorf("draining tar content: %w", err)
			}
			return nil
		},
		ContainerExecFn: func(_ context.Context, _ string, cmd []string) (int64, error) {
			execCmd = append([]string(nil), cmd...)
			return 0, nil
		},
	}

	if err := syncpkg.In(t.Context(), stub, srcDir, "test-vol", "/data", nil); err != nil {
		t.Fatalf("SyncIn() error: %v", err)
	}

	want := []string{"sh", "-c", fmt.Sprintf("find /data -mindepth 1 -maxdepth 1 ! -name .abx-tmp -exec rm -rf {} + && cp -a /data/.abx-tmp/. /data/ && rm -rf /data/.abx-tmp && chown -R %d:%d /data", os.Getuid(), os.Getgid())}
	if len(execCmd) != len(want) {
		t.Fatalf("exec command = %v, want %v", execCmd, want)
	}
	for i := range want {
		if execCmd[i] != want[i] {
			t.Fatalf("exec command = %v, want %v", execCmd, want)
		}
	}
}
