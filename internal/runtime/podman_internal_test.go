package runtime

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestPodmanSocket_UsesPODMANHostNotDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/docker.sock")
	t.Setenv("PODMAN_HOST", "")

	sock, err := podmanSocket()
	if err != nil {
		t.Fatalf("podmanSocket() error = %v", err)
	}
	want := fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid())
	if sock != want {
		t.Fatalf("podmanSocket() = %q, want default %q", sock, want)
	}
}

func TestPodmanSocket_RejectsUnsupportedScheme(t *testing.T) {
	t.Setenv("PODMAN_HOST", "tcp://localhost:1234")

	_, err := podmanSocket()
	if err == nil || !strings.Contains(err.Error(), "unsupported PODMAN_HOST scheme") {
		t.Fatalf("podmanSocket() error = %v, want unsupported scheme", err)
	}
}
