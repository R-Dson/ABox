package cli_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/r-dson/abox/internal/cli"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/runtimetest"
)

func TestRunSession_CreatesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	rec := newCallRecorder()

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	exitErr, ok := err.(*cli.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 0 {
		t.Errorf("exit code = %d, want 0", exitErr.Code)
	}

	calls := rec.methods()
	creates := filter(calls, "VolumeCreate")
	if len(creates) < 4 {
		t.Errorf("expected >= 4 VolumeCreate calls, got %d", len(creates))
	}

	removes := filter(calls, "VolumeRemove")
	if len(removes) < 4 {
		t.Errorf("expected >= 4 VolumeRemove calls, got %d", len(removes))
	}

	waits := filter(calls, "ContainerWait")
	if len(waits) < 1 {
		t.Error("expected at least one ContainerWait (editor container)")
	}
}

func TestRunSession_PropagatesExitCode(t *testing.T) {
	dir := t.TempDir()
	rec := newCallRecorder()
	rec.exitCode = 42

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	exitErr, ok := err.(*cli.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 42 {
		t.Errorf("exit code = %d, want 42", exitErr.Code)
	}
}

func TestRunSession_CleansUpOnVolumeFailure(t *testing.T) {
	dir := t.TempDir()
	rec := newCallRecorder()
	rec.failOn = "VolumeCreate"

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	calls := rec.methods()
	removes := filter(calls, "VolumeRemove")
	if len(removes) == 0 && len(filter(calls, "VolumeCreate")) > 0 {
		t.Error("expected VolumeRemove calls to clean up after VolumeCreate failure")
	}
}

func TestRunSession_SyncsWorkspaceBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	if len(filter(rec.methods(), "CopyFromContainer")) == 0 {
		t.Error("expected workspace volume to be copied back to host workdir")
	}
}

func TestRunSession_PassesWorkspaceMatcherToSyncOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	mustWriteFile(t, filepath.Join(dir, ".env"), []byte("HOST_SECRET"), 0o600)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: ".env", Mode: 0o600, Size: int64(len("SANDBOX_SECRET"))}); err != nil {
		t.Fatalf("tar write header: %v", err)
	}
	if _, err := tw.Write([]byte("SANDBOX_SECRET")); err != nil {
		t.Fatalf("tar write data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}

	rec := newCallRecorder()
	rec.copyFromData = buf.Bytes()

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})
	if err != nil {
		exitErr, ok := err.(*cli.ExitError)
		if !ok || exitErr.Code != 0 {
			t.Fatalf("RunSession() error = %v, want nil or exit code 0", err)
		}
	}
	if len(filter(rec.methods(), "CopyFromContainer")) == 0 {
		t.Fatal("expected sync-out to copy from container")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("reading host .env: %v", err)
	}
	if string(data) != "HOST_SECRET" {
		t.Fatalf("host .env = %q, want HOST_SECRET", string(data))
	}
}

func TestRunSession_PullsMissingImages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()
	rec.imageExists = false

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor:     "opencode",
		PullPolicy: "missing",
	})

	if len(filter(rec.methods(), "ImageExists")) == 0 {
		t.Error("expected image existence checks for missing pull policy")
	}
	if len(filter(rec.methods(), "ImagePull")) == 0 {
		t.Error("expected missing images to be pulled")
	}
}

func TestRunSession_OfflineSkipsImagePull(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()
	rec.imageExists = false

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor:  "opencode",
		Offline: true,
	})

	if len(filter(rec.methods(), "ImagePull")) != 0 {
		t.Error("offline mode must not pull images")
	}
}

func TestRunSession_NoInternetSkipsImagePull(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()
	rec.imageExists = false

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor:     "opencode",
		NoInternet: true,
	})

	if len(filter(rec.methods(), "ImagePull")) != 0 {
		t.Error("no-internet mode must not pull images")
	}
}

func TestRunSession_NoInternetRejectsExcludeURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor:     "opencode",
		NoInternet: true,
		ExcludeURL: "https://example.com/.abxignore",
	})
	if err == nil {
		t.Fatal("expected no-internet with exclude-url to fail")
	}
	if !strings.Contains(err.Error(), "exclude-url") {
		t.Fatalf("error = %v, want exclude-url", err)
	}
	if len(filter(rec.methods(), "ImagePull")) != 0 {
		t.Error("no-internet exclude-url failure must happen before image pull")
	}
}

func TestRunSession_DefaultPullPolicySkipsImagePull(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()
	rec.imageExists = false

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	if len(filter(rec.methods(), "ImagePull")) != 0 {
		t.Error("default pull policy must not auto-pull mutable image tags")
	}
}

