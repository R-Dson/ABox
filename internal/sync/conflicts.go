package sync

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/r-dson/abox/internal/config"
)

// MtimeSnapshot records file modification times for conflict detection.
// Mirrors snapshot_mtimes() from the Bash implementation but stores
// in a Go struct instead of /tmp/abx_mtimes_$VOL_ID.
type MtimeSnapshot struct {
	entries map[string]os.FileInfo
}

// SnapshotMtimes records mtimes for all files under the given directories.
// Non-existent directories are silently skipped.
func SnapshotMtimes(dirs []string) (*MtimeSnapshot, error) {
	snap := &MtimeSnapshot{entries: make(map[string]os.FileInfo)}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			snap.entries[path] = info
			return nil
		})
	}

	return snap, nil
}

// SnapshotMtimesFromProfile creates a snapshot of all editor data directories
// derived from the EditorProfile (config, cache, state, share).
func SnapshotMtimesFromProfile(profile config.EditorProfile, home string) (*MtimeSnapshot, error) {
	dirs := []string{
		profile.ConfigFullPath(home),
		profile.CachePath(home),
		profile.StatePath(home),
		profile.SharePath(home),
	}
	return SnapshotMtimes(dirs)
}

// DetectConflicts returns paths of files whose mtime changed since the snapshot.
// Deleted files are not conflicts — they're simply gone.
func (s *MtimeSnapshot) DetectConflicts() []string {
	var conflicts []string

	for path, origInfo := range s.entries {
		currentInfo, err := os.Stat(path)
		if err != nil {
			// File deleted — not a conflict
			continue
		}
		if currentInfo.ModTime() != origInfo.ModTime() {
			conflicts = append(conflicts, path)
		}
	}

	sort.Strings(conflicts)
	return conflicts
}
