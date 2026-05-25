package container_test

import (
	"context"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
)

func TestManager_RunCreatesAndStartsContainer(t *testing.T) {
	created := false
	started := false
	waited := false

	stub := &runtime.StubRuntime{
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
		ContainerWaitFn: func(context.Context, string) (int64, error) {
			waited = true
			return 0, nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(stub)
	sess, _ := mgr.CreateSession(t.Context(), profile, cfg)
	defer sess.Cleanup(t.Context())

	spec := container.BuildSpec(profile, sess, "/workspace", cfg)
	exitCode, err := mgr.Run(t.Context(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !created {
		t.Error("ContainerCreate was not called")
	}
	if !started {
		t.Error("ContainerStart was not called")
	}
	if !waited {
		t.Error("ContainerWait was not called")
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
}

func TestManager_RunPropagatesExitCode(t *testing.T) {
	stub := &runtime.StubRuntime{
		ContainerCreateFn: func(context.Context, runtime.ContainerSpec) (string, error) {
			return "c-1", nil
		},
		ContainerStartFn: func(context.Context, string) error { return nil },
		ContainerWaitFn:  func(context.Context, string) (int64, error) { return 42, nil },
	}

	mgr := container.NewManager(stub)
	spec := container.BuildSpec(
		config.EditorProfile{ImageTag: "test", CmdName: "sh"},
		container.NewSession("t", stub, container.SessionVolumes{}),
		"/workspace",
		&config.Config{},
	)

	exitCode, _ := mgr.Run(t.Context(), spec)
	if exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", exitCode)
	}
}
