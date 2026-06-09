package runtime

import (
	"context"
	"fmt"
	"os"
)

// Detect auto-detects the container runtime.
// It respects the ABOX_RUNTIME env var for explicit override,
// then falls back to Docker, then Podman.
// When both fail, the error includes diagnostic details from each.
func Detect(ctx context.Context) (ContainerRuntime, error) {
	if name := os.Getenv("ABOX_RUNTIME"); name != "" {
		return detectNamed(ctx, name)
	}

	dockerRT, dockerErr := NewDocker(ctx)
	if dockerErr == nil {
		return dockerRT, nil
	}

	podmanRT, podmanErr := NewPodman(ctx)
	if podmanErr == nil {
		return podmanRT, nil
	}

	return nil, fmt.Errorf("neither Docker nor Podman is available (docker: %v, podman: %v)",
		dockerErr, podmanErr)
}

func detectNamed(ctx context.Context, name string) (ContainerRuntime, error) {
	switch name {
	case "docker":
		return NewDocker(ctx)
	case "podman":
		return NewPodman(ctx)
	default:
		return nil, fmt.Errorf("unknown runtime %q: must be docker or podman", name)
	}
}