func TestRunSession_InvalidConfigFailsBeforeRuntimeCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor:      "opencode",
		MemoryLimit: "not-memory",
	})
	if err == nil {
		t.Fatal("expected invalid config error")
	}
	if len(rec.methods()) != 0 {
		t.Fatalf("runtime calls = %v, want none", rec.methods())
	}
}

func TestRunSession_SyncsMissingFileConfigBack(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rec := newCallRecorder()

	_ = cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "aider",
	})

	copyFromCalls := len(filter(rec.methods(), "CopyFromContainer"))
	if copyFromCalls < 2 {
		t.Fatalf("CopyFromContainer calls = %d, want at least config file and workspace sync-out", copyFromCalls)
	}
}

func TestRunSession_ConflictPreservesAffectedVolumesAndReturnsNonZero(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	cacheFile := filepath.Join(home, ".cache", "opencode", "conflict.txt")
	mustWriteFile(t, cacheFile, []byte("before"), 0o644)
	workspaceFile := filepath.Join(dir, "workspace.txt")
	mustWriteFile(t, workspaceFile, []byte("host"), 0o644)

	rec := newCallRecorder()
	rec.copyFromDataByVolumePart = map[string][]byte{
		"cache":     tarBytes(t, "conflict.txt", "sandbox"),
		"workspace": tarBytes(t, "workspace.txt", "sandbox-workspace"),
	}
	rec.onEditorWait = func() {
		mustWriteFile(t, cacheFile, []byte("host changed"), 0o644)
	}

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{Editor: "opencode"})
	exitErr, ok := err.(*cli.ExitError)
	if !ok {
		t.Fatalf("RunSession() error = %T %[1]v, want ExitError", err)
	}
	if exitErr.Code == 0 {
		t.Fatal("conflict exit code = 0, want non-zero")
	}
	if len(filter(rec.copyFromVolumes(), "workspace")) == 0 {
		t.Fatal("workspace root should still sync out when cache conflicts")
	}
	if !containsVolumePart(rec.removedVolumeNames(), "workspace") {
		t.Fatal("workspace volume should be cleaned up")
	}
	if containsVolumePart(rec.removedVolumeNames(), "cache") {
		t.Fatal("conflicted cache volume should be preserved for recovery")
	}
}

func TestRunSession_ReturnsSyncOutError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rec := newCallRecorder()
	rec.failOn = "CopyFromContainer"

	err := cli.RunSession(t.Context(), rec.stub(), dir, &cli.SessionConfig{
		Editor: "opencode",
	})
	if err == nil {
		t.Fatal("expected sync-out error")
	}
	if _, ok := err.(*cli.ExitError); ok {
		t.Fatalf("expected sync-out error, got ExitError: %v", err)
	}
	if !strings.Contains(err.Error(), "sync-out") {
		t.Fatalf("error = %v, want sync-out", err)
	}
}

// --- callRecorder wraps the runtime test stub with method call recording ---

type callRecorder struct {
	mu                       sync.Mutex
	calls                    []string
	exitCode                 int64
	failOn                   string
	nextID                   int
	waitCount                int
	imageExists              bool
	copyFromData             []byte
	copyFromDataByVolumePart map[string][]byte
	copyFromVolumeParts      []string
	removedVolumes           []string
	containerVolumeByID      map[string]string
	onEditorWait             func()
	s                        runtimetest.StubRuntime
}

