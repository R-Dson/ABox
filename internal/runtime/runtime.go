package runtime

import (
	"context"
	"io"
)

// SyncImage is the lightweight Alpine image used for sync and bootstrap operations.
const SyncImage = "ghcr.io/r-dson/abox:sync"

// ContainerSpec holds the typed specification for creating a container.
type ContainerSpec struct {
	// Name is the container name.
	Name string
	// Image is the container image reference.
	Image string
	// Cmd is the command to run.
	Cmd []string
	// Env is the environment variables in KEY=VALUE format.
	Env []string
	// User is the user:group for the container (e.g. "0:0").
	User string
	// WorkingDir is the working directory inside the container.
	WorkingDir string
	// Tty enables terminal allocation.
	Tty bool
	// OpenStdin keeps stdin open.
	OpenStdin bool
	// Binds are the volume mount specifications (-v flags).
	Binds []string
	// CapDrop are the Linux capabilities to drop.
	CapDrop []string
	// CapAdd are the Linux capabilities to add.
	CapAdd []string
	// SecurityOpt are security options (seccomp, no-new-privileges).
	SecurityOpt []string
	// NetworkMode is the network mode (bridge, none, container:id, etc.).
	NetworkMode string
	// AutoRemove automatically removes the container on exit.
	AutoRemove bool
	// Init runs an init process as PID 1 inside the container.
	Init bool
	// PidsLimit bounds the number of processes in the container (0 = daemon default).
	PidsLimit int64
	// Memory is the memory limit in bytes (0 = unlimited).
	Memory int64
	// NanoCPUs is the CPU limit in nanocores (0 = unlimited).
	NanoCPUs int64
}

// ContainerRuntime abstracts Docker and Podman operations.
// No subprocess exec. No shell. Everything goes through the API.
type ContainerRuntime interface {
	// Close releases runtime client resources.
	Close() error

	// Volumes
	VolumeCreate(ctx context.Context, name string, labels map[string]string) error
	VolumeRemove(ctx context.Context, name string, force bool) error

	// Networks
	NetworkCreate(ctx context.Context, name string, internal bool) (string, error)
	NetworkRemove(ctx context.Context, id string) error

	// Containers
	ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error)
	ContainerStart(ctx context.Context, id string) error
	ContainerWait(ctx context.Context, id string) (int64, error)
	ContainerRemove(ctx context.Context, id string, force bool) error
	ContainerAttach(ctx context.Context, id string) (io.ReadWriteCloser, error)
	ContainerResize(ctx context.Context, id string, height, width uint) error
	ContainerSignal(ctx context.Context, id, signal string) error
	ContainerExec(ctx context.Context, id string, cmd []string) (int64, error)

	// Data transfer — streaming tar, no temp files
	CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error
	CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error)

	// Images
	ImagePull(ctx context.Context, ref string, out io.Writer) error
	ImageExists(ctx context.Context, ref string) (bool, error)
}
