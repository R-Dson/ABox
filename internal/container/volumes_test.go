package container_test

import (
	"context"
	"sync"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
)

func TestNewSession_CreatesAllVolumes(t *testing.T) {
	createdVolumes := []string{}
	var mu sync.Mutex

	stub := &runtime.StubRuntime{
		VolumeCreateFn: func(_ context.Context, name string, _ map[string]string) error {
			mu.Lock()
			createdVolumes = append(createdVolumes, name)
			mu.Unlock()
			return nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(stub)
	sess, err := mgr.CreateSession(t.Context(), profile, cfg, false)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	defer sess.Cleanup(context.Background())

	expected := []string{"abox-config-", "abox-cache-", "abox-state-", "abox-share-"}
	for _, prefix := range expected {
		found := false
		for _, name := range createdVolumes {
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no volume created with prefix %q (created: %v)", prefix, createdVolumes)
		}
	}

	if sess.ID() == "" {
		t.Error("session ID should not be empty")
	}
}

func TestNewSession_CleanupOnError(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	stub := &runtime.StubRuntime{
		VolumeCreateFn: func(_ context.Context, _ string, _ map[string]string) error {
			mu.Lock()
			callCount++
			isSecond := callCount == 2
			mu.Unlock()
			if isSecond {
				return errTestFailed
			}
			return nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(stub)
	_, err := mgr.CreateSession(t.Context(), profile, cfg, false)
	if err == nil {
		t.Fatal("expected error when volume creation fails")
	}
}

var errTestFailed = errTestFailedType{}

type errTestFailedType struct{}

func (errTestFailedType) Error() string { return "test-induced failure" }
