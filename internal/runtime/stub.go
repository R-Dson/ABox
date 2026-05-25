package runtime

import (
	"bytes"
	"context"
	"io"
)

// StubRuntime is a test helper that satisfies ContainerRuntime with no-op defaults.
// Embed this struct in test-specific mocks and override only the function fields
// you need. This avoids duplicating 16 method stubs in every test package.
type StubRuntime struct {
	VolumeCreateFn      func(ctx context.Context, name string, labels map[string]string) error
	VolumeRemoveFn      func(ctx context.Context, name string, force bool) error
	NetworkCreateFn     func(ctx context.Context, name string, internal bool) (string, error)
	NetworkRemoveFn     func(ctx context.Context, id string) error
	ContainerCreateFn   func(ctx context.Context, spec ContainerSpec) (string, error)
	ContainerStartFn    func(ctx context.Context, id string) error
	ContainerWaitFn     func(ctx context.Context, id string) (int64, error)
	ContainerRemoveFn   func(ctx context.Context, id string, force bool) error
	ContainerAttachFn   func(ctx context.Context, id string) (io.ReadWriteCloser, error)
	ContainerExecFn     func(ctx context.Context, id string, cmd []string) (int64, error)
	CopyToContainerFn   func(ctx context.Context, id, dstPath string, content io.Reader) error
	CopyFromContainerFn func(ctx context.Context, id, srcPath string) (io.ReadCloser, error)
	ImagePullFn         func(ctx context.Context, ref string, out io.Writer) error
	ImageExistsFn       func(ctx context.Context, ref string) (bool, error)
	PingFn              func(ctx context.Context) error
}

func (s *StubRuntime) VolumeCreate(ctx context.Context, name string, labels map[string]string) error {
	if s.VolumeCreateFn != nil {
		return s.VolumeCreateFn(ctx, name, labels)
	}
	return nil
}

func (s *StubRuntime) VolumeRemove(ctx context.Context, name string, force bool) error {
	if s.VolumeRemoveFn != nil {
		return s.VolumeRemoveFn(ctx, name, force)
	}
	return nil
}

func (s *StubRuntime) NetworkCreate(ctx context.Context, name string, internal bool) (string, error) {
	if s.NetworkCreateFn != nil {
		return s.NetworkCreateFn(ctx, name, internal)
	}
	return "", nil
}

func (s *StubRuntime) NetworkRemove(ctx context.Context, id string) error {
	if s.NetworkRemoveFn != nil {
		return s.NetworkRemoveFn(ctx, id)
	}
	return nil
}

func (s *StubRuntime) ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error) {
	if s.ContainerCreateFn != nil {
		return s.ContainerCreateFn(ctx, spec)
	}
	return "c-0", nil
}

func (s *StubRuntime) ContainerStart(ctx context.Context, id string) error {
	if s.ContainerStartFn != nil {
		return s.ContainerStartFn(ctx, id)
	}
	return nil
}

func (s *StubRuntime) ContainerWait(ctx context.Context, id string) (int64, error) {
	if s.ContainerWaitFn != nil {
		return s.ContainerWaitFn(ctx, id)
	}
	return 0, nil
}

func (s *StubRuntime) ContainerRemove(ctx context.Context, id string, force bool) error {
	if s.ContainerRemoveFn != nil {
		return s.ContainerRemoveFn(ctx, id, force)
	}
	return nil
}

func (s *StubRuntime) ContainerAttach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	if s.ContainerAttachFn != nil {
		return s.ContainerAttachFn(ctx, id)
	}
	return nopRWC{}, nil
}

func (s *StubRuntime) ContainerExec(ctx context.Context, id string, cmd []string) (int64, error) {
	if s.ContainerExecFn != nil {
		return s.ContainerExecFn(ctx, id, cmd)
	}
	return 0, nil
}

func (s *StubRuntime) CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error {
	if s.CopyToContainerFn != nil {
		return s.CopyToContainerFn(ctx, id, dstPath, content)
	}
	return nil
}

func (s *StubRuntime) CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error) {
	if s.CopyFromContainerFn != nil {
		return s.CopyFromContainerFn(ctx, id, srcPath)
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (s *StubRuntime) ImagePull(ctx context.Context, ref string, out io.Writer) error {
	if s.ImagePullFn != nil {
		return s.ImagePullFn(ctx, ref, out)
	}
	return nil
}

func (s *StubRuntime) ImageExists(ctx context.Context, ref string) (bool, error) {
	if s.ImageExistsFn != nil {
		return s.ImageExistsFn(ctx, ref)
	}
	return false, nil
}

func (s *StubRuntime) Ping(ctx context.Context) error {
	if s.PingFn != nil {
		return s.PingFn(ctx)
	}
	return nil
}

type nopRWC struct{}

func (nopRWC) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopRWC) Write(_ []byte) (int, error) { return 0, nil }
func (nopRWC) Close() error                { return nil }
