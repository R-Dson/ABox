package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/spf13/viper"
)

func TestLoadEditorRegistry(t *testing.T) {
	registry, err := config.LoadEditorRegistry()
	if err != nil {
		t.Fatalf("LoadEditorRegistry() error: %v", err)
	}
	if registry == nil {
		t.Fatal("LoadEditorRegistry() returned nil")
	}
}

func TestEditorRegistry_Names(t *testing.T) {
	registry, err := config.LoadEditorRegistry()
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
	registry, _ := config.LoadEditorRegistry()
	if registry.Has("cursor") {
		t.Error("cursor should not exist in registry (removed in dev branch)")
	}
}

func TestEditorRegistry_FieldsCorrect(t *testing.T) {
	registry, err := config.LoadEditorRegistry()
	if err != nil {
		t.Fatalf("LoadEditorRegistry() error: %v", err)
	}

	tests := []struct {
		name       string
		editor     string
		imageTag   string
		cmdName    string
		configPath string
		envVars    []string
		legacy     string
	}{
		{"claude fields", "claude", "ghcr.io/r-dson/abox:claude", "claude", ".claude", []string{"ANTHROPIC_API_KEY"}, ""},
		{"opencode fields", "opencode", "ghcr.io/r-dson/abox:opencode", "opencode", ".config/opencode", []string{}, ".opencode"},
		{"aider fields", "aider", "ghcr.io/r-dson/abox:aider", "aider", ".aider.conf.yml", []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}, ""},
		{"codex fields", "codex", "ghcr.io/r-dson/abox:codex", "codex", ".codex", []string{}, ""},
		{"copilot fields", "copilot", "ghcr.io/r-dson/abox:copilot", "copilot", ".copilot", []string{"GITHUB_TOKEN"}, ""},
		{"gemini fields", "gemini", "ghcr.io/r-dson/abox:gemini", "gemini", ".gemini", []string{"GOOGLE_API_KEY"}, ""},
		{"goose fields", "goose", "ghcr.io/r-dson/abox:goose", "goose", ".config/goose", []string{}, ""},
		{"vibe fields", "vibe", "ghcr.io/r-dson/abox:vibe", "vibe", ".vibe", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	registry, _ := config.LoadEditorRegistry()

	p, err := registry.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get(nonexistent) error: %v", err)
	}
	if p.CmdName != "opencode" {
		t.Errorf("Unknown editor fell back to %q, want opencode", p.CmdName)
	}
}

func TestEditorProfile_DerivedPaths(t *testing.T) {
	p := config.EditorProfile{CmdName: "claude"}
	home := "/home/testuser"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"CachePath", p.CachePath(home), "/home/testuser/.cache/claude"},
		{"StatePath", p.StatePath(home), "/home/testuser/.local/state/claude"},
		{"SharePath", p.SharePath(home), "/home/testuser/.local/share/claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestEditorProfile_ConfigFullPath(t *testing.T) {
	p := config.EditorProfile{CmdName: "claude", ConfigPath: ".claude"}
	home := "/home/testuser"

	if got := p.ConfigFullPath(home); got != "/home/testuser/.claude" {
		t.Errorf("ConfigFullPath = %q, want /home/testuser/.claude", got)
	}
}

func TestHomeDir(t *testing.T) {
	orig := os.Getenv("HOME")
	defer os.Setenv("HOME", orig)

	os.Setenv("HOME", "/test/home")
	if got := config.HomeDir(); got != "/test/home" {
		t.Errorf("HomeDir() = %q, want /test/home", got)
	}
}

func TestLoad_Defaults(t *testing.T) {
	v := viper.New()
	cfg, err := config.Load(v)
	if err != nil {
		t.Fatalf("Load() error without config file: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"default editor", cfg.Editor, "opencode"},
		{"default pull policy", cfg.PullPolicy, "missing"},
		{"default memory limit", cfg.MemoryLimit, "4g"},
		{"default cpu limit", cfg.CPULimit, 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestLoad_ReadsJSONConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "abx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.json")
	content := `{"editor":"claude","exclude_url":"https://example.com/ignore","verbose":true}`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	cfg, err := config.Load(v)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"editor from config", cfg.Editor, "claude"},
		{"exclude_url from config", cfg.ExcludeURL, "https://example.com/ignore"},
		{"verbose from config", cfg.Verbose, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("ABX_EDITOR", "gemini")
	defer os.Unsetenv("ABX_EDITOR")

	v := viper.New()
	v.SetEnvPrefix("ABX")
	v.AutomaticEnv()

	cfg, err := config.Load(v)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Editor != "gemini" {
		t.Errorf("Editor with ABX_EDITOR=gemini = %q, want gemini", cfg.Editor)
	}
}

func TestLoad_MissingConfigFileIsFine(t *testing.T) {
	v := viper.New()
	v.SetConfigFile("/nonexistent/path/config.json")
	cfg, err := config.Load(v)
	if err != nil {
		t.Fatalf("Load() with missing config should not error: %v", err)
	}
	if cfg.Editor != "opencode" {
		t.Errorf("default Editor = %q, want opencode", cfg.Editor)
	}
}
