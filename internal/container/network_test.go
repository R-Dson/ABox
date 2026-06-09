package container_test

import (
	"context"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtimetest"
)

func TestCreateSession_StrictNetwork(t *testing.T) {
	var createdInternal bool
	stub := &runtimetest.StubRuntime{
		NetworkCreateFn: func(_ context.Context, _ string, internal bool) (string, error) {
			createdInternal = internal
			return "net-strict-123", nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude", StrictNetwork: true}

	sess, err := container.CreateSession(t.Context(), stub, profile, cfg, false)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	defer sess.Cleanup(context.Background())

	if !createdInternal {
		t.Error("NetworkCreate was not called with internal=true")
	}
	if sess.Vol.NetworkID != "net-strict-123" {
		t.Errorf("NetworkID = %q, want net-strict-123", sess.Vol.NetworkID)
	}
}

func TestCreateSession_NoNetworkByDefault(t *testing.T) {
	networkCreated := false
	stub := &runtimetest.StubRuntime{
		NetworkCreateFn: func(_ context.Context, _ string, _ bool) (string, error) {
			networkCreated = true
			return "net-xyz", nil
		},
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	cfg := &config.Config{Editor: "claude"}

	sess, err := container.CreateSession(t.Context(), stub, profile, cfg, false)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	defer sess.Cleanup(context.Background())

	if networkCreated {
		t.Error("network should not be created when StrictNetwork=false")
	}
	if sess.Vol.NetworkID != "" {
		t.Errorf("NetworkID = %q, want empty", sess.Vol.NetworkID)
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
			sess: container.NewSession("t", nil, container.Volumes{}),
			want: "none",
		},
		{
			name: "strict network returns network id",
			cfg:  config.Config{StrictNetwork: true},
			sess: container.NewSession("t", nil, container.Volumes{NetworkID: "net-abc"}),
			want: "net-abc",
		},
		{
			name: "default returns empty (bridge)",
			cfg:  config.Config{},
			sess: container.NewSession("t", nil, container.Volumes{}),
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
