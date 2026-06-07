package container_test

import (
	"context"
	"slices"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/runtimetest"
)

func TestCreateSession_BootstrapHelperHasSecurityDefaults(t *testing.T) {
	var bootstrapSpec runtime.ContainerSpec
	stub := &runtimetest.StubRuntime{
		ContainerCreateFn: func(_ context.Context, spec runtime.ContainerSpec) (string, error) {
			bootstrapSpec = spec
			return "bootstrap", nil
		},
	}
	profile := config.EditorProfile{CmdName: "test", ImageTag: "test"}

	sess, err := container.CreateSession(t.Context(), stub, profile, &config.Config{}, false)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sess.Cleanup(t.Context())

	if bootstrapSpec.NetworkMode != "none" {
		t.Fatalf("bootstrap helper NetworkMode = %q, want none", bootstrapSpec.NetworkMode)
	}
	if !slices.Contains(bootstrapSpec.CapDrop, "ALL") {
		t.Fatalf("bootstrap helper CapDrop = %v, want ALL", bootstrapSpec.CapDrop)
	}
	if slices.Contains(bootstrapSpec.CapAdd, "DAC_OVERRIDE") {
		t.Fatalf("bootstrap helper CapAdd = %v, must not include DAC_OVERRIDE", bootstrapSpec.CapAdd)
	}
	if !slices.Contains(bootstrapSpec.SecurityOpt, "no-new-privileges") {
		t.Fatalf("bootstrap helper SecurityOpt = %v, want no-new-privileges", bootstrapSpec.SecurityOpt)
	}
	if bootstrapSpec.Memory == 0 || bootstrapSpec.NanoCPUs == 0 {
		t.Fatalf("bootstrap helper resources = memory %d nanoCPUs %d, want bounded", bootstrapSpec.Memory, bootstrapSpec.NanoCPUs)
	}
}

func TestSession_CleanupRemovesVolumes(t *testing.T) {
	removedVolumes := []string{}
	stub := &runtimetest.StubRuntime{
		VolumeRemoveFn: func(_ context.Context, name string, _ bool) error {
			removedVolumes = append(removedVolumes, name)
			return nil
		},
	}

	sess := container.NewSession("test-123", stub,
		container.Volumes{
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
	stub := &runtimetest.StubRuntime{
		NetworkRemoveFn: func(_ context.Context, id string) error {
			networkRemoved = true
			if id != "net-abc" {
				t.Errorf("NetworkRemove id = %q, want net-abc", id)
			}
			return nil
		},
	}

	sess := container.NewSession("test-456", stub,
		container.Volumes{NetworkID: "net-abc"},
	)

	sess.Cleanup(context.Background())

	if !networkRemoved {
		t.Error("NetworkRemove was not called")
	}
}

func TestSession_CleanupSkipsEmptyFields(t *testing.T) {
	removedVolumes := []string{}
	networkRemoved := false
	stub := &runtimetest.StubRuntime{
		VolumeRemoveFn: func(_ context.Context, name string, _ bool) error {
			removedVolumes = append(removedVolumes, name)
			return nil
		},
		NetworkRemoveFn: func(_ context.Context, _ string) error {
			networkRemoved = true
			return nil
		},
	}

	sess := container.NewSession("test-789", stub,
		container.Volumes{
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
