package sync_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	if !slices.Contains(spec.CapAdd, "CHOWN") {
		t.Fatalf("sync container CapAdd = %v, want CHOWN", spec.CapAdd)
	}
	if !slices.Contains(spec.CapAdd, "DAC_OVERRIDE") {
		t.Fatalf("sync container CapAdd = %v, must include DAC_OVERRIDE for volume writes", spec.CapAdd)
	}
	if spec.NetworkMode != "none" {
		t.Fatalf("sync container NetworkMode = %q, want none", spec.NetworkMode)
	}
	if !slices.Contains(spec.SecurityOpt, "no-new-privileges") {
		t.Fatalf("sync container SecurityOpt = %v, want no-new-privileges", spec.SecurityOpt)
	}
	if spec.Memory == 0 || spec.NanoCPUs == 0 {
		t.Fatalf("sync helper resources = memory %d nanoCPUs %d, want bounded", spec.Memory, spec.NanoCPUs)
	}
	if spec.PidsLimit <= 0 {
		t.Fatalf("sync helper PidsLimit = %d, want positive", spec.PidsLimit)
	}
}

func TestSyncIn_CopyToContainerFailureUnblocksTarGoroutine(t *testing.T) {
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "file.txt"), []byte(strings.Repeat("x", 1024)), 0o644)

	var tarReader io.Reader
	stub := &runtimetest.StubRuntime{
		CopyToContainerFn: func(_ context.Context, _ string, _ string, content io.Reader) error {
			tarReader = content
			return errors.New("copy failed")
		},
	}

	err := syncpkg.In(t.Context(), stub, srcDir, "test-vol", "/data", nil)
	if err == nil || !strings.Contains(err.Error(), "copy failed") {
		t.Fatalf("In() error = %v, want copy failed", err)
	}
	if tarReader == nil {
		t.Fatal("CopyToContainer was not called")
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := tarReader.Read(make([]byte, 1))
		readDone <- err
	}()

	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("reader read succeeded after copy failure, want closed pipe error")
		}
	case <-time.After(time.Second):
		t.Fatal("tar reader remained blocked after CopyToContainer failure")
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
