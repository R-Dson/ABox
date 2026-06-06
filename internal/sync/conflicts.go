package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
