package container_test

import (
	"os"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
)

func TestBuildSpec_Capabilities(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")

	sess := container.NewSession("test", nil, container.Volumes{
		ConfigVol: "abox-config-test",
		CacheVol:  "abox-cache-test",
		StateVol:  "abox-state-test",
		ShareVol:  "abox-share-test",
	})

	spec := container.BuildSpec(profile, sess, "/workspace", &config.Config{})

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"cap drop", spec.CapDrop, []string{"ALL"}},
		{"cap add", spec.CapAdd, []string{"CHOWN", "SETUID", "SETGID"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.got) != len(tt.want) {
				t.Fatalf("got %v, want %v", tt.got, tt.want)
			}
			for i := range tt.want {
				if tt.got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", tt.got, tt.want)
				}
			}
		})
	}
}

func TestBuildSpec_NoDACOverride(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := container.BuildSpec(profile, sess, "/workspace", &config.Config{})

	for _, cap := range spec.CapAdd {
		if cap == "DAC_OVERRIDE" {
			t.Error("DAC_OVERRIDE must not be in CapAdd (review finding C1b)")
		}
	}
}

func TestBuildSpec_SeccompApplied(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := container.BuildSpec(profile, sess, "/workspace", &config.Config{})

	found := false
	for _, opt := range spec.SecurityOpt {
		if len(opt) >= 7 && opt[:7] == "seccomp" {
			found = true
			// Extract path and verify file exists
			break
		}
	}
	if !found {
		t.Error("no seccomp option in SecurityOpt")
	}
}

func TestBuildSpec_WorkingDir(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := container.BuildSpec(profile, sess, "/my/workspace", &config.Config{})

	if spec.WorkingDir != "/workspace" {
		t.Errorf("WorkingDir = %q, want /workspace", spec.WorkingDir)
	}
}

func TestBuildSpec_ImageFromProfile(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := container.BuildSpec(profile, sess, "/workspace", &config.Config{})

	if spec.Image != "ghcr.io/r-dson/abox:claude" {
		t.Errorf("Image = %q, want ghcr.io/r-dson/abox:claude", spec.Image)
	}
}

func TestBuildSpec_NoNewPrivileges(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := container.BuildSpec(profile, sess, "/workspace", &config.Config{})

	found := false
	for _, opt := range spec.SecurityOpt {
		if opt == "no-new-privileges" {
			found = true
		}
	}
	if !found {
		t.Error("no-new-privileges not in SecurityOpt")
	}
}

func TestSeccompProfileIsValid(t *testing.T) {
	path := container.SeccompProfilePath()
	if path == "" {
		t.Fatal("SeccompProfilePath() returned empty string")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read seccomp profile at %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Error("seccomp profile is empty")
	}
}
