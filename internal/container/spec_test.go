package container_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/runtime"
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

	spec := mustBuildSpec(t, profile, sess, "/workspace", &config.Config{})

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

	spec := mustBuildSpec(t, profile, sess, "/workspace", &config.Config{})

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

	spec := mustBuildSpec(t, profile, sess, "/workspace", &config.Config{})

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

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{})

	if spec.WorkingDir != "/workspace" {
		t.Errorf("WorkingDir = %q, want /workspace", spec.WorkingDir)
	}
}

func TestBuildSpec_EditorDataMountTargets(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{
		ConfigVol: "abox-config-test",
		CacheVol:  "abox-cache-test",
		StateVol:  "abox-state-test",
		ShareVol:  "abox-share-test",
	})

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{})

	want := map[string]bool{
		"abox-config-test:/home/agent/.claude":            false,
		"abox-cache-test:/home/agent/.cache/claude":       false,
		"abox-state-test:/home/agent/.local/state/claude": false,
		"abox-share-test:/home/agent/.local/share/claude": false,
		"/host/project:/workspace":                        false,
	}
	for _, bind := range spec.Binds {
		if _, ok := want[bind]; ok {
			want[bind] = true
		}
		if bind == "abox-config-test:/vol/config" {
			t.Fatalf("config volume mounted at legacy sync path %q", bind)
		}
	}
	for bind, found := range want {
		if !found {
			t.Errorf("missing bind %q in %v", bind, spec.Binds)
		}
	}
}

func TestBuildSpec_DoesNotMountSSHDirectoryFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("creating .ssh fixture: %v", err)
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{})

	for _, bind := range spec.Binds {
		if strings.Contains(bind, ".ssh") {
			t.Fatalf("host .ssh directory must not be mounted implicitly: %q", bind)
		}
	}
}

func TestBuildSpec_DoesNotMountSSHAgentSocketByDefault(t *testing.T) {
	home := t.TempDir()
	socketPath := filepath.Join(home, "ssh-agent.sock")
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", socketPath)
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatalf("creating ssh agent socket fixture: %v", err)
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{})

	for _, bind := range spec.Binds {
		if strings.Contains(bind, "ssh-agent.sock") {
			t.Fatalf("SSH agent socket must not be mounted by default: %q", bind)
		}
	}
	for _, env := range spec.Env {
		if strings.HasPrefix(env, "SSH_AUTH_SOCK=") {
			t.Fatalf("SSH_AUTH_SOCK must not be set by default: %q", env)
		}
	}
}

func TestBuildSpec_MountsSSHAgentSocketWhenEnabled(t *testing.T) {
	home := t.TempDir()
	socketPath := filepath.Join(home, "ssh-agent.sock")
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", socketPath)
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatalf("creating ssh agent socket fixture: %v", err)
	}

	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{ForwardSSHAgent: true})

	want := socketPath + ":/tmp/ssh-agent.sock:ro"
	for _, bind := range spec.Binds {
		if bind == want {
			return
		}
	}
	t.Fatalf("missing SSH agent bind %q in %v", want, spec.Binds)
}

func TestBuildSpec_FileConfigProfileUsesSymlinkWrapper(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("aider")
	sess := container.NewSession("test", nil, container.Volumes{
		ConfigVol: "abox-config-test",
		CacheVol:  "abox-cache-test",
		StateVol:  "abox-state-test",
		ShareVol:  "abox-share-test",
	})

	spec := mustBuildSpec(t, profile, sess, "/host/project", &config.Config{})

	foundConfigBind := false
	for _, bind := range spec.Binds {
		if bind == "abox-config-test:/vol/config" {
			foundConfigBind = true
		}
		if bind == "abox-config-test:/home/agent/.aider.conf.yml" {
			t.Fatalf("file config volume must not mount directly to file path: %q", bind)
		}
	}
	if !foundConfigBind {
		t.Fatalf("missing file config bind to /vol/config in %v", spec.Binds)
	}
	if len(spec.Cmd) != 4 || spec.Cmd[0] != "sh" || spec.Cmd[1] != "-lc" || spec.Cmd[3] != "aider" {
		t.Fatalf("file config command should be shell wrapper with argv0 placeholder, got %v", spec.Cmd)
	}
	if !strings.Contains(spec.Cmd[2], "ln -sf /vol/config/.aider.conf.yml /home/agent/.aider.conf.yml") {
		t.Fatalf("file config command missing symlink setup: %q", spec.Cmd[2])
	}
	if !strings.Contains(spec.Cmd[2], "exec aider \"$@\"") {
		t.Fatalf("file config command missing editor exec with arg forwarding: %q", spec.Cmd[2])
	}
}

func TestBuildSpec_ImageFromProfile(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := mustBuildSpec(t, profile, sess, "/workspace", &config.Config{})

	if spec.Image != "ghcr.io/r-dson/abox:claude" {
		t.Errorf("Image = %q, want ghcr.io/r-dson/abox:claude", spec.Image)
	}
}

func TestBuildSpec_NoNewPrivileges(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	spec := mustBuildSpec(t, profile, sess, "/workspace", &config.Config{})

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
	path, err := container.SeccompProfilePath()
	if err != nil {
		t.Fatalf("SeccompProfilePath() error: %v", err)
	}
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

func TestBuildSpec_ReturnsInvalidMemoryLimitError(t *testing.T) {
	registry, _ := config.LoadEditorRegistry()
	profile, _ := registry.Get("claude")
	sess := container.NewSession("test", nil, container.Volumes{})

	_, err := container.BuildSpec(profile, sess, "/workspace", &config.Config{MemoryLimit: "not-memory"})
	if err == nil {
		t.Fatal("expected invalid memory limit error")
	}
}

func mustBuildSpec(t *testing.T, profile config.EditorProfile, sess *container.Session, workdir string, cfg *config.Config) runtime.ContainerSpec {
	t.Helper()
	spec, err := container.BuildSpec(profile, sess, workdir, cfg)
	if err != nil {
		t.Fatalf("BuildSpec() error: %v", err)
	}
	return spec
}
