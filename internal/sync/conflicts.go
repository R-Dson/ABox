package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/r-dson/abox/internal/exclusion"
)

// RootSpec identifies a host root to snapshot for sync conflict detection.
type RootSpec struct {
	Name    string
	Path    string
	Matcher *exclusion.Matcher
}

// Snapshot records host state for one or more sync roots.
type Snapshot struct {
	Roots []RootSnapshot
}

// RootSnapshot records host state for one sync root.
type RootSnapshot struct {
	Name    string
	Path    string
	Entries map[string]RootEntry
}

// RootEntry records comparable filesystem metadata for a path in a root.
type RootEntry struct {
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	Exists  bool
}

// ConflictKind classifies a host-side conflict that would be overwritten by sync-out.
type ConflictKind string

const (
	// ConflictHostCreated means the host created a path that also exists in the outgoing archive.
	ConflictHostCreated ConflictKind = "host-created"
	// ConflictHostDeleted means the host deleted a path that exists in the outgoing archive.
	ConflictHostDeleted ConflictKind = "host-deleted"
	// ConflictHostModified means the host changed a tracked path that exists in the outgoing archive.
	ConflictHostModified ConflictKind = "host-modified"
)

// RootConflict describes one conflict in a sync root.
type RootConflict struct {
	Root   string
	Path   string
	Kind   ConflictKind
	Detail string
}

// ConflictError reports sync-out conflicts for one root.
type ConflictError struct {
	Conflicts []RootConflict
}

func (e *ConflictError) Error() string {
	summary, _ := FormatRootConflicts(e.Conflicts)
	if summary == "" {
		return "sync conflicts"
	}
	return summary
}

// SnapshotRoots records host metadata for all files and directories under the given roots.
// Non-existent roots are represented as empty snapshots so later host-created paths can be detected.
func SnapshotRoots(ctx context.Context, specs []RootSpec) (*Snapshot, error) {
	snap := &Snapshot{Roots: make([]RootSnapshot, 0, len(specs))}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("snapshot roots: %w", err)
		}
		rootSnap, err := snapshotRoot(ctx, spec)
		if err != nil {
			return nil, err
		}
		snap.Roots = append(snap.Roots, rootSnap)
	}
	return snap, nil
}

func snapshotRoot(ctx context.Context, spec RootSpec) (RootSnapshot, error) {
	root := RootSnapshot{
		Name:    spec.Name,
		Path:    spec.Path,
		Entries: make(map[string]RootEntry),
	}
	if _, err := os.Stat(spec.Path); err != nil {
		if os.IsNotExist(err) {
			return root, nil
		}
		return root, fmt.Errorf("stat root %s: %w", spec.Path, err)
	}

	if err := filepath.WalkDir(spec.Path, func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("snapshot canceled: %w", ctxErr)
		}
		if err != nil {
			return fmt.Errorf("walking %s: %w", path, err)
		}
		rel, relErr := filepath.Rel(spec.Path, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if rel == volumeInitializedMarker {
			return nil
		}
		if spec.Matcher != nil && spec.Matcher.Match(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("file info %s: %w", path, infoErr)
		}
		root.Entries[rel] = RootEntry{
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Exists:  true,
		}
		return nil
	}); err != nil {
		return root, fmt.Errorf("snapshotting %s: %w", spec.Path, err)
	}

	return root, nil
}

// DetectRootConflicts compares a root snapshot with current host state for paths in the outgoing archive.
func DetectRootConflicts(ctx context.Context, snap RootSnapshot, outgoing map[string]struct{}) ([]RootConflict, error) {
	paths := make([]string, 0, len(outgoing))
	for path := range outgoing {
		paths = append(paths, filepath.ToSlash(filepath.Clean(path)))
	}
	sort.Strings(paths)

	conflicts := make([]RootConflict, 0)
	for _, rel := range paths {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("detect root conflicts: %w", err)
		}
		if rel == "." || rel == volumeInitializedMarker {
			continue
		}
		original, tracked := snap.Entries[rel]
		current, err := os.Lstat(filepath.Join(snap.Path, filepath.FromSlash(rel)))
		if err != nil {
			if os.IsNotExist(err) {
				if tracked {
					conflicts = append(conflicts, RootConflict{Root: snap.Name, Path: rel, Kind: ConflictHostDeleted, Detail: "host path was deleted during session"})
				}
				continue
			}
			return nil, fmt.Errorf("stat current path %s: %w", rel, err)
		}
		if !tracked {
			conflicts = append(conflicts, RootConflict{Root: snap.Name, Path: rel, Kind: ConflictHostCreated, Detail: "host path was created during session"})
			continue
		}
		if rootEntryChanged(original, current) {
			conflicts = append(conflicts, RootConflict{Root: snap.Name, Path: rel, Kind: ConflictHostModified, Detail: "host path was modified during session"})
		}
	}
	return conflicts, nil
}

