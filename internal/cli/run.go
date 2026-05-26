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

// SessionConfig holds the resolved configuration for a run command.
// Cobra flags bind directly to this struct.
type SessionConfig struct {
	Editor        string
	Shell         bool
	ForceIT       bool
	Offline       bool
	StrictNetwork bool
	NoInternet    bool
	ForceSync     bool
	ExtraEnv      []string
	EditorArgs    []string
}

// ExitError wraps an exit code from the container process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func newRunCmd() *cobra.Command {
	cfg := &SessionConfig{}
	cmd := &cobra.Command{
		Use:   "run [directory]",
		Short: "Run an editor in a secure sandbox",
		Long:  "Launch an AI coding editor inside an isolated container with workspace sync and exclusion filtering.",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Load .abxenv from workspace and merge with --env flags
			cfg.ExtraEnv = append(cfg.ExtraEnv, LoadDotEnv(absWorkdir)...)

			rt, err := runtime.Detect(cmd.Context())
			if err != nil {
				return err
			}
			return RunSession(cmd.Context(), rt, absWorkdir, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Editor, "editor", "", "editor to use (aider|claude|codex|copilot|gemini|goose|opencode|vibe)")
	cmd.Flags().BoolVar(&cfg.Shell, "shell", false, "drop into an interactive shell")
	cmd.Flags().BoolVar(&cfg.ForceIT, "force-it", false, "force interactive TTY allocation")
	cmd.Flags().BoolVar(&cfg.Offline, "offline", false, "do not pull images")
	cmd.Flags().BoolVar(&cfg.StrictNetwork, "strict-network", false, "block all external network access")
	cmd.Flags().BoolVar(&cfg.NoInternet, "no-internet", false, "disable networking entirely")
	cmd.Flags().BoolVar(&cfg.ForceSync, "force-sync", false, "overwrite host files even if modified during session")
	cmd.Flags().StringArrayVar(&cfg.ExtraEnv, "env", nil, "pass environment variable to container (repeatable)")

	return cmd
}

// blockedEnvKeys are environment variables that must never be injected
// into the container from .abxenv — they control critical runtime behavior.
var blockedEnvKeys = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "HOSTNAME": true,
	"SHELL": true, "UID": true, "GID": true, "PWD": true,
	"LANG": true, "TERM": true, "DISPLAY": true, "XAUTHORITY": true,
	"DBUS_SESSION_BUS_ADDRESS": true,
}

// LoadDotEnv reads a .abxenv file from the given directory.
// Returns KEY=VALUE pairs resolved from the host environment.
// Dangerous variable names (PATH, HOME, etc.) are silently skipped.
// Returns nil if the file doesn't exist.
func LoadDotEnv(dir string) []string {
	path := filepath.Join(dir, ".abxenv")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var env []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if key == "" || blockedEnvKeys[key] {
			continue
		}
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}
	return env
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
	matcher, err := exclusion.BuildMatcher(ctx, workdir)
	if err != nil {
		return fmt.Errorf("building exclusion matcher: %w", err)
	}

	// 3. Create container session (volumes, optional strict network)
	hasWorkspaceVol := matcher.HasPatterns()
	sess, err := container.CreateSession(ctx, rt, profile, &config.Config{
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
	syncDirs := map[string]string{
		sess.Vol.ConfigVol: profile.ConfigFullPath(home),
		sess.Vol.CacheVol:  profile.CachePath(home),
		sess.Vol.StateVol:  profile.StatePath(home),
		sess.Vol.ShareVol:  profile.SharePath(home),
	}

	g, gctx := errgroup.WithContext(ctx)
	for vol, srcDir := range syncDirs {
		vol, srcDir := vol, srcDir
		g.Go(func() error {
			if err := sync.In(gctx, rt, srcDir, vol, "/data", nil); err != nil {
				return fmt.Errorf("sync-in %s: %w", srcDir, err)
			}
			return nil
		})
	}

	// Workspace sync with exclusion filtering
	if sess.Vol.WorkspaceVol != "" {
		wsVol := sess.Vol.WorkspaceVol
		g.Go(func() error {
			if err := sync.In(gctx, rt, workdir, wsVol, "/data", matcher); err != nil {
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
		spec.Env = append(spec.Env, cfg.ExtraEnv...)
	}

	if cfg.Shell {
		spec.Cmd = []string{"/bin/sh"}
	} else if len(cfg.EditorArgs) > 0 {
		spec.Cmd = append(spec.Cmd, cfg.EditorArgs...)
	}

	exitCode, err := container.Run(ctx, rt, spec)
	if err != nil {
		return fmt.Errorf("running container: %w", err)
	}

	// 7. Check for mtime conflicts
	if snap != nil {
		conflicts := snap.DetectConflicts()
		if len(conflicts) > 0 && !cfg.ForceSync {
			summary, detail := sync.FormatConflicts(conflicts)
			slog.WarnContext(ctx, "skipping sync-out: "+summary,
				"force-sync", cfg.ForceSync)
			slog.DebugContext(ctx, detail)
			return &ExitError{Code: exitCode}
		}
	}

	// 8. SyncOut: container → host (parallel, best-effort)
	g, _ = errgroup.WithContext(ctx)
	for vol, srcDir := range syncDirs {
		vol, srcDir := vol, srcDir
		g.Go(func() error {
			if err := sync.Out(ctx, rt, vol, "/data", srcDir); err != nil {
				slog.WarnContext(ctx, "sync-out failed", "dir", srcDir, "error", err)
			}
			return nil
		})
	}
	_ = g.Wait()

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
