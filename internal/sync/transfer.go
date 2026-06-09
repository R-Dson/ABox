package sync

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	containerpkg "github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtime"
)

const volumeInitializedMarker = ".abx-volume-initialized"

// SyncIn transfers files from a host directory to a container volume.
// If matcher is non-nil, files matching the exclusion patterns are skipped.
// Missing sources initialize an empty writable volume.
func In(ctx context.Context, rt runtime.ContainerRuntime, srcDir, volumeName, dstPath string, matcher *exclusion.Matcher) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		slog.DebugContext(ctx, "sync source does not exist, initializing empty volume", "path", srcDir)
		return initializeEmptyVolume(ctx, rt, volumeName, dstPath)
	}

	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	stagingPath := path.Join(dstPath, ".abx-tmp")
	if err := execSuccessful(ctx, rt, containerID,
		[]string{"sh", "-c", "rm -rf " + stagingPath + " && mkdir -p " + stagingPath},
		"preparing staging path in volume"); err != nil {
		return err
	}

	// Stream tar via pipe; CloseWithError propagates tar errors to the reader side.
	pr, pw := io.Pipe()
	tarDone := make(chan error, 1)
	go func() {
		if err := TarFiltered(srcDir, pw, matcher); err != nil {
			_ = pw.CloseWithError(err)
			tarDone <- err
			return
		}
		if err := pw.Close(); err != nil {
			tarDone <- fmt.Errorf("closing tar pipe writer: %w", err)
			return
		}
		tarDone <- nil
	}()

	if err := rt.CopyToContainer(ctx, containerID, stagingPath, pr); err != nil {
		_ = pr.CloseWithError(err)
		<-tarDone
		return fmt.Errorf("streaming %s to volume: %w", srcDir, err)
	}
	if err := <-tarDone; err != nil {
		return fmt.Errorf("creating tar stream for %s: %w", srcDir, err)
	}

	replaceCmd := fmt.Sprintf("find %[1]s -mindepth 1 -maxdepth 1 ! -name .abx-tmp -exec rm -rf {} + && cp -a %[2]s/. %[1]s/ && rm -rf %[2]s && touch %[1]s/%[5]s && chown -R %[3]d:%[4]d %[1]s", dstPath, stagingPath, os.Getuid(), os.Getgid(), volumeInitializedMarker)
	if err := execSuccessful(ctx, rt, containerID, []string{"sh", "-c", replaceCmd}, "replacing volume contents"); err != nil {
		return err
	}

	return nil
}

func initializeEmptyVolume(ctx context.Context, rt runtime.ContainerRuntime, volumeName, dstPath string) error {
	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	cmd := fmt.Sprintf("mkdir -p %[1]s && touch %[1]s/%[4]s && chown -R %[2]d:%[3]d %[1]s", dstPath, os.Getuid(), os.Getgid(), volumeInitializedMarker)
	return execSuccessful(ctx, rt, containerID, []string{"sh", "-c", cmd}, "initializing empty volume")
}

func execSuccessful(ctx context.Context, rt runtime.ContainerRuntime, containerID string, cmd []string, purpose string) error {
	code, err := rt.ContainerExec(ctx, containerID, cmd)
	if err != nil {
		return fmt.Errorf("%s: %w", purpose, err)
	}
	if code != 0 {
		return fmt.Errorf("%s exited with code %d", purpose, code)
	}
	return nil
}

// SyncOut transfers files from a container volume to a host directory.
// It copies a tar archive from the container, then extracts it to destDir.
// Skips if destDir doesn't exist.
func Out(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destDir string) error {
	return OutWithOptions(ctx, rt, volumeName, srcPath, destDir, Options{})
}

// OutWithOptions transfers files from a container volume to a host directory using opts.
func OutWithOptions(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destDir string, opts Options) error {
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		slog.DebugContext(ctx, "sync dest does not exist, skipping", "path", destDir)
		return nil
	}
	if opts.Snapshot != nil && !opts.ForceSync {
		conflicts, err := outgoingConflicts(ctx, rt, volumeName, srcPath, opts)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			return &ConflictError{Conflicts: conflicts}
		}
	}
	return out(ctx, rt, volumeName, srcPath, destDir, opts)
}

// OutFile transfers a single-file archive from a container volume to a host file.
// Unlike Out, it creates the destination file when its parent directory exists.
func OutFile(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destFile string) error {
	return OutFileWithOptions(ctx, rt, volumeName, srcPath, destFile, Options{})
}

// OutFileWithOptions transfers a single-file archive from a container volume to a host file.
// Options are accepted for API symmetry with OutWithOptions; single-file sync-out does not use them yet.
func OutFileWithOptions(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destFile string, _ Options) error {
	if err := os.MkdirAll(filepath.Dir(destFile), 0o755); err != nil {
		return fmt.Errorf("creating file destination parent: %w", err)
	}
	return outFile(ctx, rt, volumeName, srcPath, destFile)
}

