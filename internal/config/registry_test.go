package config

import (
	"os"
	"testing"
)

func TestLoadEditorRegistry(t *testing.T) {
	registry, err := LoadEditorRegistry()
	if err != nil {
		t.Fatalf("LoadEditorRegistry() error: %v", err)
	}
	if registry == nil {
		t.Fatal("LoadEditorRegistry() returned nil")
	}
}

func TestEditorRegistry_Names(t *testing.T) {
	registry, err := LoadEditorRegistry()
	if err != nil {
		t.Fatalf("LoadEditorRegistry() error: %v", err)
	}

	expected := []string{"aider", "claude", "codex", "copilot", "gemini", "goose", "opencode", "vibe"}
	names := registry.Names()

	if len(names) != len(expected) {
		t.Fatalf("Names() returned %d editors, want %d: got %v", len(names), len(expected), names)
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("Names()[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestEditorRegistry_CursorAbsent(t *testing.T) {
	registry, _ := LoadEditorRegistry()
	if registry.Has("cursor") {
		t.Error("cursor should not exist in registry (removed in dev branch)")
	}
}

func TestEditorRegistry_FieldsCorrect(t *testing.T) {
	registry, err := LoadEditorRegistry()
	if err != nil {
		t.Fatalf("LoadEditorRegistry() error: %v", err)
	}

	tests := []struct {
		editor     string
		imageTag   string
		cmdName    string
		configPath string
		envVars    []string
		legacy     string
	}{
		{"claude", "ghcr.io/r-dson/abox:claude", "claude", ".claude", []string{"ANTHROPIC_API_KEY"}, ""},
		{"opencode", "ghcr.io/r-dson/abox:opencode", "opencode", ".config/opencode", []string{}, ".opencode"},
		{"aider", "ghcr.io/r-dson/abox:aider", "aider", ".aider.conf.yml", []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}, ""},
		{"codex", "ghcr.io/r-dson/abox:codex", "codex", ".codex", []string{}, ""},
		{"copilot", "ghcr.io/r-dson/abox:copilot", "copilot", ".copilot", []string{"GITHUB_TOKEN"}, ""},
		{"gemini", "ghcr.io/r-dson/abox:gemini", "gemini", ".gemini", []string{"GOOGLE_API_KEY"}, ""},
		{"goose", "ghcr.io/r-dson/abox:goose", "goose", ".config/goose", []string{}, ""},
		{"vibe", "ghcr.io/r-dson/abox:vibe", "vibe", ".vibe", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.editor, func(t *testing.T) {
			p, err := registry.Get(tt.editor)
			if err != nil {
				t.Fatalf("Get(%q) error: %v", tt.editor, err)
			}
			if p.ImageTag != tt.imageTag {
				t.Errorf("ImageTag = %q, want %q", p.ImageTag, tt.imageTag)
			}
			if p.CmdName != tt.cmdName {
				t.Errorf("CmdName = %q, want %q", p.CmdName, tt.cmdName)
			}
			if p.ConfigPath != tt.configPath {
				t.Errorf("ConfigPath = %q, want %q", p.ConfigPath, tt.configPath)
			}
			if len(p.EnvVars) != len(tt.envVars) {
				t.Errorf("EnvVars = %v, want %v", p.EnvVars, tt.envVars)
			} else {
				for i, want := range tt.envVars {
					if p.EnvVars[i] != want {
						t.Errorf("EnvVars[%d] = %q, want %q", i, p.EnvVars[i], want)
					}
				}
			}
			if p.LegacyPath != tt.legacy {
				t.Errorf("LegacyPath = %q, want %q", p.LegacyPath, tt.legacy)
			}
		})
	}
}

func TestEditorRegistry_UnknownEditorFallsBackToOpencode(t *testing.T) {
	registry, _ := LoadEditorRegistry()

	p, err := registry.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get(nonexistent) error: %v", err)
	}
	if p.CmdName != "opencode" {
		t.Errorf("Unknown editor fell back to %q, want opencode", p.CmdName)
	}
}

func TestEditorProfile_DerivedPaths(t *testing.T) {
	p := EditorProfile{CmdName: "claude"}
	home := "/home/testuser"

	if got := p.CachePath(home); got != "/home/testuser/.cache/claude" {
		t.Errorf("CachePath = %q, want %q", got, "/home/testuser/.cache/claude")
	}
	if got := p.StatePath(home); got != "/home/testuser/.local/state/claude" {
		t.Errorf("StatePath = %q, want %q", got, "/home/testuser/.local/state/claude")
	}
	if got := p.SharePath(home); got != "/home/testuser/.local/share/claude" {
		t.Errorf("SharePath = %q, want %q", got, "/home/testuser/.local/share/claude")
	}
	if got := p.ConfigFullPath(home); got != "/home/testuser" {
		t.Errorf("ConfigFullPath with empty ConfigPath = %q, want /home/testuser", got)
	}

	p.ConfigPath = ".claude"
	if got := p.ConfigFullPath(home); got != "/home/testuser/.claude" {
		t.Errorf("ConfigFullPath = %q, want %q", got, "/home/testuser/.claude")
	}
}

func TestHomeDir(t *testing.T) {
	orig := os.Getenv("HOME")
	defer os.Setenv("HOME", orig)

	os.Setenv("HOME", "/test/home")
	if got := HomeDir(); got != "/test/home" {
		t.Errorf("HomeDir() = %q, want /test/home", got)
	}
}
