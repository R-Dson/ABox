package sync_test

import (
	"io/fs"
	"os"
	"testing"
)

func mustWriteFile(t *testing.T, path string, data []byte, perm int) {
	t.Helper()
	if err := os.WriteFile(path, data, fs.FileMode(perm)); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
