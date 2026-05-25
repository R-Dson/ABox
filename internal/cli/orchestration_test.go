package cli_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/r-dson/abox/internal/cli"
	"github.com/r-dson/abox/internal/runtime"
)

func TestRunSession_CreatesAndCleansUp(t *testing.T) {
	dir := t.TempDir()

	rec := newRecordingRuntime()

	err := cli.RunSessionForTest(t.Context(), rec, dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	// Should get an ExitError with code 0 (default)
	if err == nil {
		t.Fatal("expected ExitError, got nil")
	}
	exitErr, ok := err.(*cli.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 0 {
		t.Errorf("exit code = %d, want 0", exitErr.Code)
	}

	calls := rec.methods()

	// Must see volume creation
	creates := filter(calls, "VolumeCreate")
	if len(creates) < 4 {
		t.Errorf("expected >= 4 VolumeCreate calls, got %d", len(creates))
	}

	// Must see cleanup
	removes := filter(calls, "VolumeRemove")
	if len(removes) < 4 {
		t.Errorf("expected >= 4 VolumeRemove calls, got %d", len(removes))
	}

	// Must see a container run
	waits := filter(calls, "ContainerWait")
	if len(waits) < 1 {
		t.Error("expected at least one ContainerWait (editor container)")
	}
}

func TestRunSession_PropagatesExitCode(t *testing.T) {
	dir := t.TempDir()

	rec := newRecordingRuntime()
	rec.exitCode = 42

	err := cli.RunSessionForTest(t.Context(), rec, dir, &cli.SessionConfig{
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

	rec := newRecordingRuntime()
	rec.failOn = "VolumeCreate"

	_ = cli.RunSessionForTest(t.Context(), rec, dir, &cli.SessionConfig{
		Editor: "opencode",
	})

	calls := rec.methods()
	removes := filter(calls, "VolumeRemove")
	// Some volumes may have been created before the failure — those must be cleaned up
	if len(removes) == 0 && len(filter(calls, "VolumeCreate")) > 0 {
		t.Error("expected VolumeRemove calls to clean up after VolumeCreate failure")
	}
}

// --- recordingRuntime satisfies runtime.ContainerRuntime ---

type call struct {
	method string
	arg    string
}

type recordingRuntime struct {
	mu       sync.Mutex
	calls    []call
	exitCode int64
	failOn   string
	nextID   int
}

func newRecordingRuntime() *recordingRuntime {
	return &recordingRuntime{}
}

func (r *recordingRuntime) record(method, arg string) {
	r.mu.Lock()
	r.calls = append(r.calls, call{method: method, arg: arg})
	r.mu.Unlock()
}

func (r *recordingRuntime) shouldFail(method string) error {
	if r.failOn == method {
		return fmt.Errorf("injected failure on %s", method)
	}
	return nil
}

func (r *recordingRuntime) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]string, len(r.calls))
	for i, c := range r.calls {
		result[i] = c.method
	}
	return result
}

func (r *recordingRuntime) genID(prefix string) string {
	r.nextID++
	return fmt.Sprintf("%s-%d", prefix, r.nextID)
}

// ContainerRuntime implementation

func (r *recordingRuntime) VolumeCreate(_ context.Context, name string, _ map[string]string) error {
	r.record("VolumeCreate", name)
	return r.shouldFail("VolumeCreate")
}

func (r *recordingRuntime) VolumeRemove(_ context.Context, name string, _ bool) error {
	r.record("VolumeRemove", name)
	return nil
}

func (r *recordingRuntime) NetworkCreate(_ context.Context, name string, _ bool) (string, error) {
	r.record("NetworkCreate", name)
	if err := r.shouldFail("NetworkCreate"); err != nil {
		return "", err
	}
	return "net-" + name, nil
}

func (r *recordingRuntime) NetworkRemove(_ context.Context, id string) error {
	r.record("NetworkRemove", id)
	return nil
}

func (r *recordingRuntime) ContainerCreate(_ context.Context, spec runtime.ContainerSpec) (string, error) {
	r.record("ContainerCreate", spec.Image)
	if err := r.shouldFail("ContainerCreate"); err != nil {
		return "", err
	}
	return r.genID("c"), nil
}

func (r *recordingRuntime) ContainerStart(_ context.Context, id string) error {
	r.record("ContainerStart", id)
	return r.shouldFail("ContainerStart")
}

func (r *recordingRuntime) ContainerWait(_ context.Context, id string) (int64, error) {
	r.record("ContainerWait", id)
	return r.exitCode, r.shouldFail("ContainerWait")
}

func (r *recordingRuntime) ContainerRemove(_ context.Context, id string, _ bool) error {
	r.record("ContainerRemove", id)
	return nil
}

func (r *recordingRuntime) ContainerAttach(_ context.Context, id string) (io.ReadWriteCloser, error) {
	r.record("ContainerAttach", id)
	return nopRWC{}, nil
}

func (r *recordingRuntime) ContainerExec(_ context.Context, id string, _ []string) (int64, error) {
	r.record("ContainerExec", id)
	return 0, nil
}

func (r *recordingRuntime) CopyToContainer(_ context.Context, _, dst string, _ io.Reader) error {
	r.record("CopyToContainer", dst)
	return r.shouldFail("CopyToContainer")
}

func (r *recordingRuntime) CopyFromContainer(_ context.Context, _, src string) (io.ReadCloser, error) {
	r.record("CopyFromContainer", src)
	// Return an empty tar so extractTar doesn't fail
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.Close()
	return io.NopCloser(&buf), r.shouldFail("CopyFromContainer")
}

func (r *recordingRuntime) ImagePull(_ context.Context, ref string, _ io.Writer) error {
	r.record("ImagePull", ref)
	return nil
}

func (r *recordingRuntime) ImageExists(_ context.Context, ref string) (bool, error) {
	r.record("ImageExists", ref)
	return true, nil
}

func (r *recordingRuntime) Ping(_ context.Context) error {
	r.record("Ping", "")
	return nil
}

// helpers

type nopRWC struct{}

func (nopRWC) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopRWC) Write(_ []byte) (int, error) { return 0, nil }
func (nopRWC) Close() error                { return nil }

func filter(methods []string, want string) []string {
	var result []string
	for _, m := range methods {
		if m == want {
			result = append(result, m)
		}
	}
	return result
}
