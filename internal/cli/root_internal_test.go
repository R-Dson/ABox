package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/runtimetest"
	"github.com/spf13/cobra"
)

func TestRoot_AllowsDirectoryAndEditorArgsAfterSeparator(t *testing.T) {
	dir := t.TempDir()
	gotWorkdir, gotEditorArgs := executeRootWithRunCapture(t, []string{dir, "--", "--model", "x"})

	if gotWorkdir != dir {
		t.Fatalf("workdir = %q, want %q", gotWorkdir, dir)
	}
	if want := []string{"--model", "x"}; !equalStringSlices(gotEditorArgs, want) {
		t.Fatalf("EditorArgs = %v, want %v", gotEditorArgs, want)
	}
}

func TestRoot_AllowsDirectoryThenEditorArgs(t *testing.T) {
	dir := t.TempDir()
	gotWorkdir, gotEditorArgs := executeRootWithRunCapture(t, []string{dir, "prompt.txt"})

	if gotWorkdir != dir {
		t.Fatalf("workdir = %q, want %q", gotWorkdir, dir)
	}
	if want := []string{"prompt.txt"}; !equalStringSlices(gotEditorArgs, want) {
		t.Fatalf("EditorArgs = %v, want %v", gotEditorArgs, want)
	}
}

func TestRoot_InfoSubcommandsSkipUserConfigLoad(t *testing.T) {
	configHome := t.TempDir()
	dir := filepath.Join(configHome, "abx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := NewRootCmd("test")
	root.SetArgs([]string{"version"})
	root.SetOut(new(bytes.Buffer))

	if err := root.Execute(); err != nil {
		t.Fatalf("version error = %v, want nil", err)
	}
}

func TestRoot_ClosesRuntimeAfterRun(t *testing.T) {
	dir := t.TempDir()
	closed := false
	oldLoad := loadUserConfigFunc
	oldDetect := detectRuntimeFunc
	oldRun := runSessionFunc
	loadUserConfigFunc = func() (*config.Config, error) {
		return &config.Config{Editor: "opencode", PullPolicy: pullPolicyNever}, nil
	}
	detectRuntimeFunc = func(context.Context) (runtime.ContainerRuntime, error) {
		return &runtimetest.StubRuntime{CloseFn: func() error {
			closed = true
			return nil
		}}, nil
	}
	runSessionFunc = func(context.Context, runtime.ContainerRuntime, string, *SessionConfig) error {
		return nil
	}
	defer func() {
		loadUserConfigFunc = oldLoad
		detectRuntimeFunc = oldDetect
		runSessionFunc = oldRun
	}()

	root := NewRootCmd("test")
	root.SetArgs([]string{dir})
	root.SetOut(new(bytes.Buffer))

	if err := root.Execute(); err != nil {
		t.Fatalf("root run error = %v", err)
	}
	if !closed {
		t.Fatal("runtime was not closed")
	}
}

func TestRoot_LoadsUserConfigOnceForRun(t *testing.T) {
	dir := t.TempDir()
	loadCount := 0
	oldLoad := loadUserConfigFunc
	oldDetect := detectRuntimeFunc
	oldRun := runSessionFunc
	loadUserConfigFunc = func() (*config.Config, error) {
		loadCount++
		return &config.Config{Editor: "opencode", PullPolicy: pullPolicyNever}, nil
	}
	detectRuntimeFunc = func(context.Context) (runtime.ContainerRuntime, error) {
		return &runtimetest.StubRuntime{}, nil
	}
	runSessionFunc = func(context.Context, runtime.ContainerRuntime, string, *SessionConfig) error {
		return nil
	}
	defer func() {
		loadUserConfigFunc = oldLoad
		detectRuntimeFunc = oldDetect
		runSessionFunc = oldRun
	}()

	root := NewRootCmd("test")
	root.SetArgs([]string{dir})
	root.SetOut(new(bytes.Buffer))

	if err := root.Execute(); err != nil {
		t.Fatalf("root run error = %v", err)
	}
	if loadCount != 1 {
		t.Fatalf("loadUserConfig calls = %d, want 1", loadCount)
	}
}

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

func executeRootWithRunCapture(t *testing.T, args []string) (string, []string) {
	t.Helper()
	oldLoad := loadUserConfigFunc
	oldDetect := detectRuntimeFunc
	oldRun := runSessionFunc
	loadUserConfigFunc = func() (*config.Config, error) {
		return &config.Config{Editor: "opencode", PullPolicy: pullPolicyNever}, nil
	}
	detectRuntimeFunc = func(context.Context) (runtime.ContainerRuntime, error) {
		return &runtimetest.StubRuntime{}, nil
	}
	var gotWorkdir string
	var gotEditorArgs []string
	runSessionFunc = func(_ context.Context, _ runtime.ContainerRuntime, workdir string, cfg *SessionConfig) error {
		gotWorkdir = workdir
		gotEditorArgs = append([]string(nil), cfg.EditorArgs...)
		return nil
	}
	defer func() {
		loadUserConfigFunc = oldLoad
		detectRuntimeFunc = oldDetect
		runSessionFunc = oldRun
	}()

	root := NewRootCmd("test")
	root.SetArgs(args)
	root.SetOut(new(bytes.Buffer))
	if err := root.Execute(); err != nil {
		t.Fatalf("root execute error: %v", err)
	}
	return gotWorkdir, gotEditorArgs
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
