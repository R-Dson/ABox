package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Detect auto-detects the container runtime.
// It respects the ABOX_RUNTIME env var for explicit override,
// then falls back to Docker, then Podman.
func Detect(ctx context.Context) (ContainerRuntime, error) {
	if name := os.Getenv("ABOX_RUNTIME"); name != "" {
		return detectNamed(ctx, name)
	}
	if rt, err := NewDocker(ctx); err == nil {
		return rt, nil
	}
	if rt, err := NewPodman(ctx); err == nil {
		return rt, nil
	}
	return nil, errors.New("neither Docker nor Podman is available or healthy")
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
