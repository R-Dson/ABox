package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoad_Defaults(t *testing.T) {
	// No config file — all defaults should apply
	v := viper.New()
	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error without config file: %v", err)
	}
	if cfg.Editor != "opencode" {
		t.Errorf("default Editor = %q, want opencode", cfg.Editor)
	}
	if cfg.PullPolicy != "missing" {
		t.Errorf("default PullPolicy = %q, want missing", cfg.PullPolicy)
	}
	if cfg.MemoryLimit != "4g" {
		t.Errorf("default MemoryLimit = %q, want 4g", cfg.MemoryLimit)
	}
	if cfg.CPULimit != 2.0 {
		t.Errorf("default CPULimit = %f, want 2.0", cfg.CPULimit)
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
	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Editor != "claude" {
		t.Errorf("Editor = %q, want claude", cfg.Editor)
	}
	if cfg.ExcludeURL != "https://example.com/ignore" {
		t.Errorf("ExcludeURL = %q, want https://example.com/ignore", cfg.ExcludeURL)
	}
	if cfg.Verbose != true {
		t.Errorf("Verbose = %v, want true", cfg.Verbose)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("ABX_EDITOR", "gemini")
	defer os.Unsetenv("ABX_EDITOR")

	v := viper.New()
	v.SetEnvPrefix("ABX")
	v.AutomaticEnv()

	cfg, err := Load(v)
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
	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() with missing config should not error: %v", err)
	}
	if cfg.Editor != "opencode" {
		t.Errorf("default Editor = %q, want opencode", cfg.Editor)
	}
}