func outFile(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destFile string) error {
	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	expectedName := filepath.Base(destFile)
	stream, err := rt.CopyFromContainer(ctx, containerID, path.Join(srcPath, expectedName))
	if err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}
	defer stream.Close()

	if err := extractTarToFile(stream, destFile, expectedName); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}

	return nil
}

func outgoingConflicts(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath string, opts Options) ([]RootConflict, error) {
	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return nil, fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	stream, err := rt.CopyFromContainer(ctx, containerID, directoryContentsPath(srcPath))
	if err != nil {
		return nil, fmt.Errorf("copying from container for conflict check: %w", err)
	}
	defer stream.Close()

	manifest, err := TarManifest(stream, opts)
	if err != nil {
		return nil, fmt.Errorf("building outgoing manifest: %w", err)
	}
	conflicts, err := DetectRootConflicts(ctx, *opts.Snapshot, manifest)
	if err != nil {
		return nil, fmt.Errorf("detecting sync conflicts: %w", err)
	}
	return conflicts, nil
}

func out(ctx context.Context, rt runtime.ContainerRuntime, volumeName, srcPath, destPath string, opts Options) error {
	containerID, cleanup, err := mountVolumeContainer(ctx, rt, volumeName)
	if err != nil {
		return fmt.Errorf("mounting volume %s: %w", volumeName, err)
	}
	defer cleanup()

	stream, err := rt.CopyFromContainer(ctx, containerID, directoryContentsPath(srcPath))
	if err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}
	defer stream.Close()

	if err := extractTar(stream, destPath, opts); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}

	return nil
}

func directoryContentsPath(srcPath string) string {
	return strings.TrimRight(srcPath, "/") + "/."
}

// TarManifest returns the archive paths that would be written after matcher/internal filtering.
// This function only reads headers and builds an in-memory map — it never writes to the filesystem.
//
//nolint:gosec // G305: TarManifest is read-only; no filesystem writes from archive entries.
func TarManifest(r io.Reader, opts Options) (map[string]struct{}, error) {
	manifest := make(map[string]struct{})
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return manifest, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		// Sanitize: skip entries with path traversal or absolute paths.
		if strings.Contains(header.Name, "..") || filepath.IsAbs(header.Name) {
			continue
		}
		entryName := cleanArchiveEntryName(header.Name)
		if entryName == "." || entryName == volumeInitializedMarker {
			continue
		}
		if opts.Matcher != nil && opts.Matcher.Match(entryName) {
			continue
		}
		manifest[entryName] = struct{}{}
	}
}

// mountVolumeContainer creates a short-lived container with the volume mounted.
// Returns the container ID and a cleanup function.
func mountVolumeContainer(ctx context.Context, rt runtime.ContainerRuntime, volumeName string) (string, func(), error) {
	spec := runtime.ContainerSpec{
		Image:      runtime.SyncImage,
		Cmd:        []string{"sleep", "300"},
		AutoRemove: false,
		Binds:      []string{volumeName + ":/data:z"},
	}
	if err := containerpkg.ApplyHelperSecurityDefaults(&spec); err != nil {
		return "", nil, err
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

	rootInfo, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if !rootInfo.IsDir() {
		if matcher != nil && matcher.Match(filepath.ToSlash(filepath.Base(dir))) {
			return nil
		}
		if err := writeTarFile(tw, dir, filepath.Base(dir)); err != nil {
			return err
		}
		if err := tw.Close(); err != nil {
			return fmt.Errorf("closing tar writer: %w", err)
		}
		return nil
	}

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

		if rel == volumeInitializedMarker {
			return nil
		}

		if matcher != nil && matcher.Match(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		return writeTarFile(tw, path, rel)
	}); err != nil {
		_ = tw.Close()
		return fmt.Errorf("walking directory: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}
	return nil
}

func writeTarFile(tw *tar.Writer, filePath, name string) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("file info: %w", err)
	}

	if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		slog.Debug("skipping special file during tar", "path", filePath, "mode", info.Mode())
		return nil
	}

	linkTarget := ""
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err = os.Readlink(filePath)
		if err != nil {
			return fmt.Errorf("read symlink: %w", err)
		}
	}

	header, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return fmt.Errorf("tar header: %w", err)
	}
	header.Name = filepath.ToSlash(name)

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("copy file content: %w", err)
	}
	return nil
}

