package container_test

import (
	"context"
	"io"
	"testing"

	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
)

func TestSession_CleanupRemovesVolumes(t *testing.T) {
	removedVolumes := []string{}
	mock := &stubRuntime{
		onVolumeRemove: func(name string, _ bool) error {
			removedVolumes = append(removedVolumes, name)
			return nil
		},
	}

	sess := container.NewSession("test-123", mock,
		container.SessionVolumes{
			ConfigVol: "abox-config-test-123",
			CacheVol:  "abox-cache-test-123",
			StateVol:  "abox-state-test-123",
			ShareVol:  "abox-share-test-123",
		},
	)

	sess.Cleanup(context.Background())

	expected := []string{
		"abox-config-test-123",
		"abox-cache-test-123",
		"abox-state-test-123",
		"abox-share-test-123",
	}
	if len(removedVolumes) != len(expected) {
		t.Fatalf("removed %d volumes, want %d: %v", len(removedVolumes), len(expected), removedVolumes)
	}
	for _, want := range expected {
		found := false
		for _, got := range removedVolumes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("volume %q not removed", want)
		}
	}
}

func TestSession_CleanupRemovesNetwork(t *testing.T) {
	networkRemoved := false
	mock := &stubRuntime{
		onNetworkRemove: func(id string) error {
			networkRemoved = true
			if id != "net-abc" {
				t.Errorf("NetworkRemove id = %q, want net-abc", id)
			}
			return nil
		},
		onVolumeRemove: func(string, bool) error { return nil },
	}

	sess := container.NewSession("test-456", mock,
		container.SessionVolumes{NetworkID: "net-abc"},
	)

	sess.Cleanup(context.Background())

	if !networkRemoved {
		t.Error("NetworkRemove was not called")
	}
}

func TestSession_CleanupSkipsEmptyFields(t *testing.T) {
	removedVolumes := []string{}
	networkRemoved := false
	mock := &stubRuntime{
		onVolumeRemove: func(name string, _ bool) error {
			removedVolumes = append(removedVolumes, name)
			return nil
		},
		onNetworkRemove: func(string) error {
			networkRemoved = true
			return nil
		},
	}

	sess := container.NewSession("test-789", mock,
		container.SessionVolumes{
			ConfigVol:    "abox-config-test-789",
			WorkspaceVol: "abox-workspace-test-789",
		},
	)

	sess.Cleanup(context.Background())

	expected := []string{"abox-config-test-789", "abox-workspace-test-789"}
	if len(removedVolumes) != len(expected) {
		t.Fatalf("removed %d volumes, want %d: %v", len(removedVolumes), len(expected), removedVolumes)
	}
	if networkRemoved {
		t.Error("NetworkRemove should not be called for empty NetworkID")
	}
}

func TestSyncImageConstant(t *testing.T) {
	if runtime.SyncImage != "ghcr.io/r-dson/abox:sync" {
		t.Errorf("SyncImage = %q, want ghcr.io/r-dson/abox:sync", runtime.SyncImage)
	}
}

// stubRuntime satisfies runtime.ContainerRuntime for tests.
// Only methods used by Session.Cleanup have real behavior; rest are no-ops.
type stubRuntime struct {
	onVolumeRemove  func(name string, force bool) error
	onNetworkRemove func(id string) error
}

func (s *stubRuntime) VolumeCreate(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (s *stubRuntime) VolumeRemove(_ context.Context, name string, force bool) error {
	if s.onVolumeRemove != nil {
		return s.onVolumeRemove(name, force)
	}
	return nil
}
func (s *stubRuntime) NetworkCreate(_ context.Context, _ string, _ bool) (string, error) {
	return "", nil
}
func (s *stubRuntime) NetworkRemove(_ context.Context, id string) error {
	if s.onNetworkRemove != nil {
		return s.onNetworkRemove(id)
	}
	return nil
}
func (s *stubRuntime) ContainerCreate(_ context.Context, _ runtime.ContainerSpec) (string, error) {
	return "", nil
}
func (s *stubRuntime) ContainerStart(_ context.Context, _ string) error          { return nil }
func (s *stubRuntime) ContainerWait(_ context.Context, _ string) (int64, error)  { return 0, nil }
func (s *stubRuntime) ContainerRemove(_ context.Context, _ string, _ bool) error { return nil }
func (s *stubRuntime) ContainerAttach(_ context.Context, _ string) (io.ReadWriteCloser, error) {
	return nil, nil
}
func (s *stubRuntime) ContainerExec(_ context.Context, _ string, _ []string) (int64, error) {
	return 0, nil
}
func (s *stubRuntime) CopyToContainer(_ context.Context, _, _ string, _ io.Reader) error {
	return nil
}
func (s *stubRuntime) CopyFromContainer(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *stubRuntime) ImagePull(_ context.Context, _ string, _ io.Writer) error { return nil }
func (s *stubRuntime) ImageExists(_ context.Context, _ string) (bool, error)    { return false, nil }
func (s *stubRuntime) Ping(_ context.Context) error                             { return nil }
