package sync

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtime"
)

// SyncIn transfers files from a host directory to a container volume.
// If matcher is non-nil, files matching the exclusion patterns are skipped.
// Skips if the source directory doesn't exist.
func In(ctx context.Context, rt runtime.ContainerRuntime, srcDir, volumeName, dstPath string, matcher *exclusion.Matcher) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		slog.DebugContext(ctx, "sync source does not exist, skipping", "path", srcDir)
		return nil
	}

	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	// Stream tar via pipe; CloseWithError propagates tar errors to the reader side.
	pr, pw := io.Pipe()
	go func() {
		if err := TarFiltered(srcDir, pw, matcher); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.Close()
	}()

	stagingPath := dstPath + ".abx-tmp"
	if err := rt.CopyToContainer(ctx, containerID, stagingPath, pr); err != nil {
		return fmt.Errorf("streaming %s to volume: %w", srcDir, err)
	}

	// Atomic rename inside the container
	if _, err := rt.ContainerExec(ctx, containerID,
		[]string{"mv", "-T", stagingPath, dstPath}); err != nil {
		return fmt.Errorf("atomic rename in volume: %w", err)
	}

	return nil
}

// SyncOut transfers files from a container volume to a host directory.
// It copies a tar archive from the container, then extracts it to destDir.
// Skips if destDir doesn't exist.
func Out(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destDir string) error {
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		slog.DebugContext(ctx, "sync dest does not exist, skipping", "path", destDir)
		return nil
	}

	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	stream, err := rt.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}
	defer stream.Close()

	if err := extractTar(stream, destDir); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}

	return nil
}

// mountVolumeContainer creates a short-lived container with the volume mounted.
// Returns the container ID and a cleanup function.
func mountVolumeContainer(ctx context.Context, rt runtime.ContainerRuntime, volumeName string) (string, func(), error) {
	spec := runtime.ContainerSpec{
		Image:      runtime.SyncImage,
		Cmd:        []string{"sleep", "300"},
		AutoRemove: false,
		Binds:      []string{volumeName + ":/data"},
		CapDrop:    []string{"ALL"},
		CapAdd:     []string{"CHOWN"},
	}

	id, err := rt.ContainerCreate(ctx, spec)
	if err != nil {
		return "", nil, fmt.Errorf("creating sync container: %w", err)
	}

	if err := rt.ContainerStart(ctx, id); err != nil {
		_ = rt.ContainerRemove(ctx, id, true)
		return "", nil, fmt.Errorf("starting sync container: %w", err)
	}

	cleanup := func() {
		_ = rt.ContainerRemove(context.Background(), id, true)
	}

	return id, cleanup, nil
}

// TarFiltered writes a tar archive of the directory contents to the writer,
// excluding files and directories that match the exclusion patterns.
// If matcher is nil, all files are included.
func TarFiltered(dir string, w io.Writer, matcher *exclusion.Matcher) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}
		rel = filepath.ToSlash(rel)

		// Skip the root directory itself
		if rel == "." {
			return nil
		}

		if matcher != nil && matcher.Match(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("file info: %w", infoErr)
		}

		header, hErr := tar.FileInfoHeader(info, "")
		if hErr != nil {
			return fmt.Errorf("tar header: %w", hErr)
		}
		header.Name = rel

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write header: %w", err)
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Errorf("open file: %w", openErr)
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

// extractTar extracts a tar archive to the destination directory.
// Validates that all extracted paths stay within dest to prevent path traversal.
func extractTar(r io.Reader, dest string) error {
	cleanDest := filepath.Clean(dest)
	if !strings.HasSuffix(cleanDest, string(os.PathSeparator)) {
		cleanDest += string(os.PathSeparator)
	}

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(dest, header.Name)

		// Path traversal check: target must be under dest
		if !strings.HasPrefix(filepath.Clean(target), cleanDest) {
			return fmt.Errorf("tar entry %q escapes destination", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("write %s: %w", target, err)
			}
			f.Close()
		}
	}
	return nil
}
