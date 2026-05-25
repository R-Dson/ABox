package container_test

import (
	"context"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
)

func TestCreateSession_StrictNetwork(t *testing.T) {
	var createdInternal bool
	stub := &runtime.StubRuntime{
		NetworkCreateFn: func(_ context.Context, _ string, internal bool) (string, error) {
			createdInternal = internal
			return "net-strict-123", nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude", StrictNetwork: true}

	mgr := container.NewManager(stub)
	sess, err := mgr.CreateSession(t.Context(), profile, cfg)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	defer sess.Cleanup(context.Background())

	if !createdInternal {
		t.Error("NetworkCreate was not called with internal=true")
	}
	if sess.NetworkID() != "net-strict-123" {
		t.Errorf("NetworkID = %q, want net-strict-123", sess.NetworkID())
	}
}

func TestCreateSession_NoNetworkByDefault(t *testing.T) {
	networkCreated := false
	stub := &runtime.StubRuntime{
		NetworkCreateFn: func(_ context.Context, _ string, _ bool) (string, error) {
			networkCreated = true
			return "net-xyz", nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	mgr := container.NewManager(stub)
	sess, err := mgr.CreateSession(t.Context(), profile, cfg)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	defer sess.Cleanup(context.Background())

	if networkCreated {
		t.Error("network should not be created when StrictNetwork=false")
	}
	if sess.NetworkID() != "" {
		t.Errorf("NetworkID = %q, want empty", sess.NetworkID())
	}
}

func TestResolveNetworkMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		sess *container.Session
		want string
	}{
		{
			name: "no internet returns none",
			cfg:  config.Config{NoInternet: true},
			sess: container.NewSession("t", nil, container.SessionVolumes{}),
			want: "none",
		},
		{
			name: "strict network returns network id",
			cfg:  config.Config{StrictNetwork: true},
			sess: container.NewSession("t", nil, container.SessionVolumes{NetworkID: "net-abc"}),
			want: "net-abc",
		},
		{
			name: "default returns empty (bridge)",
			cfg:  config.Config{},
			sess: container.NewSession("t", nil, container.SessionVolumes{}),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := container.ResolveNetworkMode(tt.sess, &tt.cfg)
			if got != tt.want {
				t.Errorf("ResolveNetworkMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