func newCallRecorder() *callRecorder {
	r := &callRecorder{containerVolumeByID: make(map[string]string)}
	r.s = runtimetest.StubRuntime{
		VolumeCreateFn: func(_ context.Context, _ string, _ map[string]string) error {
			r.add("VolumeCreate")
			return r.shouldFail("VolumeCreate")
		},
		VolumeRemoveFn: func(_ context.Context, name string, _ bool) error {
			r.add("VolumeRemove")
			r.mu.Lock()
			r.removedVolumes = append(r.removedVolumes, name)
			r.mu.Unlock()
			return nil
		},
		NetworkCreateFn: func(_ context.Context, name string, _ bool) (string, error) {
			r.add("NetworkCreate")
			if err := r.shouldFail("NetworkCreate"); err != nil {
				return "", err
			}
			return "net-" + name, nil
		},
		NetworkRemoveFn: func(_ context.Context, _ string) error {
			r.add("NetworkRemove")
			return nil
		},
		ContainerCreateFn: func(_ context.Context, spec runtime.ContainerSpec) (string, error) {
			r.add("ContainerCreate")
			if err := r.shouldFail("ContainerCreate"); err != nil {
				return "", err
			}
			id := r.nextContainerID()
			r.recordContainerVolume(id, spec)
			return id, nil
		},
		ContainerStartFn: func(_ context.Context, _ string) error {
			r.add("ContainerStart")
			return r.shouldFail("ContainerStart")
		},
		ContainerWaitFn: func(_ context.Context, _ string) (int64, error) {
			r.add("ContainerWait")
			r.mu.Lock()
			r.waitCount++
			waitCount := r.waitCount
			onEditorWait := r.onEditorWait
			r.mu.Unlock()
			// Bootstrap containers must succeed (exit 0).
			if r.exitCode != 0 && waitCount == 1 {
				return 0, nil
			}
			if waitCount > 1 && onEditorWait != nil {
				onEditorWait()
			}
			return r.exitCode, r.shouldFail("ContainerWait")
		},
		ContainerRemoveFn: func(_ context.Context, _ string, _ bool) error {
			r.add("ContainerRemove")
			return nil
		},
		ContainerExecFn: func(_ context.Context, _ string, _ []string) (int64, error) {
			r.add("ContainerExec")
			return 0, nil
		},
		CopyToContainerFn: func(_ context.Context, _, _ string, content io.Reader) error {
			r.add("CopyToContainer")
			if err := r.shouldFail("CopyToContainer"); err != nil {
				return err
			}
			if _, err := io.Copy(io.Discard, content); err != nil {
				return fmt.Errorf("draining copy-to-container content: %w", err)
			}
			return nil
		},
		CopyFromContainerFn: func(_ context.Context, id, _ string) (io.ReadCloser, error) {
			r.add("CopyFromContainer")
			data := r.copyFromDataForContainer(id)
			if data == nil {
				var buf bytes.Buffer
				tw := tar.NewWriter(&buf)
				tw.Close()
				data = buf.Bytes()
			}
			return io.NopCloser(bytes.NewReader(data)), r.shouldFail("CopyFromContainer")
		},
		ImageExistsFn: func(_ context.Context, _ string) (bool, error) {
			r.add("ImageExists")
			return r.imageExists, r.shouldFail("ImageExists")
		},
		ImagePullFn: func(_ context.Context, _ string, _ io.Writer) error {
			r.add("ImagePull")
			return r.shouldFail("ImagePull")
		},
	}
	return r
}

func (r *callRecorder) stub() runtime.ContainerRuntime { return &r.s }

func (r *callRecorder) add(method string) {
	r.mu.Lock()
	r.calls = append(r.calls, method)
	r.mu.Unlock()
}

func (r *callRecorder) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *callRecorder) nextContainerID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return fmt.Sprintf("c-%d", r.nextID)
}

func (r *callRecorder) shouldFail(method string) error {
	if r.failOn == method {
		return fmt.Errorf("injected failure on %s", method)
	}
	return nil
}

func (r *callRecorder) recordContainerVolume(id string, spec runtime.ContainerSpec) {
	for _, bind := range spec.Binds {
		volume, target, ok := strings.Cut(bind, ":")
		if !ok || target != "/data" {
			continue
		}
		r.mu.Lock()
		r.containerVolumeByID[id] = volume
		r.mu.Unlock()
		return
	}
}

func (r *callRecorder) copyFromDataForContainer(id string) []byte {
	r.mu.Lock()
	volume := r.containerVolumeByID[id]
	dataByPart := r.copyFromDataByVolumePart
	fallback := r.copyFromData
	r.mu.Unlock()

	if volume == "" {
		return fallback
	}
	for part, data := range dataByPart {
		if strings.Contains(volume, part) {
			r.mu.Lock()
			r.copyFromVolumeParts = append(r.copyFromVolumeParts, part)
			r.mu.Unlock()
			return data
		}
	}
	return fallback
}

func (r *callRecorder) copyFromVolumes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.copyFromVolumeParts))
	copy(out, r.copyFromVolumeParts)
	return out
}

func (r *callRecorder) removedVolumeNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.removedVolumes))
	copy(out, r.removedVolumes)
	return out
}

func filter(methods []string, want string) []string {
	var result []string
	for _, m := range methods {
		if m == want {
			result = append(result, m)
		}
	}
	return result
}

func mustWriteFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func tarBytes(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar write header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("tar write data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	return buf.Bytes()
}

func containsVolumePart(volumes []string, part string) bool {
	for _, volume := range volumes {
		if strings.Contains(volume, part) {
			return true
		}
	}
	return false
}
