package runtime

import (
	"context"
	"fmt"
	"io"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockermount "github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockerimage "github.com/docker/docker/api/types/image"
	dockervolume "github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-units"
)

type dockerRuntime struct {
	client *dockerclient.Client
}

// Compile-time interface check.
var _ ContainerRuntime = (*dockerRuntime)(nil)

// NewDocker creates a Docker runtime using the Moby SDK client.
func NewDocker(ctx context.Context) (*dockerRuntime, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("Docker daemon unreachable: %w", err)
	}
	return &dockerRuntime{client: cli}, nil
}

func (d *dockerRuntime) VolumeCreate(ctx context.Context, name string, labels map[string]string) error {
	_, err := d.client.VolumeCreate(ctx, dockervolume.CreateOptions{
		Name:   name,
		Labels: labels,
	})
	if err != nil {
		return fmt.Errorf("creating volume %s: %w", name, err)
	}
	return nil
}

func (d *dockerRuntime) VolumeRemove(ctx context.Context, name string, force bool) error {
	return d.client.VolumeRemove(ctx, name, force)
}

func (d *dockerRuntime) NetworkCreate(ctx context.Context, name string, internal bool) (string, error) {
	resp, err := d.client.NetworkCreate(ctx, name, dockernetwork.CreateOptions{
		Internal: internal,
		Labels:   map[string]string{"app": "abox"},
	})
	if err != nil {
		return "", fmt.Errorf("creating network %s: %w", name, err)
	}
	return resp.ID, nil
}

func (d *dockerRuntime) NetworkRemove(ctx context.Context, id string) error {
	return d.client.NetworkRemove(ctx, id)
}

func (d *dockerRuntime) ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error) {
	containerConfig := &dockercontainer.Config{
		Image:       spec.Image,
		Cmd:         spec.Cmd,
		Env:         spec.Env,
		User:        spec.User,
		WorkingDir:  spec.WorkingDir,
		Tty:         spec.Tty,
		OpenStdin:   spec.OpenStdin,
		AttachStdin: spec.OpenStdin,
		AttachStdout: true,
		AttachStderr: true,
	}

	mounts := make([]dockermount.Mount, 0, len(spec.Binds))
	for _, bind := range spec.Binds {
		mounts = append(mounts, dockermount.Mount{
			Type:   dockermount.TypeBind,
			Source: bind, // simplified: will be parsed properly in full impl
			Target: bind,
		})
	}

	hostConfig := &dockercontainer.HostConfig{
		Mounts:      mounts,
		CapDrop:     spec.CapDrop,
		CapAdd:      spec.CapAdd,
		SecurityOpt: spec.SecurityOpt,
		AutoRemove:  spec.AutoRemove,
	}

	if spec.NetworkMode != "" {
		hostConfig.NetworkMode = dockercontainer.NetworkMode(spec.NetworkMode)
	}

	if spec.Memory > 0 {
		hostConfig.Memory = spec.Memory
	}
	if spec.NanoCPUs > 0 {
		hostConfig.NanoCPUs = spec.NanoCPUs
	}

	resp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("creating container %s: %w", spec.Name, err)
	}
	return resp.ID, nil
}

func (d *dockerRuntime) ContainerStart(ctx context.Context, id string) error {
	return d.client.ContainerStart(ctx, id, dockercontainer.StartOptions{})
}

func (d *dockerRuntime) ContainerWait(ctx context.Context, id string) (int64, error) {
	statusCh, errCh := d.client.ContainerWait(ctx, id, dockercontainer.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return -1, err
	case status := <-statusCh:
		return status.StatusCode, nil
	}
}

func (d *dockerRuntime) ContainerRemove(ctx context.Context, id string, force bool) error {
	return d.client.ContainerRemove(ctx, id, dockercontainer.RemoveOptions{Force: force})
}

func (d *dockerRuntime) ContainerAttach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	resp, err := d.client.ContainerAttach(ctx, id, dockercontainer.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("attaching to container %s: %w", id, err)
	}
	return resp.Conn, nil
}

func (d *dockerRuntime) ContainerExec(ctx context.Context, id string, cmd []string) (int64, error) {
	execConfig := dockercontainer.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	execResp, err := d.client.ContainerExecCreate(ctx, id, execConfig)
	if err != nil {
		return -1, fmt.Errorf("creating exec: %w", err)
	}
	if err := d.client.ContainerExecStart(ctx, execResp.ID, dockercontainer.ExecStartOptions{}); err != nil {
		return -1, fmt.Errorf("starting exec: %w", err)
	}
	resp, err := d.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return -1, fmt.Errorf("inspecting exec: %w", err)
	}
	return int64(resp.ExitCode), nil
}

func (d *dockerRuntime) CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error {
	return d.client.CopyToContainer(ctx, id, dstPath, content, dockercontainer.CopyToContainerOptions{})
}

func (d *dockerRuntime) CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error) {
	reader, _, err := d.client.CopyFromContainer(ctx, id, srcPath)
	return reader, err
}

func (d *dockerRuntime) ImagePull(ctx context.Context, ref string, out io.Writer) error {
	resp, err := d.client.ImagePull(ctx, ref, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", ref, err)
	}
	defer resp.Close()
	if out != nil {
		_, _ = io.Copy(out, resp)
	} else {
		_, _ = io.Copy(io.Discard, resp)
	}
	return nil
}

func (d *dockerRuntime) ImageExists(ctx context.Context, ref string) (bool, error) {
	_, _, err := d.client.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		if dockerclient.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *dockerRuntime) Ping(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	return err
}

// parseMemoryBytes converts human-readable memory strings ("4g", "512m") to bytes.
func parseMemoryBytes(s string) int64 {
	bytes, err := units.RAMInBytes(s)
	if err != nil {
		return 0
	}
	return bytes
}
