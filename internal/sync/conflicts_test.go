package sync_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/r-dson/abox/internal/exclusion"
	syncpkg "github.com/r-dson/abox/internal/sync"
)

func TestSnapshotRoot_RecordsSizeMtimeModeAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nested", "file.txt")
	mustMkdirAll(t, filepath.Dir(file))
	mustWriteFile(t, file, []byte("hello"), 0o640)
	modTime := time.Now().Add(-time.Hour).Round(time.Second)
	setModTime(t, file, modTime)

	snap, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "workspace", Path: dir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	if len(snap.Roots) != 1 {
		t.Fatalf("snapshot roots = %d, want 1", len(snap.Roots))
	}
	root := snap.Roots[0]
	if root.Name != "workspace" {
		t.Fatalf("root name = %q, want workspace", root.Name)
	}
	entry, ok := root.Entries["nested/file.txt"]
	if !ok {
		t.Fatalf("missing relative entry nested/file.txt: %#v", root.Entries)
	}
	if entry.Size != int64(len("hello")) {
		t.Fatalf("entry size = %d, want %d", entry.Size, len("hello"))
	}
	if entry.Mode.Perm() != 0o640 {
		t.Fatalf("entry mode = %v, want 0640", entry.Mode.Perm())
	}
	if !entry.Exists {
		t.Fatal("entry Exists = false, want true")
	}
	if entry.ModTime.IsZero() {
		t.Fatal("entry ModTime is zero")
	}
}

func TestDetectRootConflicts_HostCreatedSameOutgoingPath(t *testing.T) {
	dir := t.TempDir()
	snap, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "workspace", Path: dir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	mustWriteFile(t, filepath.Join(dir, "new.txt"), []byte("host"), 0o644)

	conflicts, err := syncpkg.DetectRootConflicts(t.Context(), snap.Roots[0], map[string]struct{}{"new.txt": {}})
	if err != nil {
		t.Fatalf("DetectRootConflicts() error: %v", err)
	}
	assertSingleRootConflict(t, conflicts, "workspace", "new.txt", syncpkg.ConflictHostCreated)
}

func TestDetectRootConflicts_HostDeletedOutgoingPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "keep.txt"), []byte("host"), 0o644)
	snap, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "cache", Path: dir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("removing host file: %v", err)
	}

	conflicts, err := syncpkg.DetectRootConflicts(t.Context(), snap.Roots[0], map[string]struct{}{"keep.txt": {}})
	if err != nil {
		t.Fatalf("DetectRootConflicts() error: %v", err)
	}
	assertSingleRootConflict(t, conflicts, "cache", "keep.txt", syncpkg.ConflictHostDeleted)
}

func TestDetectRootConflicts_ModifiedUsesSizeOrMtime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "changed.txt")
	mustWriteFile(t, file, []byte("one"), 0o644)
	modTime := time.Now().Add(-time.Hour).Round(time.Second)
	setModTime(t, file, modTime)
	snap, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{Name: "state", Path: dir}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	mustWriteFile(t, file, []byte("longer"), 0o644)
	setModTime(t, file, modTime)

	conflicts, err := syncpkg.DetectRootConflicts(t.Context(), snap.Roots[0], map[string]struct{}{"changed.txt": {}})
	if err != nil {
		t.Fatalf("DetectRootConflicts() error: %v", err)
	}
	assertSingleRootConflict(t, conflicts, "state", "changed.txt", syncpkg.ConflictHostModified)
}

func TestSnapshotRoot_AppliesMatcher(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), []byte("secret"), 0o600)
	mustWriteFile(t, filepath.Join(dir, "visible.txt"), []byte("ok"), 0o644)

	snap, err := syncpkg.SnapshotRoots(t.Context(), []syncpkg.RootSpec{{
		Name:    "workspace",
		Path:    dir,
		Matcher: exclusion.NewMatcher([]string{".env"}),
	}})
	if err != nil {
		t.Fatalf("SnapshotRoots() error: %v", err)
	}
	if _, ok := snap.Roots[0].Entries[".env"]; ok {
		t.Fatal("excluded .env should not be snapshotted")
	}
	if _, ok := snap.Roots[0].Entries["visible.txt"]; !ok {
		t.Fatal("visible.txt should be snapshotted")
	}
}

func TestFormatRootConflictsGroupsByRoot(t *testing.T) {
	summary, detail := syncpkg.FormatRootConflicts([]syncpkg.RootConflict{
		{Root: "workspace", Path: "main.go", Kind: syncpkg.ConflictHostModified},
		{Root: "cache", Path: "index.db", Kind: syncpkg.ConflictHostCreated},
	})
	if summary == "" || detail == "" {
		t.Fatalf("summary/detail should not be empty")
	}
	if !strings.Contains(detail, "workspace") || !strings.Contains(detail, "main.go") {
		t.Fatalf("detail = %q, want workspace/main.go", detail)
	}
	if !strings.Contains(detail, "cache") || !strings.Contains(detail, "index.db") {
		t.Fatalf("detail = %q, want cache/index.db", detail)
	}
}

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

	mustWriteFile(t, file, []byte("modified"), 0o644)
	setModTime(t, file, time.Now().Add(time.Second))

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

	changedFile := filepath.Join(dir1, "a.txt")
	mustWriteFile(t, changedFile, []byte("modified"), 0o644)
	setModTime(t, changedFile, time.Now().Add(time.Second))

	conflicts := snap.DetectConflicts()
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict from multi-dir snapshot, got %d", len(conflicts))
	}
}

func setModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("setting mtime: %v", err)
	}
}

func assertSingleRootConflict(t *testing.T, conflicts []syncpkg.RootConflict, root, path string, kind syncpkg.ConflictKind) {
	t.Helper()
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1: %#v", len(conflicts), conflicts)
	}
	got := conflicts[0]
	if got.Root != root || got.Path != path || got.Kind != kind {
		t.Fatalf("conflict = %#v, want root=%q path=%q kind=%v", got, root, path, kind)
	}
}
