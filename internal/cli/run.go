package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/go-units"
	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/container"
	"github.com/r-dson/abox/internal/exclusion"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/sync"
	"golang.org/x/sync/errgroup"
)

// SessionConfig holds the resolved configuration for a run command.
// Cobra flags bind directly to this struct.
type SessionConfig struct {
	Editor          string
	Shell           bool
	ForceIT         bool
	Offline         bool
	StrictNetwork   bool
	NoInternet      bool
	ForceSync       bool
	ExcludeURL      string
	PullPolicy      string
	MemoryLimit     string
	CPULimit        float64
	ForwardSSHAgent bool
	ExtraEnv        []string
	EditorArgs      []string
}

// ExitError wraps an exit code from the container process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
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
func LoadDotEnv(dir string) ([]string, error) {
	path := filepath.Join(dir, ".abxenv")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading .abxenv: %w", err)
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
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parsing .abxenv: %w", err)
	}
	return env, nil
}

// RunSession runs the full session orchestration with the given runtime.
// Exported for testability — tests inject a mock runtime.
func RunSession(ctx context.Context, rt runtime.ContainerRuntime, workdir string, cfg *SessionConfig) error {
	if err := validateSessionConfig(cfg); err != nil {
		return err
	}

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
	matcher, err := exclusion.BuildMatcherWithRemote(ctx, workdir, cfg.ExcludeURL)
	if err != nil {
		return fmt.Errorf("building exclusion matcher: %w", err)
	}

	if err := ensureRequiredImages(ctx, rt, profile.ImageTag, cfg); err != nil {
		return err
	}

	resolvedConfig := &config.Config{
		Editor:          editorName,
		StrictNetwork:   cfg.StrictNetwork,
		NoInternet:      cfg.NoInternet,
		PullPolicy:      cfg.PullPolicy,
		MemoryLimit:     cfg.MemoryLimit,
		CPULimit:        cfg.CPULimit,
		ForwardSSHAgent: cfg.ForwardSSHAgent,
	}
	if _, err := container.SeccompProfilePath(); err != nil {
		return fmt.Errorf("materializing seccomp profile: %w", err)
	}

	home := config.HomeDir()
	hasWorkspaceVol := matcher.HasPatterns()
	snapshotDirs := []string{
		profile.ConfigFullPath(home),
		profile.CachePath(home),
		profile.StatePath(home),
		profile.SharePath(home),
	}
	if hasWorkspaceVol {
		snapshotDirs = append(snapshotDirs, workdir)
	}
	snap, err := sync.SnapshotMtimes(snapshotDirs)
	if err != nil {
		return fmt.Errorf("snapshotting mtimes: %w", err)
	}

	// 3. Create container session (volumes, optional strict network)
	sess, err := container.CreateSession(ctx, rt, profile, resolvedConfig, hasWorkspaceVol)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer sess.Cleanup(context.Background())

	syncDirs := map[string]string{
		sess.Vol.ConfigVol: profile.ConfigFullPath(home),
		sess.Vol.CacheVol:  profile.CachePath(home),
		sess.Vol.StateVol:  profile.StatePath(home),
		sess.Vol.ShareVol:  profile.SharePath(home),
	}

	// 4. SyncIn: host → container volumes (parallel, bounded concurrency)

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
	spec, err := container.BuildSpec(profile, sess, workdir, resolvedConfig)
	if err != nil {
		return fmt.Errorf("building container spec: %w", err)
	}
	spec.Tty = shouldAllocateTTY(isTerminalFile(os.Stdin), cfg.Shell, cfg.ForceIT)

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

	// 8. SyncOut: container → host (parallel, bounded concurrency)
	if sess.Vol.WorkspaceVol != "" {
		syncDirs[sess.Vol.WorkspaceVol] = workdir
	}

	g, gctx = errgroup.WithContext(ctx)
	for vol, srcDir := range syncDirs {
		vol, srcDir := vol, srcDir
		g.Go(func() error {
			var err error
			if profile.ConfigIsFile && vol == sess.Vol.ConfigVol {
				err = sync.OutFile(gctx, rt, vol, "/data", srcDir)
			} else {
				err = sync.Out(gctx, rt, vol, "/data", srcDir)
			}
			if err != nil {
				return fmt.Errorf("sync-out %s: %w", srcDir, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel sync-out: %w", err)
	}

	return &ExitError{Code: exitCode}
}

// ValidateWorkdir rejects unsafe workspace paths.
// Expects an absolute path.
func shouldAllocateTTY(hasTerminalInput, shell, forceInteractive bool) bool {
	return hasTerminalInput || shell || forceInteractive
}

func isTerminalFile(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func validateSessionConfig(cfg *SessionConfig) error {
	if cfg.NoInternet && cfg.ExcludeURL != "" {
		return fmt.Errorf("--exclude-url cannot be used with --no-internet")
	}
	if cfg.CPULimit < 0 {
		return fmt.Errorf("cpu limit must be non-negative")
	}
	if cfg.MemoryLimit != "" {
		if _, err := units.RAMInBytes(cfg.MemoryLimit); err != nil {
			return fmt.Errorf("parsing memory limit %q: %w", cfg.MemoryLimit, err)
		}
	}
	pullPolicy := cfg.PullPolicy
	if pullPolicy == "" {
		pullPolicy = "never"
	}
	if pullPolicy != "never" && pullPolicy != "always" && pullPolicy != "missing" {
		return fmt.Errorf("unsupported pull policy %q: use always, missing, or never", pullPolicy)
	}
	return nil
}

func ensureRequiredImages(ctx context.Context, rt runtime.ContainerRuntime, editorImage string, cfg *SessionConfig) error {
	pullPolicy := cfg.PullPolicy
	if pullPolicy == "" {
		pullPolicy = "never"
	}
	if cfg.Offline || cfg.NoInternet {
		pullPolicy = "never"
	}

	for _, image := range []string{runtime.SyncImage, editorImage} {
		if err := ensureImage(ctx, rt, image, pullPolicy); err != nil {
			return err
		}
	}
	return nil
}

func ensureImage(ctx context.Context, rt runtime.ContainerRuntime, image, pullPolicy string) error {
	switch pullPolicy {
	case "never":
		return nil
	case "always":
		if err := rt.ImagePull(ctx, image, io.Discard); err != nil {
			return fmt.Errorf("pulling image %s: %w", image, err)
		}
		return nil
	case "missing":
		exists, err := rt.ImageExists(ctx, image)
		if err != nil {
			return fmt.Errorf("checking image %s: %w", image, err)
		}
		if exists {
			return nil
		}
		if err := rt.ImagePull(ctx, image, io.Discard); err != nil {
			return fmt.Errorf("pulling image %s: %w", image, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported pull policy %q: use always, missing, or never", pullPolicy)
	}
}

func ValidateWorkdir(abs string) error {
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("workspace %s does not exist", abs)
	}
	abs = resolved

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
