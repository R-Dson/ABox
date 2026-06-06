package container_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/runtimetest"
)

func TestRun_CreatesAndStartsContainer(t *testing.T) {
	created := false
	started := false
	attached := false
	waited := false

	stub := &runtimetest.StubRuntime{
		VolumeCreateFn: func(context.Context, string, map[string]string) error { return nil },
		ContainerCreateFn: func(_ context.Context, _ runtime.ContainerSpec) (string, error) {
			created = true
			return "container-123", nil
		},
		ContainerStartFn: func(_ context.Context, id string) error {
			started = true
			if id != "container-123" {
				t.Errorf("ContainerStart id = %q, want container-123", id)
			}
			return nil
		},
		ContainerAttachFn: func(_ context.Context, id string) (io.ReadWriteCloser, error) {
			attached = true
			if id != "container-123" {
				t.Errorf("ContainerAttach id = %q, want container-123", id)
			}
			return testReadWriteCloser{reader: strings.NewReader("")}, nil
		},
		ContainerWaitFn: func(context.Context, string) (int64, error) {
			waited = true
			return 0, nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	sess, _ := container.CreateSession(t.Context(), stub, profile, cfg, false)
	defer sess.Cleanup(t.Context())

	spec := mustBuildSpec(t, profile, sess, "/workspace", cfg)
	exitCode, err := container.Run(t.Context(), stub, spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !created {
		t.Error("ContainerCreate was not called")
	}
	if !started {
		t.Error("ContainerStart was not called")
	}
	if !attached {
		t.Error("ContainerAttach was not called")
	}
	if !waited {
		t.Error("ContainerWait was not called")
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
}

type testReadWriteCloser struct {
	reader io.Reader
}

func (r testReadWriteCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p) //nolint:wrapcheck // io.Reader implementations must preserve io.EOF.
}

func (testReadWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (testReadWriteCloser) Close() error {
	return nil
}

func TestRun_PropagatesExitCode(t *testing.T) {
	stub := &runtimetest.StubRuntime{
		ContainerCreateFn: func(context.Context, runtime.ContainerSpec) (string, error) {
			return "c-1", nil
		},
		ContainerStartFn: func(context.Context, string) error { return nil },
		ContainerWaitFn:  func(context.Context, string) (int64, error) { return 42, nil },
	}

	spec := mustBuildSpec(t,
		config.EditorProfile{ImageTag: "test", CmdName: "sh"},
		container.NewSession("t", stub, container.Volumes{}),
		"/workspace",
		&config.Config{},
	)

	exitCode, _ := container.Run(t.Context(), stub, spec)
	if exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", exitCode)
	}
}
