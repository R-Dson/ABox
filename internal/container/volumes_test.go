package container_test

import (
	"context"
	"sync"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
)

func TestNewSession_CreatesAllVolumes(t *testing.T) {
	createdVolumes := []string{}
	var mu sync.Mutex
	mock := &volumeTrackingRuntime{
		onVolumeCreate: func(name string, _ map[string]string) error {
			mu.Lock()
			createdVolumes = append(createdVolumes, name)
			mu.Unlock()
			return nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(mock)
	sess, err := mgr.CreateSession(t.Context(), profile, cfg)
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
	callCount := 0
	mock := &volumeTrackingRuntime{
		onVolumeCreate: func(name string, _ map[string]string) error {
			callCount++
			if callCount == 2 {
				return errTestFailed
			}
			return nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(mock)
	_, err := mgr.CreateSession(t.Context(), profile, cfg)
	if err == nil {
		t.Fatal("expected error when volume creation fails")
	}
}

// volumeTrackingRuntime extends stubRuntime with volume creation tracking.
type volumeTrackingRuntime struct {
	stubRuntime
	onVolumeCreate func(name string, labels map[string]string) error
}

func (v *volumeTrackingRuntime) VolumeCreate(_ context.Context, name string, labels map[string]string) error {
	if v.onVolumeCreate != nil {
		return v.onVolumeCreate(name, labels)
	}
	return nil
}

var errTestFailed = errTestFailedType{}

type errTestFailedType struct{}

func (errTestFailedType) Error() string { return "test-induced failure" }
