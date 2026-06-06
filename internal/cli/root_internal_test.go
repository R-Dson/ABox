package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/spf13/cobra"
)

func TestApplyLoadedConfigDefaults(t *testing.T) {
	cmd, cfg := testConfigCommand()
	loaded := &config.Config{
		Editor:          "claude",
		StrictNetwork:   true,
		NoInternet:      true,
		ForwardSSHAgent: true,
		MemoryLimit:     "8g",
		CPULimit:        4,
	}

	applyLoadedConfig(cmd, cfg, loaded)

	if cfg.Editor != "claude" {
		t.Errorf("Editor = %q, want claude", cfg.Editor)
	}
	if !cfg.StrictNetwork {
		t.Error("StrictNetwork should default from loaded config")
	}
	if !cfg.NoInternet {
		t.Error("NoInternet should default from loaded config")
	}
	if !cfg.ForwardSSHAgent {
		t.Error("ForwardSSHAgent should default from loaded config")
	}
	if cfg.MemoryLimit != "8g" {
		t.Errorf("MemoryLimit = %q, want 8g", cfg.MemoryLimit)
	}
	if cfg.CPULimit != 4 {
		t.Errorf("CPULimit = %v, want 4", cfg.CPULimit)
	}
}

func TestApplyLoadedConfig_PreservesExplicitFlags(t *testing.T) {
	cmd, cfg := testConfigCommand()
	if err := cmd.ParseFlags([]string{"--editor", "gemini", "--strict-network=false", "--no-internet=false"}); err != nil {
		t.Fatalf("ParseFlags() error: %v", err)
	}
	loaded := &config.Config{
		Editor:        "claude",
		StrictNetwork: true,
		NoInternet:    true,
		MemoryLimit:   "8g",
		CPULimit:      4,
	}

	applyLoadedConfig(cmd, cfg, loaded)

	if cfg.Editor != "gemini" {
		t.Errorf("Editor = %q, want gemini", cfg.Editor)
	}
	if cfg.StrictNetwork {
		t.Error("StrictNetwork explicit false flag should override loaded config")
	}
	if cfg.NoInternet {
		t.Error("NoInternet explicit false flag should override loaded config")
	}
}

func TestResolveWorkdirRejectsSymlinkToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	linkPath := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(home, linkPath); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	_, err := resolveWorkdir(linkPath)
	if err == nil {
		t.Fatal("expected symlink to HOME to be rejected")
	}
}

func TestResolveWorkdirReturnsCanonicalPath(t *testing.T) {
	realDir := t.TempDir()
	linkPath := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	got, err := resolveWorkdir(linkPath)
	if err != nil {
		t.Fatalf("resolveWorkdir() error: %v", err)
	}
	if got != realDir {
		t.Fatalf("resolveWorkdir() = %q, want %q", got, realDir)
	}
}

func testConfigCommand() (*cobra.Command, *SessionConfig) {
	cfg := &SessionConfig{}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringVar(&cfg.Editor, "editor", "", "")
	cmd.Flags().BoolVar(&cfg.StrictNetwork, "strict-network", false, "")
	cmd.Flags().BoolVar(&cfg.NoInternet, "no-internet", false, "")
	cmd.Flags().BoolVar(&cfg.ForwardSSHAgent, "ssh-agent", false, "")
	return cmd, cfg
}
