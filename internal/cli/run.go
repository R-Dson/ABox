package cli

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/sync"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
type SessionConfig struct {
	Editor        string
	StrictNetwork bool
	NoInternet    bool
	ForceSync     bool
	ExcludeURL    string
	Offline       bool
	Shell         bool
	EditorArgs    []string
	ExtraEnv      []string
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
		Use:   "run [directory] [-- editor-args...]",
		Short: "Run an editor in a secure sandbox",
		Long:  "Launch an AI coding editor inside an isolated container with workspace sync and exclusion filtering.",
		Args:  cobra.MinimumNArgs(0),
		RunE:  runSessionFromOpts(opts),
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

// resolveEditorArgs splits args into directory args and editor args at "--".
func resolveEditorArgs(args []string) (dirArgs []string, editorArgs []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func runSessionFromOpts(opts *RunOptions) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		dirArgs, editorArgs := resolveEditorArgs(args)
		// Merge editor args from flags and from --
		allEditorArgs := append(opts.EditorArgs, editorArgs...)

		// Collect extra env from --env flags and .abxenv
		extraEnv := opts.ExtraEnv

		workdir := "."
		if len(dirArgs) > 0 {
			workdir = dirArgs[0]
		}
		absWorkdir, err := filepath.Abs(workdir)
		if err != nil {
			return fmt.Errorf("resolving workdir: %w", err)
		}

		if err := ValidateWorkdir(absWorkdir); err != nil {
			return err
		}

		// Load .abxenv from workspace
		extraEnv = append(extraEnv, LoadDotEnv(absWorkdir)...)

		cfg := &SessionConfig{
			Editor:        opts.Editor,
			StrictNetwork: opts.StrictNetwork,
			NoInternet:    opts.NoInternet,
			ForceSync:     opts.ForceSync,
			ExcludeURL:    opts.ExcludeURL,
			Offline:       opts.Offline,
			Shell:         opts.Shell,
			EditorArgs:    allEditorArgs,
			ExtraEnv:      extraEnv,
		}

		rt, err := runtime.Detect(context.Background())
		if err != nil {
			return err
		}

		return RunSession(context.Background(), rt, absWorkdir, cfg)
	}
}

// LoadDotEnv reads a .abxenv file from the given directory.
// Returns env key names (bare KEY or KEY=value both yield just the key).
// Returns nil if the file doesn't exist.
func LoadDotEnv(dir string) []string {
	path := filepath.Join(dir, ".abxenv")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var keys []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// RunSession runs the full session orchestration with the given runtime.
// Exported for testability — tests inject a mock runtime.
func RunSession(ctx context.Context, rt runtime.ContainerRuntime, workdir string, cfg *SessionConfig) error {
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
	matcher, err := exclusion.BuildMatcher(ctx, workdir, cfg.ExcludeURL)
	if err != nil {
		return fmt.Errorf("building exclusion matcher: %w", err)
	}

	// 3. Create container session (volumes, optional strict network)
	hasWorkspaceVol := matcher.HasPatterns()
	mgr := container.NewManager(rt)
	sess, err := mgr.CreateSession(ctx, profile, &config.Config{
		Editor:        editorName,
		StrictNetwork: cfg.StrictNetwork,
		NoInternet:    cfg.NoInternet,
	}, hasWorkspaceVol)
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

	// 5. SyncIn: host → container volumes (parallel)
	syncer := sync.NewSyncer(rt)

	syncDirs := map[string]string{
		sess.ConfigVol(): profile.ConfigFullPath(home),
		sess.CacheVol():  profile.CachePath(home),
		sess.StateVol():  profile.StatePath(home),
		sess.ShareVol():  profile.SharePath(home),
	}

	g, gctx := errgroup.WithContext(ctx)
	for vol, srcDir := range syncDirs {
		vol, srcDir := vol, srcDir
		g.Go(func() error {
			if err := syncer.SyncIn(gctx, srcDir, vol, "/data"); err != nil {
				return fmt.Errorf("sync-in %s: %w", srcDir, err)
			}
			return nil
		})
	}

	// Workspace sync with exclusion filtering
	if sess.WorkspaceVol() != "" {
		wsVol := sess.WorkspaceVol()
		g.Go(func() error {
			if err := syncer.SyncInFiltered(gctx, workdir, wsVol, "/data", matcher); err != nil {
				return fmt.Errorf("workspace sync-in: %w", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel sync-in: %w", err)
	}

	// 6. Build spec and run the editor container
	spec := container.BuildSpec(profile, sess, workdir, &config.Config{
		Editor:        editorName,
		StrictNetwork: cfg.StrictNetwork,
		NoInternet:    cfg.NoInternet,
	})

	if len(cfg.ExtraEnv) > 0 {
		spec.Env = append(spec.Env, resolveEnvKeys(cfg.ExtraEnv)...)
	}

	if cfg.Shell {
		spec.Cmd = []string{"/bin/sh"}
	} else if len(cfg.EditorArgs) > 0 {
		spec.Cmd = append(spec.Cmd, cfg.EditorArgs...)
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

	// 8. SyncOut: container → host (parallel, best-effort)
	g, _ = errgroup.WithContext(ctx)
	for vol, srcDir := range syncDirs {
		vol, srcDir := vol, srcDir
		g.Go(func() error {
			if err := syncer.SyncOut(ctx, vol, "/data", srcDir); err != nil {
				slog.WarnContext(ctx, "sync-out failed", "dir", srcDir, "error", err)
			}
			return nil
		})
	}
	_ = g.Wait()

	return &ExitError{Code: exitCode}
}

// resolveEnvKeys looks up env keys from the host and returns KEY=VALUE pairs.
func resolveEnvKeys(keys []string) []string {
	var env []string
	for _, key := range keys {
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}
	return env
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
