package sync

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/r-dson/abox/internal/runtime"
)

// Syncer handles data transfer between host and container volumes.
type Syncer struct {
	rt runtime.ContainerRuntime
}

// NewSyncer creates a new Syncer with the given container runtime.
func NewSyncer(rt runtime.ContainerRuntime) *Syncer {
	return &Syncer{rt: rt}
}

// SyncIn transfers files from a host directory to a container volume.
// Skips if the source directory doesn't exist.
func (s *Syncer) SyncIn(ctx context.Context, srcDir, volumeName, dstPath string) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		slog.DebugContext(ctx, "sync source does not exist, skipping", "path", srcDir)
		return nil
	}

	// Mount the volume in a short-lived sync container
	containerID, cleanup, err := s.mountVolumeContainer(ctx, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	// Create a tar stream from the source directory
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := TarDir(srcDir, pw); err != nil {
			slog.WarnContext(ctx, "tar creation failed", "error", err)
		}
	}()

	stagingPath := dstPath + ".abx-tmp"
	if err := s.rt.CopyToContainer(ctx, containerID, stagingPath, pr); err != nil {
		return fmt.Errorf("streaming %s to volume: %w", srcDir, err)
	}

	// Atomic rename inside the container
	if _, err := s.rt.ContainerExec(ctx, containerID,
		[]string{"mv", "-T", stagingPath, dstPath}); err != nil {
		return fmt.Errorf("atomic rename in volume: %w", err)
	}

	return nil
}

// mountVolumeContainer creates a short-lived container with the volume mounted.
// Returns the container ID and a cleanup function.
func (s *Syncer) mountVolumeContainer(ctx context.Context, volumeName string) (string, func(), error) {
	spec := runtime.ContainerSpec{
		Image:      runtime.SyncImage,
		Cmd:        []string{"sleep", "300"},
		AutoRemove: false,
		Binds:      []string{volumeName + ":/data"},
		CapDrop:    []string{"ALL"},
		CapAdd:     []string{"CHOWN"},
	}

	id, err := s.rt.ContainerCreate(ctx, spec)
	if err != nil {
		return "", nil, fmt.Errorf("creating sync container: %w", err)
	}

	if err := s.rt.ContainerStart(ctx, id); err != nil {
		_ = s.rt.ContainerRemove(ctx, id, true)
		return "", nil, fmt.Errorf("starting sync container: %w", err)
	}

	cleanup := func() {
		_ = s.rt.ContainerRemove(context.Background(), id, true)
	}

	return id, cleanup, nil
}

// TarDir writes a tar archive of the directory contents to the writer.
// Files are stored with paths relative to dir.
func TarDir(dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("file info: %w", err)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("tar header: %w", err)
		}
		header.Name = rel

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write header: %w", err)
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("copy file content: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walking directory: %w", err)
	}
	return nil
}
