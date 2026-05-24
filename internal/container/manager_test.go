package container_test

import (
	"context"
	"io"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
)

func TestManager_RunCreatesAndStartsContainer(t *testing.T) {
	created := false
	started := false
	waited := false

	mock := &lifecycleTrackingRuntime{
		onVolumeCreate:    func(string, map[string]string) error { return nil },
		onVolumeRemove:    func(string, bool) error { return nil },
		onContainerCreate: func(runtime.ContainerSpec) (string, error) {
			created = true
			return "container-123", nil
		},
		onContainerStart: func(id string) error {
			started = true
			if id != "container-123" {
				t.Errorf("ContainerStart id = %q, want container-123", id)
			}
			return nil
		},
		onContainerWait: func(id string) (int64, error) {
			waited = true
			return 0, nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(mock)
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
	mock := &lifecycleTrackingRuntime{
		onVolumeCreate:    func(string, map[string]string) error { return nil },
		onVolumeRemove:    func(string, bool) error { return nil },
		onContainerCreate: func(runtime.ContainerSpec) (string, error) { return "c-1", nil },
		onContainerStart:  func(string) error { return nil },
		onContainerWait:   func(string) (int64, error) { return 42, nil },
	}

	mgr := container.NewManager(mock)
	spec := container.BuildSpec(
		config.EditorProfile{ImageTag: "test", CmdName: "sh"},
		container.NewSession("t", mock, container.SessionVolumes{}),
		"/workspace",
		&config.Config{},
	)

	exitCode, _ := mgr.Run(t.Context(), spec)
	if exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", exitCode)
	}
}

// lifecycleTrackingRuntime satisfies runtime.ContainerRuntime for lifecycle tests.
type lifecycleTrackingRuntime struct {
	onVolumeCreate    func(string, map[string]string) error
	onVolumeRemove    func(string, bool) error
	onNetworkCreate   func(string, bool) (string, error)
	onNetworkRemove   func(string) error
	onContainerCreate func(runtime.ContainerSpec) (string, error)
	onContainerStart  func(string) error
	onContainerWait   func(string) (int64, error)
}

func (l *lifecycleTrackingRuntime) VolumeCreate(_ context.Context, name string, labels map[string]string) error {
	if l.onVolumeCreate != nil {
		return l.onVolumeCreate(name, labels)
	}
	return nil
}
func (l *lifecycleTrackingRuntime) VolumeRemove(_ context.Context, name string, force bool) error {
	if l.onVolumeRemove != nil {
		return l.onVolumeRemove(name, force)
	}
	return nil
}
func (l *lifecycleTrackingRuntime) NetworkCreate(_ context.Context, name string, internal bool) (string, error) {
	if l.onNetworkCreate != nil {
		return l.onNetworkCreate(name, internal)
	}
	return "", nil
}
func (l *lifecycleTrackingRuntime) NetworkRemove(_ context.Context, id string) error {
	if l.onNetworkRemove != nil {
		return l.onNetworkRemove(id)
	}
	return nil
}
func (l *lifecycleTrackingRuntime) ContainerCreate(_ context.Context, spec runtime.ContainerSpec) (string, error) {
	if l.onContainerCreate != nil {
		return l.onContainerCreate(spec)
	}
	return "c-0", nil
}
func (l *lifecycleTrackingRuntime) ContainerStart(_ context.Context, id string) error {
	if l.onContainerStart != nil {
		return l.onContainerStart(id)
	}
	return nil
}
func (l *lifecycleTrackingRuntime) ContainerWait(_ context.Context, id string) (int64, error) {
	if l.onContainerWait != nil {
		return l.onContainerWait(id)
	}
	return 0, nil
}
func (l *lifecycleTrackingRuntime) ContainerRemove(_ context.Context, _ string, _ bool) error { return nil }
func (l *lifecycleTrackingRuntime) ContainerAttach(_ context.Context, _ string) (io.ReadWriteCloser, error) {
	return nil, nil
}
func (l *lifecycleTrackingRuntime) ContainerExec(_ context.Context, _ string, _ []string) (int64, error) {
	return 0, nil
}
func (l *lifecycleTrackingRuntime) CopyToContainer(_ context.Context, _, _ string, _ io.Reader) error {
	return nil
}
func (l *lifecycleTrackingRuntime) CopyFromContainer(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return nil, nil
}
func (l *lifecycleTrackingRuntime) ImagePull(_ context.Context, _ string, _ io.Writer) error { return nil }
func (l *lifecycleTrackingRuntime) ImageExists(_ context.Context, _ string) (bool, error)    { return false, nil }
func (l *lifecycleTrackingRuntime) Ping(_ context.Context) error                             { return nil }
