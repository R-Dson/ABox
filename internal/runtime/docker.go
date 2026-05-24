package runtime

import (
	"context"
	"io"

	dockerclient "github.com/docker/docker/client"
)

type dockerRuntime struct {
	client *dockerclient.Client
}

// Compile-time interface check.
var _ ContainerRuntime = (*dockerRuntime)(nil)

func (d *dockerRuntime) VolumeCreate(ctx context.Context, name string, labels map[string]string) error {
	return nil
}

func (d *dockerRuntime) VolumeRemove(ctx context.Context, name string, force bool) error {
	return nil
}

func (d *dockerRuntime) NetworkCreate(ctx context.Context, name string, internal bool) (string, error) {
	return "", nil
}

func (d *dockerRuntime) NetworkRemove(ctx context.Context, id string) error {
	return nil
}

func (d *dockerRuntime) ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error) {
	return "", nil
}

func (d *dockerRuntime) ContainerStart(ctx context.Context, id string) error {
	return nil
}

func (d *dockerRuntime) ContainerWait(ctx context.Context, id string) (int64, error) {
	return 0, nil
}

func (d *dockerRuntime) ContainerRemove(ctx context.Context, id string, force bool) error {
	return nil
}

func (d *dockerRuntime) ContainerAttach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *dockerRuntime) ContainerExec(ctx context.Context, id string, cmd []string) (int64, error) {
	return 0, nil
}

func (d *dockerRuntime) CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error {
	return nil
}

func (d *dockerRuntime) CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error) {
	return nil, nil
}

func (d *dockerRuntime) ImagePull(ctx context.Context, ref string, out io.Writer) error {
	return nil
}

func (d *dockerRuntime) ImageExists(ctx context.Context, ref string) (bool, error) {
	return false, nil
}

func (d *dockerRuntime) Ping(ctx context.Context) error {
	return nil
}
