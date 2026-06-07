package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockermount "github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockervolume "github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
)

type dockerRuntime struct {
	client *dockerclient.Client
}

// Compile-time interface check.
var _ ContainerRuntime = (*dockerRuntime)(nil)

// NewDocker creates a Docker runtime using the Moby SDK client.
func NewDocker(ctx context.Context) (ContainerRuntime, error) {
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
	if err := d.client.VolumeRemove(ctx, name, force); err != nil {
		return fmt.Errorf("removing volume: %w", err)
	}
	return nil
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
	if err := d.client.NetworkRemove(ctx, id); err != nil {
		return fmt.Errorf("removing network: %w", err)
	}
	return nil
}

func (d *dockerRuntime) ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error) {
	containerConfig := &dockercontainer.Config{
		Image:        spec.Image,
		Cmd:          spec.Cmd,
		Env:          spec.Env,
		User:         spec.User,
		WorkingDir:   spec.WorkingDir,
		Tty:          spec.Tty,
		OpenStdin:    spec.OpenStdin,
		AttachStdin:  spec.OpenStdin,
		AttachStdout: true,
		AttachStderr: true,
	}

	mounts := make([]dockermount.Mount, 0, len(spec.Binds))
	for _, bind := range spec.Binds {
		src, dst, opts := parseBind(bind)

		// Named volumes (no '/') use TypeVolume; host paths use TypeBind.
		mountType := dockermount.TypeBind
		if !strings.Contains(src, "/") {
			mountType = dockermount.TypeVolume
		}

		mount := dockermount.Mount{
			Type:   mountType,
			Source: src,
			Target: dst,
		}
		if mountType == dockermount.TypeVolume {
			mount.VolumeOptions = &dockermount.VolumeOptions{NoCopy: true}
		}
		for _, opt := range opts {
			if opt == "ro" {
				mount.ReadOnly = true
			}
			// "z" and "Z" are SELinux relabeling flags handled by
			// the Docker daemon; no API field needed.
		}
		mounts = append(mounts, mount)
	}

	securityOpt, err := normalizeSecurityOpt(spec.SecurityOpt)
	if err != nil {
		return "", err
	}

	hostConfig := &dockercontainer.HostConfig{
		Mounts:      mounts,
		CapDrop:     spec.CapDrop,
		CapAdd:      spec.CapAdd,
		SecurityOpt: securityOpt,
		AutoRemove:  spec.AutoRemove,
	}
	if spec.Init {
		hostConfig.Init = new(true)
	}
	if spec.PidsLimit > 0 {
		hostConfig.PidsLimit = new(spec.PidsLimit)
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

func normalizeSecurityOpt(opts []string) ([]string, error) {
	normalized := make([]string, 0, len(opts))
	for _, opt := range opts {
		profile, ok := strings.CutPrefix(opt, "seccomp=")
		if !ok || profile == "unconfined" || strings.HasPrefix(strings.TrimSpace(profile), "{") {
			normalized = append(normalized, opt)
			continue
		}

		data, err := os.ReadFile(profile)
		if err != nil {
			return nil, fmt.Errorf("reading seccomp profile %s: %w", profile, err)
		}
		normalized = append(normalized, "seccomp="+string(data))
	}
	return normalized, nil
}

func (d *dockerRuntime) ContainerStart(ctx context.Context, id string) error {
	if err := d.client.ContainerStart(ctx, id, dockercontainer.StartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}
	return nil
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
	if err := d.client.ContainerRemove(ctx, id, dockercontainer.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	return nil
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

func (d *dockerRuntime) ContainerResize(ctx context.Context, id string, height, width uint) error {
	if err := d.client.ContainerResize(ctx, id, dockercontainer.ResizeOptions{Height: height, Width: width}); err != nil {
		return fmt.Errorf("resizing container: %w", err)
	}
	return nil
}

func (d *dockerRuntime) ContainerSignal(ctx context.Context, id, signal string) error {
	if err := d.client.ContainerKill(ctx, id, signal); err != nil {
		return fmt.Errorf("signaling container: %w", err)
	}
	return nil
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

	return waitForExecResult(ctx, func(ctx context.Context) (execInspectResult, error) {
		resp, err := d.client.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			return execInspectResult{}, fmt.Errorf("inspecting exec: %w", err)
		}
		return execInspectResult{Running: resp.Running, ExitCode: int64(resp.ExitCode)}, nil
	}, 100*time.Millisecond)
}

type execInspectResult struct {
	Running  bool
	ExitCode int64
}

func waitForExecResult(ctx context.Context, inspect func(context.Context) (execInspectResult, error), interval time.Duration) (int64, error) {
	for {
		select {
		case <-ctx.Done():
			return -1, fmt.Errorf("waiting for exec completion: %w", ctx.Err())
		default:
		}

		result, err := inspect(ctx)
		if err != nil {
			return -1, err
		}
		if !result.Running {
			return result.ExitCode, nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return -1, fmt.Errorf("waiting for exec completion: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func (d *dockerRuntime) CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error {
	if err := d.client.CopyToContainer(ctx, id, dstPath, content, dockercontainer.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copy to container: %w", err)
	}
	return nil
}

func (d *dockerRuntime) CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error) {
	reader, _, err := d.client.CopyFromContainer(ctx, id, srcPath)
	if err != nil {
		return nil, fmt.Errorf("copy from container %s: %w", id, err)
	}
	return reader, nil
}

func (d *dockerRuntime) ImagePull(ctx context.Context, ref string, out io.Writer) error {
	resp, err := d.client.ImagePull(ctx, ref, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", ref, err)
	}
	if err := drainImagePullStream(resp, out); err != nil {
		return fmt.Errorf("reading image pull stream for %s: %w", ref, err)
	}
	return nil
}

func drainImagePullStream(resp io.ReadCloser, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if _, err := io.Copy(out, resp); err != nil {
		_ = resp.Close()
		return fmt.Errorf("copying image pull stream: %w", err)
	}
	if err := resp.Close(); err != nil {
		return fmt.Errorf("closing image pull stream: %w", err)
	}
	return nil
}

func (d *dockerRuntime) ImageExists(ctx context.Context, ref string) (bool, error) {
	_, err := d.client.ImageInspect(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting image: %w", err)
	}
	return true, nil
}

// parseBind splits a bind string (src:dst[:opts]) into components.
func parseBind(bind string) (src, dst string, opts []string) {
	parts := strings.SplitN(bind, ":", 3)
	src = parts[0]
	dst = src
	if len(parts) >= 2 {
		dst = parts[1]
	}
	if len(parts) >= 3 {
		opts = strings.Split(parts[2], ",")
	}
	return src, dst, opts
}