func extractTarToFile(r io.Reader, dest, expectedName string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("archive contains no regular file")
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != expectedName {
			return fmt.Errorf("archive file %q does not match expected %q", header.Name, expectedName)
		}
		if err := ensureFileIsNotSymlink(dest); err != nil {
			return err
		}

		file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		if _, err := io.Copy(file, tr); err != nil {
			_ = file.Close()
			return fmt.Errorf("write %s: %w", dest, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", dest, err)
		}
		return nil
	}
}

// extractTar extracts a tar archive to the destination directory or file.
// Validates that directory extractions stay within dest to prevent path traversal.
func extractTar(r io.Reader, dest string, opts Options) error {
	destInfo, err := os.Lstat(dest)
	if err != nil {
		return fmt.Errorf("stat destination: %w", err)
	}
	if destInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination root is a symlink: %s", dest)
	}
	if !destInfo.IsDir() {
		return extractTarToFile(r, dest, filepath.Base(dest))
	}

	cleanDest := filepath.Clean(dest)
	resolvedDest, err := filepath.EvalSymlinks(cleanDest)
	if err != nil {
		return fmt.Errorf("resolve destination root %s: %w", cleanDest, err)
	}

	manifest := make(map[string]struct{})
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Sanitize the archive entry name: reject path traversal before any filesystem use.
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("tar entry %q escapes destination", header.Name)
		}
		if filepath.IsAbs(header.Name) {
			continue
		}

		entryName := cleanArchiveEntryName(header.Name)
		if entryName == "" || entryName == "." || entryName == volumeInitializedMarker {
			continue
		}
		if opts.Matcher != nil && opts.Matcher.Match(entryName) {
			continue
		}

		manifest[entryName] = struct{}{}

		target := filepath.Join(cleanDest, filepath.FromSlash(entryName))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensurePathHasNoSymlink(cleanDest, target); err != nil {
				return err
			}
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := ensurePathHasNoSymlink(cleanDest, filepath.Dir(target)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			if err := ensurePathHasNoSymlink(cleanDest, target); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return fmt.Errorf("write %s: %w", target, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", target, err)
			}
		case tar.TypeSymlink:
			// Resolve symlinks in the parent directory to detect previously-extracted
			// symlinks that could redirect the new link outside the root.
			resolvedParent, evalErr := filepath.EvalSymlinks(filepath.Dir(target))
			if evalErr != nil {
				resolvedParent = filepath.Dir(target)
			}
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("symlink %s has absolute target %q outside destination root", target, header.Linkname)
			}
			if strings.Contains(header.Linkname, "..") {
				return fmt.Errorf("symlink %s target %q escapes destination", target, header.Linkname)
			}
			// Verify the symlink target stays within the resolved destination root.
			effectiveTarget := filepath.Clean(filepath.Join(resolvedParent, header.Linkname))
			realTarget, evalErr := filepath.EvalSymlinks(effectiveTarget)
			if evalErr == nil {
				effectiveTarget = realTarget
			}
			relTarget, relErr := filepath.Rel(resolvedDest, effectiveTarget)
			if relErr != nil || relTarget == ".." || strings.HasPrefix(relTarget, ".."+string(filepath.Separator)) {
				return fmt.Errorf("symlink %s points outside destination root: %q", target, header.Linkname)
			}
			if err := ensurePathHasNoSymlink(cleanDest, filepath.Dir(target)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("create symlink %s: %w", target, err)
			}
		}
	}
	if opts.DeleteMissing && opts.Snapshot != nil {
		if err := reconcileMissingEntries(cleanDest, *opts.Snapshot, manifest, opts.Matcher); err != nil {
			return err
		}
	}
	return nil
}

func reconcileMissingEntries(root string, snapshot RootSnapshot, manifest map[string]struct{}, matcher *exclusion.Matcher) error {
	paths := make([]string, 0, len(snapshot.Entries))
	for rel := range snapshot.Entries {
		if _, ok := manifest[rel]; ok {
			continue
		}
		if matcher != nil && matcher.Match(rel) {
			continue
		}
		paths = append(paths, rel)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], "/") > strings.Count(paths[j], "/")
	})
	for _, rel := range paths {
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := ensurePathStaysInRoot(root, target, rel); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("checking missing sync path %s: %w", target, err)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove missing sync path %s: %w", target, err)
		}
	}
	return nil
}

func cleanArchiveEntryName(name string) string {
	return filepath.ToSlash(path.Clean(name))
}

func ensureFileIsNotSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlink %s", path)
	}
	return nil
}

func ensurePathStaysInRoot(root, target, entryName string) error {
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil {
		return fmt.Errorf("checking tar entry %q: %w", entryName, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("tar entry %q escapes destination", entryName)
	}
	return nil
}

func ensurePathHasNoSymlink(root, target string) error {
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil {
		return fmt.Errorf("checking symlink path: %w", err)
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("checking %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink %s", current)
		}
	}
	return nil
}