func rootEntryChanged(original RootEntry, current os.FileInfo) bool {
	return original.Size != current.Size() || original.Mode != current.Mode() || !original.ModTime.Equal(current.ModTime())
}

// FormatRootConflicts returns a single summary line and grouped detail for conflict reporting.
func FormatRootConflicts(conflicts []RootConflict) (summary string, detail string) {
	if len(conflicts) == 0 {
		return "", ""
	}

	summary = fmt.Sprintf("%d sync conflicts across %d roots", len(conflicts), countConflictRoots(conflicts))
	byRoot := make(map[string][]RootConflict)
	var roots []string
	for _, conflict := range conflicts {
		if _, ok := byRoot[conflict.Root]; !ok {
			roots = append(roots, conflict.Root)
		}
		byRoot[conflict.Root] = append(byRoot[conflict.Root], conflict)
	}
	sort.Strings(roots)

	var b strings.Builder
	b.WriteString("Conflicts by root:")
	for _, root := range roots {
		b.WriteString("\n  ")
		b.WriteString(root)
		rootConflicts := byRoot[root]
		sort.Slice(rootConflicts, func(i, j int) bool { return rootConflicts[i].Path < rootConflicts[j].Path })
		for _, conflict := range rootConflicts {
			b.WriteString("\n    ")
			b.WriteString(string(conflict.Kind))
			b.WriteString(": ")
			b.WriteString(conflict.Path)
		}
	}
	return summary, b.String()
}

func countConflictRoots(conflicts []RootConflict) int {
	seen := make(map[string]struct{})
	for _, conflict := range conflicts {
		seen[conflict.Root] = struct{}{}
	}
	return len(seen)
}

// MtimeSnapshot records file modification times for conflict detection.
type MtimeSnapshot struct {
	entries map[string]os.FileInfo
}

// SnapshotMtimes records mtimes for all files under the given directories.
// Non-existent directories are silently skipped.
// Returns an error if any directory walk fails due to permission or I/O errors.
func SnapshotMtimes(dirs []string) (*MtimeSnapshot, error) {
	snap := &MtimeSnapshot{entries: make(map[string]os.FileInfo)}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				// Propagate permission and I/O errors instead of silently skipping
				return fmt.Errorf("walking %s: %w", path, err)
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("file info %s: %w", path, err)
			}
			snap.entries[path] = info
			return nil
		}); err != nil {
			return nil, fmt.Errorf("snapshotting %s: %w", dir, err)
		}
	}

	return snap, nil
}

// DetectConflicts returns paths of files whose mtime changed since the snapshot.
// Deleted files are not conflicts — they're simply gone.
func (s *MtimeSnapshot) DetectConflicts() []string {
	var conflicts []string

	for path, origInfo := range s.entries {
		currentInfo, err := os.Stat(path)
		if err != nil {
			continue
		}
		if currentInfo.ModTime() != origInfo.ModTime() {
			conflicts = append(conflicts, path)
		}
	}

	sort.Strings(conflicts)
	return conflicts
}

// FormatConflicts returns a single summary line and a detailed multi-line string
// for conflict reporting. Returns ("", "") if no conflicts.
func FormatConflicts(conflicts []string) (summary string, detail string) {
	if len(conflicts) == 0 {
		return "", ""
	}

	summary = fmt.Sprintf("%d files modified during session", len(conflicts))

	if len(conflicts) <= 5 {
		detail = "Modified files:\n  " + strings.Join(conflicts, "\n  ")
	} else {
		detail = fmt.Sprintf("Modified files (first 5 of %d):\n  %s",
			len(conflicts), strings.Join(conflicts[:5], "\n  "))
	}

	return summary, detail
}
