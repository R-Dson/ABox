package sync_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/r-dson/abox/internal/config"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSnapshotMtimes(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)

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
	mustWriteFile(t, filepath.Join(dir, "stable.txt"), []byte("unchanged"), 0o644)

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})
	conflicts := snap.DetectConflicts()

	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d: %v", len(conflicts), conflicts)
	}
}

func TestDetectConflicts_FileModified(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "changed.txt")
	mustWriteFile(t, file, []byte("original"), 0o644)

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})

	time.Sleep(10 * time.Millisecond)
	mustWriteFile(t, file, []byte("modified"), 0o644)

	conflicts := snap.DetectConflicts()

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %v", len(conflicts), conflicts)
	}
	if conflicts[0] != file {
		t.Errorf("conflict path = %q, want %q", conflicts[0], file)
	}
}

func TestDetectConflicts_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deleted.txt")
	mustWriteFile(t, file, []byte("temporary"), 0o644)

	snap, _ := syncpkg.SnapshotMtimes([]string{dir})

	os.Remove(file)

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
	mustWriteFile(t, filepath.Join(dir1, "a.txt"), []byte("a"), 0o644)
	mustWriteFile(t, filepath.Join(dir2, "b.txt"), []byte("b"), 0o644)

	snap, _ := syncpkg.SnapshotMtimes([]string{dir1, dir2})

	time.Sleep(10 * time.Millisecond)
	mustWriteFile(t, filepath.Join(dir1, "a.txt"), []byte("modified"), 0o644)

	conflicts := snap.DetectConflicts()
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict from multi-dir snapshot, got %d", len(conflicts))
	}
}

func TestSnapshotMtimes_UsesProfile(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")

	home := t.TempDir()
	mustMkdirAll(t, profile.ConfigFullPath(home))
	mustWriteFile(t, filepath.Join(profile.ConfigFullPath(home), "settings.json"), []byte("{}"), 0o644)

	snap, err := syncpkg.SnapshotMtimesFromProfile(profile, home)
	if err != nil {
		t.Fatalf("SnapshotMtimesFromProfile() error: %v", err)
	}
	if snap == nil {
		t.Fatal("SnapshotMtimesFromProfile() returned nil")
	}
}
