package runtimetest

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/r-dson/abox/internal/runtime"
)

// StubRuntime satisfies runtime.ContainerRuntime with no-op defaults for tests.
type StubRuntime struct {
	CloseFn             func() error
	VolumeCreateFn      func(ctx context.Context, name string, labels map[string]string) error
	VolumeRemoveFn      func(ctx context.Context, name string, force bool) error
	NetworkCreateFn     func(ctx context.Context, name string, internal bool) (string, error)
	NetworkRemoveFn     func(ctx context.Context, id string) error
	ContainerCreateFn   func(ctx context.Context, spec runtime.ContainerSpec) (string, error)
	ContainerStartFn    func(ctx context.Context, id string) error
	ContainerWaitFn     func(ctx context.Context, id string) (int64, error)
	ContainerRemoveFn   func(ctx context.Context, id string, force bool) error
	ContainerAttachFn   func(ctx context.Context, id string) (io.ReadWriteCloser, error)
	ContainerResizeFn   func(ctx context.Context, id string, height, width uint) error
	ContainerSignalFn   func(ctx context.Context, id, signal string) error
	ContainerExecFn     func(ctx context.Context, id string, cmd []string) (int64, error)
	CopyToContainerFn   func(ctx context.Context, id, dstPath string, content io.Reader) error
	CopyFromContainerFn func(ctx context.Context, id, srcPath string) (io.ReadCloser, error)
	ImagePullFn         func(ctx context.Context, ref string, out io.Writer) error
	ImageExistsFn       func(ctx context.Context, ref string) (bool, error)
}

func (s *StubRuntime) Close() error {
	if s.CloseFn != nil {
		return s.CloseFn()
	}
	return nil
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

func (s *StubRuntime) ContainerCreate(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
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
	return nopReadWriteCloser{}, nil
}

func (s *StubRuntime) ContainerResize(ctx context.Context, id string, height, width uint) error {
	if s.ContainerResizeFn != nil {
		return s.ContainerResizeFn(ctx, id, height, width)
	}
	return nil
}

func (s *StubRuntime) ContainerSignal(ctx context.Context, id, signal string) error {
	if s.ContainerSignalFn != nil {
		return s.ContainerSignalFn(ctx, id, signal)
	}
	return nil
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
	if _, err := io.Copy(io.Discard, content); err != nil {
		return fmt.Errorf("draining copy-to-container content: %w", err)
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

type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }
