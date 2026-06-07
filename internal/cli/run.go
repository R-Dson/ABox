package cli

import (
	"bufio"
	"context"
	"errors"
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
	"github.com/r-dson/abox/internal/osutil"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/r-dson/abox/internal/sync"
	"golang.org/x/sync/errgroup"
)

// SessionConfig holds the resolved configuration for a run command.
// Cobra flags bind directly to this struct.
type SessionConfig struct {
	Editor            string
	Shell             bool
	ForceIT           bool
	Offline           bool
	StrictNetwork     bool
	NoInternet        bool
	ForceSync         bool
	ExcludeURL        string
	PullPolicy        string
	MemoryLimit       string
	CPULimit          float64
	ForwardSSHAgent   bool
	TrustWorkspaceEnv bool
	ExtraEnv          []string
	EditorArgs        []string
}

const (
	pullPolicyAlways  = "always"
	pullPolicyMissing = "missing"
	pullPolicyNever   = "never"
)

// ExitError wraps an exit code from the container process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func nonZeroExitCode(code int) int {
	if code != 0 {
		return code
	}
	return 1
}

// blockedEnvKeys are environment variables that must never be injected
// into the container from .abxenv — they control critical runtime behavior.
var blockedEnvKeys = map[string]bool{
	"HOST_UID": true, "HOST_GID": true, "SSH_AUTH_SOCK": true,
	"ABX_SESSION_ID": true, "ABX_WORKSPACE": true,
	"PATH": true, "HOME": true, "USER": true, "HOSTNAME": true,
	"SHELL": true, "UID": true, "GID": true, "PWD": true,
	"LANG": true, "TERM": true, "DISPLAY": true, "XAUTHORITY": true,
	"DBUS_SESSION_BUS_ADDRESS": true,
}

// LoadDotEnv reads a .abxenv file from the given directory.
// Returns KEY=VALUE pairs resolved from the host environment.
// Dangerous variable names (PATH, HOME, etc.) are silently skipped.
// Returns nil if the file doesn't exist.
func LoadDotEnv(dir string, trustWorkspaceEnv ...bool) ([]string, error) {
	trusted := len(trustWorkspaceEnv) > 0 && trustWorkspaceEnv[0]
	if !trusted {
		return nil, nil
	}

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

	registry, err := config.LoadEditorRegistry()
	if err != nil {
		return fmt.Errorf("loading editor registry: %w", err)
	}

	editorName := cfg.Editor
	if editorName == "" {
		editorName = config.DefaultEditor
	}

	profile, err := registry.Get(editorName)
	if err != nil {
		return fmt.Errorf("resolving editor: %w", err)
	}

	home := osutil.HomeDir()
	if err := ensureEditorDataDirs(profile, home); err != nil {
		return fmt.Errorf("preparing editor data directories: %w", err)
	}

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

	hasWorkspaceVol := matcher.HasPatterns()
	rootSpecs := []sync.RootSpec{
		{Name: "config", Path: profile.ConfigFullPath(home)},
		{Name: "cache", Path: profile.CachePath(home)},
		{Name: "state", Path: profile.StatePath(home)},
		{Name: "share", Path: profile.SharePath(home)},
	}
	if hasWorkspaceVol {
		rootSpecs = append(rootSpecs, sync.RootSpec{Name: "workspace", Path: workdir, Matcher: matcher})
	}
	snapshot, err := sync.SnapshotRoots(ctx, rootSpecs)
	if err != nil {
		return fmt.Errorf("snapshotting roots: %w", err)
	}
	snapshotByName := make(map[string]*sync.RootSnapshot, len(snapshot.Roots))
	for i := range snapshot.Roots {
		snapshotByName[snapshot.Roots[i].Name] = &snapshot.Roots[i]
	}

	sess, err := container.CreateSession(ctx, rt, profile, resolvedConfig, hasWorkspaceVol)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	preserveVolumes := make(map[string]bool)
	defer func() { sess.CleanupExcept(context.Background(), preserveVolumes) }()

	type sessionSyncRoot struct {
		name         string
		volume       string
		hostPath     string
		matcher      *exclusion.Matcher
		configIsFile bool
	}
	syncRoots := []sessionSyncRoot{
		{name: "config", volume: sess.Vol.ConfigVol, hostPath: profile.ConfigFullPath(home), configIsFile: profile.ConfigIsFile},
		{name: "cache", volume: sess.Vol.CacheVol, hostPath: profile.CachePath(home)},
		{name: "state", volume: sess.Vol.StateVol, hostPath: profile.StatePath(home)},
		{name: "share", volume: sess.Vol.ShareVol, hostPath: profile.SharePath(home)},
	}
	if sess.Vol.WorkspaceVol != "" {
		syncRoots = append(syncRoots, sessionSyncRoot{name: "workspace", volume: sess.Vol.WorkspaceVol, hostPath: workdir, matcher: matcher})
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, root := range syncRoots {
		root := root
		g.Go(func() error {
			if err := sync.In(gctx, rt, root.hostPath, root.volume, "/data", root.matcher); err != nil {
				return fmt.Errorf("sync-in %s: %w", root.hostPath, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel sync-in: %w", err)
	}

	spec, err := container.BuildSpec(profile, sess, workdir, resolvedConfig)
	if err != nil {
		return fmt.Errorf("building container spec: %w", err)
	}
	spec.Tty = shouldAllocateTTY(isTerminalFile(os.Stdin), cfg.Shell, cfg.ForceIT)

	if len(cfg.ExtraEnv) > 0 {
		mergedEnv, err := mergeEnv(spec.Env, cfg.ExtraEnv)
		if err != nil {
			return err
		}
		spec.Env = mergedEnv
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

	var rootConflicts []sync.RootConflict
	var syncOutErr error
	for _, root := range syncRoots {
		opts := sync.Options{
			Matcher:   root.matcher,
			RootName:  root.name,
			Snapshot:  snapshotByName[root.name],
			ForceSync: cfg.ForceSync,
		}
		var err error
		if root.configIsFile {
			err = sync.OutFileWithOptions(ctx, rt, root.volume, "/data", root.hostPath, opts)
		} else {
			err = sync.OutWithOptions(ctx, rt, root.volume, "/data", root.hostPath, opts)
		}
		if err == nil {
			continue
		}
		conflictErr, ok := errors.AsType[*sync.ConflictError](err)
		if !ok {
			syncOutErr = errors.Join(syncOutErr, fmt.Errorf("sync-out %s: %w", root.hostPath, err))
			continue
		}
		preserveVolumes[root.volume] = true
		rootConflicts = append(rootConflicts, conflictErr.Conflicts...)
		summary, detail := sync.FormatRootConflicts(conflictErr.Conflicts)
		slog.WarnContext(ctx, "skipping conflicted sync-out root: "+summary,
			"root", root.name,
			"volume", root.volume,
			"recovery", "docker volume ls --filter label=app=abox")
		slog.DebugContext(ctx, detail)
	}

	if syncOutErr != nil {
		return fmt.Errorf("sync-out: %w", syncOutErr)
	}
	if len(rootConflicts) > 0 {
		return &ExitError{Code: nonZeroExitCode(exitCode)}
	}
	if exitCode != 0 {
		return &ExitError{Code: exitCode}
	}
	return nil
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
	if len(cfg.ExtraEnv) > 0 {
		normalized, err := normalizeExtraEnv(cfg.ExtraEnv)
		if err != nil {
			return err
		}
		cfg.ExtraEnv = normalized
	}
	pullPolicy := cfg.PullPolicy
	if pullPolicy == "" {
		pullPolicy = pullPolicyNever
	}
	if pullPolicy != pullPolicyNever && pullPolicy != pullPolicyAlways && pullPolicy != pullPolicyMissing {
		return fmt.Errorf("unsupported pull policy %q: use always, missing, or never", pullPolicy)
	}
	return nil
}

func normalizeExtraEnv(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, raw := range values {
		key, value, hasValue := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("environment key cannot be empty")
		}
		if blockedEnvKeys[key] {
			return nil, fmt.Errorf("environment key %q is reserved", key)
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate environment key %q", key)
		}
		seen[key] = true
		if !hasValue {
			var ok bool
			value, ok = os.LookupEnv(key)
			if !ok {
				return nil, fmt.Errorf("environment key %q is not present in host environment", key)
			}
		}
		normalized = append(normalized, key+"="+value)
	}
	return normalized, nil
}

func mergeEnv(base, extra []string) ([]string, error) {
	seen := make(map[string]bool, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		seen[key] = true
		merged = append(merged, entry)
	}
	for _, entry := range extra {
		key, _, _ := strings.Cut(entry, "=")
		if seen[key] {
			return nil, fmt.Errorf("duplicate environment key %q", key)
		}
		seen[key] = true
		merged = append(merged, entry)
	}
	return merged, nil
}

func ensureRequiredImages(ctx context.Context, rt runtime.ContainerRuntime, editorImage string, cfg *SessionConfig) error {
	pullPolicy := cfg.PullPolicy
	if pullPolicy == "" {
		pullPolicy = pullPolicyNever
	}
	if cfg.Offline || cfg.NoInternet {
		pullPolicy = pullPolicyNever
	}

	for _, image := range []string{runtime.SyncImage, editorImage} {
		if err := ensureImage(ctx, rt, image, pullPolicy); err != nil {
			return err
		}
	}
	return nil
}

func ensureEditorDataDirs(profile config.EditorProfile, home string) error {
	dirs := []string{
		profile.CachePath(home),
		profile.StatePath(home),
		profile.SharePath(home),
	}
	if !profile.ConfigIsFile {
		dirs = append(dirs, profile.ConfigFullPath(home))
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

func ensureImage(ctx context.Context, rt runtime.ContainerRuntime, image, pullPolicy string) error {
	switch pullPolicy {
	case pullPolicyNever:
		return nil
	case pullPolicyAlways:
		if err := rt.ImagePull(ctx, image, io.Discard); err != nil {
			return fmt.Errorf("pulling image %s: %w", image, err)
		}
		return nil
	case pullPolicyMissing:
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
