package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/sync"
	"github.com/spf13/cobra"
)

// RunOptions holds all CLI flags for the run command.
type RunOptions struct {
	Editor        string
	Shell         bool
	ForceIT       bool
	Offline       bool
	StrictNetwork bool
	NoInternet    bool
	ForceSync     bool
	ExcludeURL    string
	ExtraEnv      []string
	EditorArgs    []string
}

// SessionConfig holds the resolved configuration for a session.
// Used by RunSessionForTest to inject test configuration.
type SessionConfig struct {
	Editor        string
	StrictNetwork bool
	NoInternet    bool
	ForceSync     bool
	ExcludeURL    string
	Offline       bool
	Shell         bool
}

// ExitError wraps an exit code from the container process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func newRunCmd() *cobra.Command {
	opts := &RunOptions{}
	cmd := &cobra.Command{
		Use:   "run [directory]",
		Short: "Run an editor in a secure sandbox",
		Long:  "Launch an AI coding editor inside an isolated container with workspace sync and exclusion filtering.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runSession(opts),
	}

	cmd.Flags().StringVar(&opts.Editor, "editor", "", "editor to use (aider|claude|codex|copilot|gemini|goose|opencode|vibe)")
	cmd.Flags().BoolVar(&opts.Shell, "shell", false, "drop into an interactive shell")
	cmd.Flags().BoolVar(&opts.ForceIT, "force-it", false, "force interactive TTY allocation")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "do not pull images")
	cmd.Flags().BoolVar(&opts.StrictNetwork, "strict-network", false, "block all external network access")
	cmd.Flags().BoolVar(&opts.NoInternet, "no-internet", false, "disable networking entirely")
	cmd.Flags().BoolVar(&opts.ForceSync, "force-sync", false, "overwrite host files even if modified during session")
	cmd.Flags().StringVar(&opts.ExcludeURL, "exclude-url", "", "URL to fetch exclusion patterns from")
	cmd.Flags().StringArrayVar(&opts.ExtraEnv, "env", nil, "pass environment variable to container (repeatable)")

	return cmd
}

func runSession(opts *RunOptions) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		workdir := "."
		if len(args) > 0 {
			workdir = args[0]
		}
		absWorkdir, err := filepath.Abs(workdir)
		if err != nil {
			return fmt.Errorf("resolving workdir: %w", err)
		}

		if err := ValidateWorkdir(absWorkdir); err != nil {
			return err
		}

		cfg := &SessionConfig{
			Editor:        opts.Editor,
			StrictNetwork: opts.StrictNetwork,
			NoInternet:    opts.NoInternet,
			ForceSync:     opts.ForceSync,
			ExcludeURL:    opts.ExcludeURL,
			Offline:       opts.Offline,
			Shell:         opts.Shell,
		}

		rt, err := runtime.Detect(context.Background())
		if err != nil {
			return err
		}

		return RunSessionForTest(context.Background(), rt, absWorkdir, cfg)
	}
}

// RunSessionForTest runs the full session orchestration with the given runtime.
// Separated from the CLI wiring for testability.
func RunSessionForTest(ctx context.Context, rt runtime.ContainerRuntime, workdir string, cfg *SessionConfig) error {
	// 1. Load editor registry and resolve editor
	registry, err := config.LoadEditorRegistry()
	if err != nil {
		return fmt.Errorf("loading editor registry: %w", err)
	}

	editorName := cfg.Editor
	if editorName == "" {
		editorName = "opencode"
	}

	profile, err := registry.Get(editorName)
	if err != nil {
		return fmt.Errorf("resolving editor: %w", err)
	}

	// 2. Build exclusion matcher
	// 2. Build exclusion matcher
	// Used for workspace SyncIn with exclusion filtering
	_, err = exclusion.BuildMatcher(ctx, workdir, cfg.ExcludeURL)
	if err != nil {
		return fmt.Errorf("building exclusion matcher: %w", err)
	}
	mgr := container.NewManager(rt)
	sess, err := mgr.CreateSession(ctx, profile, &config.Config{
		Editor:        editorName,
		StrictNetwork: cfg.StrictNetwork,
		NoInternet:    cfg.NoInternet,
	})
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer sess.Cleanup(context.Background())

	home := config.HomeDir()

	// 4. Snapshot mtimes before sync-in
	snap, err := sync.SnapshotMtimesFromProfile(profile, home)
	if err != nil {
		slog.WarnContext(ctx, "mtime snapshot failed, continuing without conflict detection", "error", err)
		snap = nil
	}

	// 5. SyncIn: host → container volumes
	syncer := sync.NewSyncer(rt)

	syncDirs := map[string]string{
		sess.ConfigVol(): profile.ConfigFullPath(home),
		sess.CacheVol():  profile.CachePath(home),
		sess.StateVol():  profile.StatePath(home),
		sess.ShareVol():  profile.SharePath(home),
	}

	for vol, srcDir := range syncDirs {
		if err := syncer.SyncIn(ctx, srcDir, vol, "/data"); err != nil {
			return fmt.Errorf("sync-in %s: %w", srcDir, err)
		}
	}

	// 6. Build spec and run the editor container
	spec := container.BuildSpec(profile, sess, workdir, &config.Config{
		Editor:        editorName,
		StrictNetwork: cfg.StrictNetwork,
		NoInternet:    cfg.NoInternet,
	})

	if cfg.Shell {
		spec.Cmd = []string{"/bin/sh"}
	}

	exitCode, err := mgr.Run(ctx, spec)
	if err != nil {
		return fmt.Errorf("running container: %w", err)
	}

	// 7. Check for mtime conflicts
	if snap != nil {
		conflicts := snap.DetectConflicts()
		if len(conflicts) > 0 && !cfg.ForceSync {
			slog.WarnContext(ctx, "files modified during session, skipping sync-out",
				"count", len(conflicts), "force-sync", cfg.ForceSync)
			for _, c := range conflicts {
				slog.WarnContext(ctx, "conflict", "file", c)
			}
			return &ExitError{Code: exitCode}
		}
	}

	// 8. SyncOut: container → host
	for vol, srcDir := range syncDirs {
		if err := syncer.SyncOut(ctx, vol, "/data", srcDir); err != nil {
			slog.WarnContext(ctx, "sync-out failed", "dir", srcDir, "error", err)
		}
	}

	return &ExitError{Code: exitCode}
}

// ValidateWorkdir rejects unsafe workspace paths.
// Expects an absolute path.
func ValidateWorkdir(abs string) error {
	home, _ := os.UserHomeDir()
	if home != "" && abs == home {
		return fmt.Errorf("cannot use $HOME (%s) as workspace", abs)
	}
	if abs == "/" {
		return fmt.Errorf("cannot use / as workspace")
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("workspace %s does not exist", abs)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace %s is not a directory", abs)
	}

	return nil
}
