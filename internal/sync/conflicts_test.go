package sync_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/r-dson/abox/internal/config"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func helperWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func helperMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestSnapshotMtimes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	helperWriteFile(t, file, []byte("hello"))

	snap, err := syncpkg.SnapshotMtimes([]string{dir})
	if err != nil {
		t.Fatalf("SnapshotMtimes() error: %v", err)
	}
	if snap == nil {
		t.Fatal("SnapshotMtimes() returned nil")
	}
}

func TestDetectConflicts_NoChanges(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stable.txt")
	helperWriteFile(t, file, []byte("unchanged"))

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})
	conflicts := snap.DetectConflicts()

	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d: %v", len(conflicts), conflicts)
	}
}

func TestDetectConflicts_FileModified(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "changed.txt")
	helperWriteFile(t, file, []byte("original"))

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})

	// Modify the file after snapshot
	time.Sleep(10 * time.Millisecond)
	helperWriteFile(t, file, []byte("modified"))

	conflicts := snap.DetectConflicts()

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %v", len(conflicts), conflicts)
	}
	// The conflict path should contain the filename
	if conflicts[0] != file {
		t.Errorf("conflict path = %q, want %q", conflicts[0], file)
	}
}

func TestDetectConflicts_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deleted.txt")
	helperWriteFile(t, file, []byte("temporary"))

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})

	os.Remove(file)

	// Deleted files are not conflicts — they're gone
	conflicts := snap.DetectConflicts()
	if len(conflicts) != 0 {
		t.Errorf("deleted files should not be conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflicts_SkipsNonexistentDir(t *testing.T) {
	snap, err := syncpkg.SnapshotMtimes([]string{"/nonexistent/path/abc"})
	if err != nil {
		t.Fatalf("SnapshotMtimes with nonexistent dir should not error: %v", err)
	}
	conflicts := snap.DetectConflicts()
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for empty snapshot, got %d", len(conflicts))
	}
}

func TestSnapshotMtimes_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	helperWriteFile(t, filepath.Join(dir1, "a.txt"), []byte("a"))
	helperWriteFile(t, filepath.Join(dir2, "b.txt"), []byte("b"))

	snap, _ := syncpkg.SnapshotMtimes([]string{dir1, dir2})

	time.Sleep(10 * time.Millisecond)
	helperWriteFile(t, filepath.Join(dir1, "a.txt"), []byte("modified"))

	conflicts := snap.DetectConflicts()
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict from multi-dir snapshot, got %d", len(conflicts))
	}
}

func TestSnapshotMtimes_UsesProfile(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")

	home := t.TempDir()
	helperMkdirAll(t, profile.ConfigFullPath(home))
	helperWriteFile(t, filepath.Join(profile.ConfigFullPath(home), "settings.json"), []byte("{}"))

	snap, err := syncpkg.SnapshotMtimesFromProfile(profile, home)
	if err != nil {
		t.Fatalf("SnapshotMtimesFromProfile() error: %v", err)
	}
	if snap == nil {
		t.Fatal("SnapshotMtimesFromProfile() returned nil")
	}
}
