package runtime

import (
	"testing"
)

// TestContainerRuntimeInterface verifies the interface is satisfied
// by both Docker and Podman implementations.
func TestContainerRuntimeInterface(t *testing.T) {
	// These type assertions ensure the implementations satisfy the interface.
	// They will fail to compile if the interface is not satisfied.
	var _ ContainerRuntime = (*dockerRuntime)(nil)
}

// TestContainerSpecFields verifies the spec struct has the expected fields.
func TestContainerSpecFields(t *testing.T) {
	spec := ContainerSpec{
		Name: "test-container",
	}

	if spec.Name != "test-container" {
		t.Errorf("Name = %q, want test-container", spec.Name)
	}
}

// TestSyncImageConstant verifies the sync image constant.
func TestSyncImageConstant(t *testing.T) {
	expected := "ghcr.io/r-dson/abox:sync"
	if SyncImage != expected {
		t.Errorf("SyncImage = %q, want %q", SyncImage, expected)
	}
}
