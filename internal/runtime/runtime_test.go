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
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"sync image", runtime.SyncImage, "ghcr.io/r-dson/abox:sync"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
