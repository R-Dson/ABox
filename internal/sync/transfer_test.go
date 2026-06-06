package sync_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/runtimetest"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSyncIn_InitializesMissingSourceAsEmptyVolume(t *testing.T) {
	var execCmd []string
	stub := &runtimetest.StubRuntime{
		ContainerExecFn: func(_ context.Context, _ string, cmd []string) (int64, error) {
			execCmd = append([]string(nil), cmd...)
			return 0, nil
		},
	}

	err := syncpkg.In(t.Context(), stub, "/nonexistent/dir", "test-vol", "/data", nil)
	if err != nil {
		t.Fatalf("SyncIn with nonexistent dir should not error: %v", err)
	}
	if !strings.Contains(execCmd[2], ".abx-volume-initialized") {
		t.Fatalf("expected missing source to initialize volume, got %v", execCmd)
	}
}

func TestSyncIn(t *testing.T) {
	created := false
	var spec runtime.ContainerSpec
	stub := &runtimetest.StubRuntime{
		ContainerCreateFn: func(_ context.Context, got runtime.ContainerSpec) (string, error) {
			created = true
			spec = got
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
	for _, want := range []string{"CHOWN", "DAC_OVERRIDE"} {
		if !slices.Contains(spec.CapAdd, want) {
			t.Fatalf("sync container CapAdd = %v, want %s", spec.CapAdd, want)
		}
	}
}

func TestSyncIn_StagesInsideMountedVolume(t *testing.T) {
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644)

	var execCmd []string
	stub := &runtimetest.StubRuntime{
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

	want := []string{"sh", "-c", fmt.Sprintf("find /data -mindepth 1 -maxdepth 1 ! -name .abx-tmp -exec rm -rf {} + && cp -a /data/.abx-tmp/. /data/ && rm -rf /data/.abx-tmp && touch /data/.abx-volume-initialized && chown -R %d:%d /data", os.Getuid(), os.Getgid())}
	if len(execCmd) != len(want) {
		t.Fatalf("exec command = %v, want %v", execCmd, want)
	}
	for i := range want {
		if execCmd[i] != want[i] {
			t.Fatalf("exec command = %v, want %v", execCmd, want)
		}
	}
}
