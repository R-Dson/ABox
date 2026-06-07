package runtime

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	dockerclient "github.com/docker/docker/client"
)

// NewPodman creates a runtime connected to the Podman socket.
// Podman's REST API is Docker-compatible, so we reuse the Docker client.
func NewPodman(ctx context.Context) (ContainerRuntime, error) {
	sock, err := podmanSocket()
	if err != nil {
		return nil, err
	}
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost("unix://"+sock),
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Podman client: %w", err)
	}
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("Podman daemon unreachable: %w", err)
	}
	return &dockerRuntime{client: cli}, nil
}

func podmanSocket() (string, error) {
	if s := os.Getenv("PODMAN_HOST"); s != "" {
		if strings.HasPrefix(s, "/") {
			return s, nil
		}
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("parsing PODMAN_HOST: %w", err)
		}
		if u.Scheme != "unix" {
			return "", fmt.Errorf("unsupported PODMAN_HOST scheme %q", u.Scheme)
		}
		return strings.TrimPrefix(s, "unix://"), nil
	}
	return fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid()), nil
}
