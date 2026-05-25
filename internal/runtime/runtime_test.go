package runtime_test

import (
	"testing"

	"github.com/r-dson/abox/internal/runtime"
)

func TestContainerSpecFields(t *testing.T) {
	spec := runtime.ContainerSpec{
		Name: "test-container",
	}
	if spec.Name != "test-container" {
		t.Errorf("Name = %q, want test-container", spec.Name)
	}
}

func TestSyncImageConstant(t *testing.T) {
	if runtime.SyncImage != "ghcr.io/r-dson/abox:sync" {
		t.Errorf("SyncImage = %q, want ghcr.io/r-dson/abox:sync", runtime.SyncImage)
	}
}
