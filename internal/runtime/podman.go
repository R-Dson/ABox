package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"

	dockerclient "github.com/docker/docker/client"
)

// NewPodman creates a runtime connected to the Podman socket.
// Podman's REST API is Docker-compatible, so we reuse the Docker client.
func NewPodman(ctx context.Context) (ContainerRuntime, error) {
	sock := podmanSocket()
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost("unix://"+sock),
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Podman client: %w", err)
	}
	if err := pingWithClient(ctx, cli); err != nil {
		return nil, fmt.Errorf("Podman daemon unreachable: %w", err)
	}
	return &dockerRuntime{client: cli}, nil
}

func podmanSocket() string {
	if s := os.Getenv("DOCKER_HOST"); s != "" {
		return strings.TrimPrefix(s, "unix://")
	}
	return fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid())
}

func pingWithClient(ctx context.Context, cli *dockerclient.Client) error {
	_, err := cli.Ping(ctx)
	return fmt.Errorf("podman ping: %w", err)
}
