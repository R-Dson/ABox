# ABox — Go Rewrite Design Document

**Version**: 2.0 (updated for `dev` branch)  
**Go Target**: 1.26+  
**Status**: Engineering Draft  
**Baseline**: This document reflects the `dev` branch state, where Phases 1–5 of the
architectural remediation are complete. All recommendations from the original review that
have been implemented are noted so the Go rewrite inherits them, not re-debates them.

---

## What Changed in Dev vs Original — Rewrite Baseline

Before the design, a concrete accounting of what the `dev` branch already implemented,
because the Go rewrite must start from this higher baseline, not from the `main` branch.

| Area | Dev branch implemented | Impact on Go design |
|------|------------------------|---------------------|
| Editor registry | `editors.json` expanded with `image_tag`, `config_path`, `env_vars`, `legacy_path`; `get_editor_info()` now reads JSON via `jq` — single source of truth | `EditorProfile` struct maps 1:1 to the actual JSON schema; no translation layer needed |
| Cursor removed | Removed entirely from all surfaces | 8 editors: opencode, claude, aider, copilot, vibe, goose, codex, gemini |
| Seccomp | `config/seccomp.json` shipped — real allowlist, ~100 syscalls, `defaultAction: SCMP_ACT_ERRNO` | Embed the file; apply via `HostConfig.SecurityOpt` |
| DAC_OVERRIDE | Removed from editor container; only bootstrap uses CHOWN | `HostConfig.CapAdd` in editor container is exactly `["CHOWN","SETUID","SETGID"]` |
| SSH | Agent socket forwarding implemented; `.ssh` directory only falls back if no socket | Go implementation uses the same preference logic |
| Strict network | Creates a Docker `--internal` network instead of `--add-host` blocking | `client.NetworkCreate` with `Internal: true` |
| Sync image | `SYNC_IMAGE=ghcr.io/r-dson/abox:sync` (Alpine + tar + shadow, ~5MB) for all sync/bootstrap operations | Go uses the same image constant; bootstrap and data transfer containers use sync image, not editor image |
| Transactional sync | `cp -r src/. dst.tmp/ && mv dst.tmp dst` pattern — atomic at directory level | Go uses same pattern via Docker exec: write to `.abx-tmp`, then `mv -T` |
| Streaming sync | Direct tar pipe: `tar -cf - | docker run -i ... tar -xf -` — no intermediate file | Go streaming: `io.Pipe()` connecting `archive/tar` writer to `CopyToContainer` |
| Conflict detection | `snapshot_mtimes()` + `check_conflicts()` — blocks sync-back if host changed during session | `mtime.Snapshot` + `DetectConflicts` before `SyncOut` |
| `--force-sync` flag | Overrides conflict protection | Added to `RunOptions` and CLI flags |
| `--env KEY` / `.abxenv` | Pass additional env vars per session or per workspace | Added to CLI flags; `.abxenv` parsing in workspace init |
| `--verbose` / log file | Structured log to `~/.local/state/abx/abx.log` | `slog` to file when verbose; text to stderr otherwise |
| SHA-256 installer | Installer downloads `.sha256` and verifies before executing | Go installer script unchanged in approach |
| Config format | `~/.config/abx/config.json` (JSON) replaces the `KEY=VALUE` `.conf` format; legacy still readable | Viper reads JSON; migration helper in `abx config migrate` |
| state.sh | Documents global variable contract | Replaced entirely by typed structs — no equivalent file needed |
| CI: Trivy | Vulnerability scan on all images, exits 1 on HIGH/CRITICAL | Unchanged, carry forward |
| CI: SBOM + Cosign | SBOM generated, images signed with Cosign | Unchanged, carry forward |
| CI: Daily cron | Version sync runs daily at 06:00 UTC (was every 15 min) | Unchanged, carry forward |
| CI: Selective rebuild | `sync-editors.yml` detects which editors changed; only rebuilds those | Unchanged, carry forward |
| Tests | 295 assertions: 66 registry, 16 sync, 13 exclusion, 200 fuzz | Go tests replace all of these with proper unit/integration coverage |

---

## 1. Guiding Philosophy

This is a complete rewrite — not a translation. The Bash implementation, even after `dev`
branch improvements, remains limited by the language: global variable state, pipe-delimited
return values, string-built `docker run` commands, and a bundling hack that substitutes for
a real module system. The Go rewrite eliminates all of these structurally.

**Concrete commitments:**

- No `os/exec` calls to `docker`, `tar`, `jq`, or any shell tool. Everything goes through the
  Docker Go SDK or the standard library.
- No global mutable state. All configuration flows through the call graph as explicit typed structs.
- Every external dependency — runtime, filesystem, clock — is behind an interface. All are
  mockable with zero Docker daemon involvement in unit tests.
- Errors are values carried up the call stack, wrapped with context at each boundary, and
  translated to user-facing messages only at `main`.
- The binary is a single static executable. `CGO_ENABLED=0`. One file to install.

---

## 2. Repository Layout

```
abox/
├── cmd/
│   └── abx/
│       └── main.go                    # ≤20 lines: wire and execute
├── internal/
│   ├── cli/
│   │   ├── root.go                    # Cobra root, persistent flags, slog init
│   │   ├── run.go                     # `abx [dir]` — primary command + runSession()
│   │   ├── audit.go                   # `abx audit [dir]`
│   │   ├── config.go                  # `abx config set-editor|set-exclude-url|migrate|list-editors`
│   │   └── completion.go              # `abx completion bash|zsh|fish`
│   ├── config/
│   │   ├── config.go                  # Config struct + Load() via Viper
│   │   └── registry.go                # EditorRegistry — loads embedded editors.json
│   ├── runtime/
│   │   ├── runtime.go                 # ContainerRuntime interface
│   │   ├── docker.go                  # Docker implementation (Moby SDK)
│   │   ├── podman.go                  # Podman (same interface, different socket)
│   │   └── detect.go                  # Auto-detect: Docker → Podman
│   ├── container/
│   │   ├── manager.go                 # Session orchestration: create, run, cleanup
│   │   ├── volumes.go                 # Volume lifecycle, bootstrap ownership
│   │   ├── network.go                 # Strict network: internal Docker network
│   │   ├── spec.go                    # Typed container.Config + HostConfig builder
│   │   └── session.go                 # Session struct: all ephemeral resources
│   ├── sync/
│   │   ├── syncer.go                  # Syncer interface + New()
│   │   ├── transfer.go                # Streaming tar in/out via Docker API
│   │   ├── atomic.go                  # Transactional write: stage → atomic rename
│   │   └── conflicts.go               # mtime snapshot + conflict detection
│   ├── exclusion/
│   │   ├── matcher.go                 # Pattern loading: local + remote, merge
│   │   ├── walk.go                    # fs.WalkDir with symlink resolution
│   │   └── hardcoded.go               # Always-excluded: .ssh, .aws, .env, *.pem…
│   ├── audit/
│   │   └── audit.go                   # Pre-run workspace security checks
│   └── logging/
│       └── logging.go                 # slog: text to stderr (TTY), JSON flag, file when verbose
├── config/
│   ├── editors.json                   # go:embed — single source of truth, 8 editors
│   └── seccomp/
│       └── abox-default.json          # go:embed — shipped seccomp profile from dev branch
├── docker/
│   ├── Dockerfile                     # Unchanged — editor images
│   ├── Dockerfile.sync                # Unchanged — Alpine sync image
│   └── entrypoint.sh                  # Unchanged
├── .goreleaser.yaml
├── go.mod
├── go.sum
└── Makefile
```

`cmd/abx/main.go` is the only file outside `internal/`. It wires `cli.NewRootCmd()` and
calls `Execute()`. All logic lives in `internal/`, enforced as unexportable by the Go
toolchain. No `pkg/` — this is a binary, not a library.

---

## 3. Module and Dependency Management

### go.mod

```
module github.com/r-dson/abox

go 1.26

require (
    github.com/spf13/cobra                  v2.1.0
    github.com/spf13/viper                  v2.0.0
    github.com/docker/docker                v28.x.x+incompatible
    github.com/docker/distribution          v2.x.x+incompatible
    github.com/opencontainers/image-spec    v1.x.x
    github.com/bmatcuk/doublestar/v4        v4.x.x
    golang.org/x/sync                       v0.x.x       // errgroup for parallel ops
)

require (
    github.com/stretchr/testify     v1.x.x   // test only
    go.uber.org/mock                v0.x.x   // test only — mockgen
)
```

**No indirect runtime dependencies beyond Docker SDK, Cobra/Viper, and doublestar.**

### Key dependency decisions

**Docker SDK: `github.com/docker/docker/client` (Moby client, v28+)**

`docker/go-sdk` is the newer higher-level client, but it abstracts away what ABox needs
direct control over: exact capability sets, seccomp profile injection, volume ownership
bootstrapping, and streaming tar transfer. The Moby client is what Docker's own CLI uses,
has 12,000+ importers, and exposes `container.Config`, `container.HostConfig`, and
`CopyToContainer`/`CopyFromContainer` directly.

Podman is supported by pointing the same client at the Podman socket — no separate SDK.
`runtime/detect.go` resolves the right socket; the client itself is identical.

**`bmatcuk/doublestar/v4`**: Replaces the 200-line custom fnmatch Bash engine
(`exclusion.sh`) with a single tested library. Supports `*`, `**`, `?`, `[abc]`, and
brace expansion — the same pattern space as `.abxignore`. This is what the `dev` branch's
Bash engine manually implements; doublestar does it correctly for free.

**`golang.org/x/sync/errgroup`**: Replaces `& / wait` Bash fan-out for parallel volume
creation and parallel config sync. Cancels all goroutines on the first error.

**Viper v2**: Replaces `~/.config/abx/config.json` (the new dev format). Viper reads
JSON natively, handles env var binding (`ABX_EDITOR` → `editor`), flag binding, and merge
priority (flag > env > config > default) without custom code.

---

## 4. Configuration System

### 4.1 EditorProfile — the single source of truth

`editors.json` is embedded at compile time via `//go:embed`. The `EditorProfile` struct
maps exactly to the schema shipped in the `dev` branch. Cache, state, and share paths are
derived from the editor name — consistent with how the Bash implementation derives
`HOST_CACHE="$HOME/.cache/$EDITOR_NAME"`.

```go
// internal/config/registry.go

import _ "embed"

//go:embed ../../config/editors.json
var editorsJSON []byte

// EditorProfile is the single typed representation of one row in editors.json.
// It replaces both the old hardcoded get_editor_info() case statement and the
// pipe-delimited string it returned.
type EditorProfile struct {
    Version    string   `json:"version"`
    InstallCmd string   `json:"install_cmd"`
    CmdName    string   `json:"cmd_name"`
    ImageTag   string   `json:"image_tag"`
    ConfigPath string   `json:"config_path"`  // relative to $HOME, e.g. ".claude"
    EnvVars    []string `json:"env_vars"`
    LegacyPath string   `json:"legacy_path"`  // optional; double-mount for backward compat
}

// Derived paths — not in JSON, computed from editor name, matching Bash convention.
func (p EditorProfile) CachePath(home string) string {
    return filepath.Join(home, ".cache", p.CmdName)
}
func (p EditorProfile) StatePath(home string) string {
    return filepath.Join(home, ".local", "state", p.CmdName)
}
func (p EditorProfile) SharePath(home string) string {
    return filepath.Join(home, ".local", "share", p.CmdName)
}

type editorsFile struct {
    Editors map[string]EditorProfile `json:"editors"`
}

type EditorRegistry struct {
    profiles map[string]EditorProfile
}

func LoadEditorRegistry() (*EditorRegistry, error) {
    var f editorsFile
    if err := json.Unmarshal(editorsJSON, &f); err != nil {
        return nil, fmt.Errorf("parsing embedded editors.json: %w", err)
    }
    return &EditorRegistry{profiles: f.Editors}, nil
}

func (r *EditorRegistry) Get(name string) (EditorProfile, error) {
    p, ok := r.profiles[name]
    if !ok {
        // Fallback to opencode, matching Bash behavior
        fallback, fok := r.profiles["opencode"]
        if !fok {
            return EditorProfile{}, fmt.Errorf(
                "unknown editor %q and no opencode fallback — check editors.json", name)
        }
        slog.Warn("unknown editor, falling back to opencode", "requested", name)
        return fallback, nil
    }
    return p, nil
}

func (r *EditorRegistry) Names() []string {
    names := make([]string, 0, len(r.profiles))
    for name := range r.profiles {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}
```

`//go:embed` eliminates the multi-path resolution logic from Bash (`_resolve_editors_json()`
checks env var, source tree, `/usr/local/share/abx`, alongside binary). The embedded JSON
is always present, always the version the binary was built from.

### 4.2 User config — Viper

The dev branch introduced `~/.config/abx/config.json`. The Go rewrite reads this as-is
via Viper (JSON is a first-class Viper format), adds env var and flag merge, and keeps the
legacy `~/.config/abx.conf` as a fallback during migration.

```go
// internal/config/config.go

type Config struct {
    Editor        string  `mapstructure:"editor"`
    ExcludeURL    string  `mapstructure:"exclude_url"`
    NoInternet    bool    `mapstructure:"no_internet"`
    StrictNetwork bool    `mapstructure:"strict_network"`
    PullPolicy    string  `mapstructure:"pull_policy"`   // "always"|"missing"
    MemoryLimit   string  `mapstructure:"memory_limit"`  // "4g"
    CPULimit      float64 `mapstructure:"cpu_limit"`     // 2.0
    Verbose       bool    `mapstructure:"verbose"`
    JSONLogs      bool    `mapstructure:"json_logs"`
}

func Load(v *viper.Viper) (*Config, error) {
    v.SetConfigName("config")
    v.SetConfigType("json")                                          // matches dev branch format
    v.AddConfigPath(filepath.Join(xdgConfigHome(), "abx"))          // ~/.config/abx/config.json

    v.SetEnvPrefix("ABX")      // ABX_EDITOR, ABX_NO_INTERNET, etc.
    v.AutomaticEnv()

    v.SetDefault("editor",        "opencode")
    v.SetDefault("pull_policy",   "missing")
    v.SetDefault("memory_limit",  "4g")
    v.SetDefault("cpu_limit",     2.0)

    if err := v.ReadInConfig(); err != nil {
        var notFound viper.ConfigFileNotFoundError
        if !errors.As(err, &notFound) {
            return nil, fmt.Errorf("reading config: %w", err)
        }
        // No config file is fine — legacy .conf migration runs separately
    }

    var cfg Config
    return &cfg, v.Unmarshal(&cfg)
}
```

`abx config migrate` reads the old `~/.config/abx.conf` (KEY=VALUE format) and writes
`~/.config/abx/config.json`, then removes the old file. This is a one-shot command, not
automatic — the user opts in.

---

## 5. The CLI Layer

### 5.1 Command surface

The surface matches the `dev` branch exactly, extended with the `config` subcommand:

```
abx [flags] [directory] [-- editor-args...]   # primary — run the editor
abx audit [directory]                          # pre-run security check
abx config set-editor <name>                   # persist default editor
abx config set-exclude-url <url>               # persist default exclude URL
abx config migrate                             # migrate from legacy .conf format
abx config list-editors                        # list all supported editors
abx version                                    # print version + commit
abx completion bash|zsh|fish|powershell        # shell completion
```

### 5.2 RunOptions — replacing all CLI_* globals

Every flag from the dev branch `main.sh` CLI parser maps to a typed field. No global
variables. The struct is constructed in `run.go` and passed down the call graph.

```go
// internal/cli/run.go

type RunOptions struct {
    Editor        string   // --editor
    Shell         bool     // --shell
    ForceIT       bool     // --force-it
    Offline       bool     // --offline
    StrictNetwork bool     // --strict-network
    NoInternet    bool     // --no-internet
    Verbose       bool     // --verbose
    ForceSync     bool     // --force-sync
    ExcludeURL    string   // --exclude-url
    ExtraEnv      []string // --env KEY (repeatable)
    EditorArgs    []string // -- arg1 arg2 ...
}
```

`--force-sync` is carried from the dev branch: when host files were modified during
the session, the default is to block sync-back with a warning. `--force-sync` overrides
this check.

### 5.3 Root command

```go
// internal/cli/root.go

func NewRootCmd(version string) *cobra.Command {
    root := &cobra.Command{
        Use:               "abx",
        Short:             "Secure sandbox for AI coding editors",
        SilenceUsage:      true,   // no usage dump on every error
        SilenceErrors:     true,   // main.go prints the error, not Cobra
        PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
            verbose, _ := cmd.Flags().GetBool("verbose")
            jsonLogs, _ := cmd.Flags().GetBool("json-logs")
            logging.Setup(verbose, jsonLogs)
            return nil
        },
    }

    root.PersistentFlags().Bool("verbose",   false, "enable debug logging to ~/.local/state/abx/abx.log")
    root.PersistentFlags().Bool("json-logs", false, "emit JSON structured logs to stderr")

    // abx with no subcommand is the run command
    root.RunE = newRunCmd().RunE

    root.AddCommand(
        newRunCmd(),
        newAuditCmd(),
        newConfigCmd(),
        newVersionCmd(version),
        newCompletionCmd(root),
    )
    return root
}
```

`SilenceUsage: true` and `SilenceErrors: true` together ensure Cobra never dumps usage
on error and never double-prints error messages. `main.go` owns all output:

```go
// cmd/abx/main.go

func main() {
    root := cli.NewRootCmd(version.String())
    if err := root.ExecuteContext(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "abx: %v\n", err)
        os.Exit(1)
    }
}
```

### 5.4 runSession — the orchestrator

```go
// internal/cli/run.go

func runSession(ctx context.Context, workdir string, opts RunOptions) error {
    cfg, err := config.Load(viper.GetViper())
    if err != nil {
        return err
    }
    applyFlags(cfg, opts)   // CLI flags override config values

    registry, err := config.LoadEditorRegistry()
    if err != nil {
        return err
    }
    profile, err := registry.Get(cfg.Editor)
    if err != nil {
        return err
    }

    rt, err := runtime.Detect(ctx)
    if err != nil {
        return fmt.Errorf("no container runtime — install Docker or Podman: %w", err)
    }

    if err := validateWorkdir(workdir); err != nil {   // block $HOME, /
        return err
    }

    matcher, err := exclusion.BuildMatcher(ctx, workdir, cfg.ExcludeURL)
    if err != nil {
        return err
    }

    mgr := container.NewManager(rt)
    session, err := mgr.NewSession(ctx, profile)
    if err != nil {
        return err
    }
    defer session.Cleanup(context.Background())  // background ctx: cleanup even if cancelled

    syncer := sync.New(rt)

    // Snapshot host mtimes before session — enables conflict detection on sync-back
    snapshot, err := sync.SnapshotMtimes(profile, workdir)
    if err != nil {
        return err
    }

    if err := syncer.SyncIn(ctx, session, profile, workdir, matcher); err != nil {
        return fmt.Errorf("syncing into sandbox: %w", err)
    }

    exitCode, err := mgr.Run(ctx, session, profile, workdir, cfg, opts)
    if err != nil {
        return err
    }

    // Conflict check: abort sync-back if host changed during session, unless forced
    if conflicts := snapshot.DetectConflicts(); len(conflicts) > 0 && !opts.ForceSync {
        slog.WarnContext(ctx, "host files changed during session — sync-back blocked",
            "count", len(conflicts),
            "files", conflicts,
            "hint", "re-run with --force-sync to overwrite")
        os.Exit(exitCode)
    }

    if err := syncer.SyncOut(ctx, session, profile, workdir, matcher); err != nil {
        return fmt.Errorf("syncing back to host: %w", err)
    }

    if exitCode != 0 {
        os.Exit(exitCode)
    }
    return nil
}
```

---

## 6. Container Runtime Abstraction

### 6.1 Interface

```go
// internal/runtime/runtime.go

// ContainerRuntime abstracts Docker and Podman. No subprocess exec. No shell.
type ContainerRuntime interface {
    // Volumes
    VolumeCreate(ctx context.Context, name string, labels map[string]string) error
    VolumeRemove(ctx context.Context, name string, force bool) error

    // Networks
    NetworkCreate(ctx context.Context, name string, internal bool) (string, error)
    NetworkRemove(ctx context.Context, id string) error

    // Containers
    ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error)
    ContainerStart(ctx context.Context, id string) error
    ContainerWait(ctx context.Context, id string) (int64, error)
    ContainerRemove(ctx context.Context, id string, force bool) error
    ContainerAttach(ctx context.Context, id string) (types.HijackedResponse, error)
    ContainerExec(ctx context.Context, id string, cmd []string) (int64, error)

    // Data transfer — streaming tar, no temp files, no intermediate containers
    CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error
    CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error)

    // Images
    ImagePull(ctx context.Context, ref string, out io.Writer) error
    ImageExists(ctx context.Context, ref string) (bool, error)

    Ping(ctx context.Context) error
}
```

`NetworkCreate` and `NetworkRemove` are new relative to the v1.0 design document,
reflecting the `dev` branch implementation of strict network mode via an internal
Docker network rather than `--add-host` DNS blocking.

### 6.2 Docker implementation

```go
// internal/runtime/docker.go

type dockerRuntime struct {
    client *dockerclient.Client
}

func NewDocker(ctx context.Context) (*dockerRuntime, error) {
    cli, err := dockerclient.NewClientWithOpts(
        dockerclient.FromEnv,
        dockerclient.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, err
    }
    if err := cli.Ping(ctx); err != nil {
        return nil, fmt.Errorf("Docker daemon unreachable: %w", err)
    }
    return &dockerRuntime{client: cli}, nil
}

func (d *dockerRuntime) NetworkCreate(ctx context.Context, name string, internal bool) (string, error) {
    resp, err := d.client.NetworkCreate(ctx, name, networktypes.CreateOptions{
        Internal: internal,    // blocks all external routing when true
        Labels:   map[string]string{"app": "abox"},
    })
    return resp.ID, err
}

func (d *dockerRuntime) CopyToContainer(ctx context.Context, id, dstPath string, content io.Reader) error {
    return d.client.CopyToContainer(ctx, id, dstPath, content,
        dockertypes.CopyToContainerOptions{AllowOverwriteDirWithFile: false})
}

func (d *dockerRuntime) CopyFromContainer(ctx context.Context, id, srcPath string) (io.ReadCloser, error) {
    reader, _, err := d.client.CopyFromContainer(ctx, id, srcPath)
    return reader, err
}
```

### 6.3 Podman implementation

Podman's REST API is Docker-compatible at the socket level. The implementation is
identical to Docker's except for socket resolution:

```go
// internal/runtime/podman.go

func NewPodman(ctx context.Context) (*dockerRuntime, error) {
    sock := podmanSocket()   // /run/user/{uid}/podman/podman.sock
    cli, err := dockerclient.NewClientWithOpts(
        dockerclient.WithHost("unix://"+sock),
        dockerclient.WithAPIVersionNegotiation(),
    )
    ...
}

func podmanSocket() string {
    if s := os.Getenv("DOCKER_HOST"); s != "" {
        return strings.TrimPrefix(s, "unix://")
    }
    return fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid())
}
```

### 6.4 Detection

```go
// internal/runtime/detect.go

func Detect(ctx context.Context) (ContainerRuntime, error) {
    // Respect explicit override (matches ABOX_RUNTIME env var behavior from Bash)
    if name := os.Getenv("ABOX_RUNTIME"); name != "" {
        switch name {
        case "docker":
            return NewDocker(ctx)
        case "podman":
            return NewPodman(ctx)
        default:
            return nil, fmt.Errorf("unknown runtime %q: must be docker or podman", name)
        }
    }
    if rt, err := NewDocker(ctx); err == nil {
        slog.DebugContext(ctx, "runtime: docker")
        return rt, nil
    }
    if rt, err := NewPodman(ctx); err == nil {
        slog.DebugContext(ctx, "runtime: podman")
        return rt, nil
    }
    return nil, errors.New("neither Docker nor Podman is available or healthy")
}
```

---

## 7. Container Spec Construction

The `ContainerSpec` is built from `EditorProfile` + session + config. This replaces the
string-building functions in `container.sh` (`build_env_flags`, `build_config_mounts`,
`build_workspace_mount`, `get_interactive_flags`, `run_container`) with typed Go that the
compiler validates entirely.

### 7.1 Seccomp profile — embedded

```go
// internal/container/spec.go

import _ "embed"

//go:embed ../../config/seccomp/abox-default.json
var seccompProfile []byte

var seccompTempPath = sync.OnceValue(func() string {
    f, _ := os.CreateTemp("", "abox-seccomp-*.json")
    f.Write(seccompProfile)
    f.Close()
    return f.Name()
})
```

`sync.OnceValue` (Go 1.21+) ensures the temp file is written exactly once per process
lifetime, not once per container. The Docker seccomp API requires a filesystem path, not
inline JSON, so we materialize it lazily.

### 7.2 BuildSpec

```go
func BuildSpec(profile config.EditorProfile, session *Session,
    workdir string, cfg *config.Config, opts cli.RunOptions) ContainerSpec {

    hostConfig := &container.HostConfig{
        Binds: buildBinds(profile, session, workdir, opts),
        CapDrop: strslice.StrSlice{"ALL"},
        CapAdd:  strslice.StrSlice{"CHOWN", "SETUID", "SETGID"},  // no DAC_OVERRIDE
        SecurityOpt: []string{
            "no-new-privileges",
            "seccomp=" + seccompTempPath(),
        },
        Resources: container.Resources{
            Memory:   parseMemoryBytes(cfg.MemoryLimit),
            NanoCPUs: int64(cfg.CPULimit * 1e9),
        },
        NetworkMode: resolveNetworkMode(session, cfg),
        AutoRemove:  true,
    }

    containerConfig := &container.Config{
        Image:       profile.ImageTag,
        Cmd:         buildCmd(profile, opts),
        Env:         buildEnv(profile, opts, session),
        Tty:         true,
        OpenStdin:   true,
        AttachStdin: true,
        WorkingDir:  "/workspace",
    }

    return ContainerSpec{
        Container: containerConfig,
        Host:      hostConfig,
        Name:      sessionContainerName(profile, workdir, session.ID),
    }
}
```

### 7.3 Environment — including --env and .abxenv

```go
func buildEnv(profile config.EditorProfile, opts cli.RunOptions, session *Session) []string {
    env := []string{
        fmt.Sprintf("HOST_UID=%d", os.Getuid()),
        fmt.Sprintf("HOST_GID=%d", os.Getgid()),
    }

    // Editor-defined env vars (from editors.json EnvVars field)
    for _, key := range profile.EnvVars {
        if val, ok := os.LookupEnv(key); ok {
            env = append(env, key+"="+val)
        }
    }

    // --env KEY flags (pass through named host env vars)
    for _, key := range opts.ExtraEnv {
        if val, ok := os.LookupEnv(key); ok {
            env = append(env, key+"="+val)
        }
    }

    // SSH agent socket forwarding — preferred over ~/.ssh directory mount
    if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
        if _, err := os.Stat(sock); err == nil {
            env = append(env, "SSH_AUTH_SOCK=/tmp/ssh-agent.sock")
        }
    }

    return env
}
```

`.abxenv` parsing happens before `BuildSpec` in `runSession`. The file contains lines of
`KEY=value` or bare `KEY` (bare means "pass through from host env"). Parsed values are
appended to `opts.ExtraEnv` before spec construction.

```go
// internal/cli/run.go

func loadDotEnv(workdir string) []string {
    path := filepath.Join(workdir, ".abxenv")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil
    }
    var keys []string
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        // Accept bare KEY or KEY=value — we only pass the key name through
        key, _, _ := strings.Cut(line, "=")
        keys = append(keys, strings.TrimSpace(key))
    }
    return keys
}
```

### 7.4 Volume mounts — SSH socket, gitconfig, Claude skills

```go
func buildBinds(profile config.EditorProfile, session *Session,
    workdir string, opts cli.RunOptions) []string {

    binds := buildVolumeMounts(profile, session)   // config/cache/state/share volumes

    // Workspace: volume (exclusions active) or direct bind (no exclusions)
    if session.WorkspaceVol != "" {
        binds = append(binds, session.WorkspaceVol+":/workspace")
    } else {
        binds = append(binds, workdir+":/workspace")
    }

    // gitconfig — read-only
    if gc := filepath.Join(os.Getenv("HOME"), ".gitconfig"); fileExists(gc) {
        binds = append(binds, gc+":/home/agent/.gitconfig:ro,z")
    }

    // SSH: prefer agent socket forwarding; fall back to .ssh directory mount
    if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
        if _, err := os.Stat(sock); err == nil {
            binds = append(binds, sock+":/tmp/ssh-agent.sock:ro")
        }
    } else if sshDir := filepath.Join(os.Getenv("HOME"), ".ssh"); dirExists(sshDir) {
        binds = append(binds, sshDir+":/home/agent/.ssh:ro,z")
    }

    return binds
}
```

---

## 8. Volume and Session Management

### 8.1 Session

```go
// internal/container/session.go

const SyncImage = "ghcr.io/r-dson/abox:sync"   // Alpine + tar + shadow; matches SYNC_IMAGE in Bash

type Session struct {
    ID           string
    ConfigVol    string
    CacheVol     string
    StateVol     string
    ShareVol     string
    WorkspaceVol string   // empty unless exclusions active
    NetworkID    string   // empty unless --strict-network
    runtime      runtime.ContainerRuntime
}

func (s *Session) Cleanup(ctx context.Context) {
    if s.NetworkID != "" {
        if err := s.runtime.NetworkRemove(ctx, s.NetworkID); err != nil {
            slog.WarnContext(ctx, "cleanup: network remove failed", "id", s.NetworkID, "error", err)
        }
    }
    for _, vol := range s.volumeNames() {
        if vol == "" {
            continue
        }
        if err := s.runtime.VolumeRemove(ctx, vol, true); err != nil {
            slog.WarnContext(ctx, "cleanup: volume remove failed", "volume", vol, "error", err)
        }
    }
}
```

### 8.2 Volume creation

```go
// internal/container/volumes.go

func (m *Manager) NewSession(ctx context.Context, profile config.EditorProfile,
    cfg *config.Config, matcher *exclusion.Matcher) (*Session, error) {

    id := strconv.FormatInt(time.Now().UnixNano(), 10)
    session := &Session{
        ID:        id,
        ConfigVol: "abox-config-" + id,
        CacheVol:  "abox-cache-" + id,
        StateVol:  "abox-state-" + id,
        ShareVol:  "abox-share-" + id,
        runtime:   m.runtime,
    }
    if matcher.HasPatterns() {
        session.WorkspaceVol = "abox-workspace-" + id
    }

    labels := map[string]string{
        "app": "abox", "editor": profile.CmdName, "session": id,
    }

    // Create volumes in parallel
    g, gctx := errgroup.WithContext(ctx)
    for _, name := range session.volumeNames() {
        name := name
        g.Go(func() error { return m.runtime.VolumeCreate(gctx, name, labels) })
    }
    if err := g.Wait(); err != nil {
        session.Cleanup(ctx)
        return nil, fmt.Errorf("creating volumes: %w", err)
    }

    // Bootstrap ownership using the sync image (not the editor image)
    if err := m.bootstrapOwnership(ctx, session); err != nil {
        session.Cleanup(ctx)
        return nil, err
    }

    // Create strict network if requested
    if cfg.StrictNetwork {
        netID, err := m.runtime.NetworkCreate(ctx, "abox-strict-"+id, true)
        if err != nil {
            session.Cleanup(ctx)
            return nil, fmt.Errorf("creating strict network: %w", err)
        }
        session.NetworkID = netID
    }

    return session, nil
}
```

### 8.3 Volume ownership bootstrap

Uses `SyncImage` (Alpine), not the full editor image. Matches the `dev` branch behavior
where `SYNC_IMAGE` is used in `init_volume_ownership`.

```go
func (m *Manager) bootstrapOwnership(ctx context.Context, session *Session) error {
    uid, gid := os.Getuid(), os.Getgid()
    targets := strings.Join(volumeMountPaths(session), " ")

    spec := ContainerSpec{
        Container: &container.Config{
            Image: SyncImage,
            Cmd:   strslice.StrSlice{"sh", "-c", fmt.Sprintf("chown -R %d:%d %s", uid, gid, targets)},
            User:  "0:0",
        },
        Host: &container.HostConfig{
            Binds:      volumeBootstrapBinds(session),
            AutoRemove: true,
            CapDrop:    strslice.StrSlice{"ALL"},
            CapAdd:     strslice.StrSlice{"CHOWN"},
        },
        Name: "abox-bootstrap-" + session.ID,
    }

    id, err := m.runtime.ContainerCreate(ctx, spec)
    if err != nil {
        return fmt.Errorf("bootstrap create: %w", err)
    }
    if err := m.runtime.ContainerStart(ctx, id); err != nil {
        return err
    }
    code, err := m.runtime.ContainerWait(ctx, id)
    if err != nil || code != 0 {
        return fmt.Errorf("bootstrap exited %d: %w", code, err)
    }
    return nil
}
```

---

## 9. The Sync System

This is the most consequential redesign from Bash to Go. The dev branch already resolved
most sync issues at the Bash level (streaming, transactional staging, SYNC_IMAGE). The Go
rewrite re-expresses these solutions using the Docker SDK directly — no intermediate
containers for sync, no extra process startups.

### 9.1 SyncIn — parallel, streaming, transactional

```go
// internal/sync/transfer.go

const SyncImage = container.SyncImage   // single constant, not repeated

type Syncer struct {
    runtime runtime.ContainerRuntime
}

func (s *Syncer) SyncIn(ctx context.Context, session *container.Session,
    profile config.EditorProfile, workdir string, matcher *exclusion.Matcher) error {

    home, _ := os.UserHomeDir()

    type transfer struct {
        src string
        vol string
        dst string   // path inside the volume container
    }

    transfers := []transfer{
        {filepath.Join(home, profile.ConfigPath), session.ConfigVol, "/data"},
        {profile.CachePath(home),                 session.CacheVol,  "/data"},
        {profile.StatePath(home),                 session.StateVol,  "/data"},
        {profile.SharePath(home),                 session.ShareVol,  "/data"},
    }

    g, gctx := errgroup.WithContext(ctx)

    for _, t := range transfers {
        t := t
        g.Go(func() error {
            return s.transferDirToVolume(gctx, t.src, t.vol, t.dst)
        })
    }

    if matcher.HasPatterns() {
        g.Go(func() error {
            return s.transferWorkspaceToVolume(gctx, workdir, session.WorkspaceVol, matcher)
        })
    }

    return g.Wait()
}
```

### 9.2 Streaming tar — no temp files, no intermediate containers

The dev branch streams: `tar -cf - | docker run -i ... tar -xf -`. The Go equivalent
uses `CopyToContainer` which accepts an `io.Reader` of tar data — the Docker daemon
receives the stream directly over the API socket. No temp file. No extra container.

```go
func (s *Syncer) transferDirToVolume(ctx context.Context, srcDir, volumeName, dstPath string) error {
    if _, err := os.Stat(srcDir); os.IsNotExist(err) {
        return nil  // source doesn't exist yet — first run; skip silently
    }

    // Mount the volume in a short-lived sync container
    containerID, cleanup, err := s.mountVolumeContainer(ctx, volumeName)
    if err != nil {
        return err
    }
    defer cleanup()

    // Write to a staging path inside the container, then atomically rename
    stagingPath := dstPath + ".abx-tmp"

    pr, pw := io.Pipe()
    var tarErr error
    go func() {
        defer pw.Close()
        tarErr = tarDir(srcDir, pw)
    }()

    if err := s.runtime.CopyToContainer(ctx, containerID, stagingPath, pr); err != nil {
        return fmt.Errorf("streaming %s to volume: %w", srcDir, err)
    }
    if tarErr != nil {
        return fmt.Errorf("archiving %s: %w", srcDir, tarErr)
    }

    // Atomic rename inside the container — same filesystem, guaranteed atomic
    if _, err := s.runtime.ContainerExec(ctx, containerID,
        []string{"mv", "-T", stagingPath, dstPath}); err != nil {
        return fmt.Errorf("atomic rename in volume: %w", err)
    }
    return nil
}
```

`io.Pipe()` connects the `archive/tar` writer goroutine directly to `CopyToContainer`'s
HTTP request body: host FS → tar encoder → pipe → HTTP → Docker daemon → volume.
One pass. Zero I/O amplification.

### 9.3 Workspace sync with exclusion filtering

```go
func (s *Syncer) transferWorkspaceToVolume(ctx context.Context,
    workdir, volumeName string, matcher *exclusion.Matcher) error {

    containerID, cleanup, err := s.mountVolumeContainer(ctx, volumeName)
    if err != nil {
        return err
    }
    defer cleanup()

    pr, pw := io.Pipe()
    var walkErr error
    go func() {
        defer pw.Close()
        walkErr = tarFiltered(workdir, pw, matcher)   // walks, resolves symlinks, applies patterns
    }()

    // Stream directly to workspace volume — no intermediate file on host
    if err := s.runtime.CopyToContainer(ctx, containerID, "/data", pr); err != nil {
        return fmt.Errorf("streaming workspace to volume: %w", err)
    }
    return walkErr
}
```

`tarFiltered` uses `fs.WalkDir` with `filepath.EvalSymlinks` on each entry before
matching — closing the symlink bypass vulnerability from the original review.

### 9.4 Conflict detection

Mirrors the `snapshot_mtimes` / `check_conflicts` functions from the dev branch, but
as typed Go with no temp files in `/tmp`.

```go
// internal/sync/conflicts.go

type MtimeSnapshot struct {
    entries map[string]time.Time   // absolute path → mtime at snapshot time
}

func SnapshotMtimes(profile config.EditorProfile, workdir string) (*MtimeSnapshot, error) {
    home, _ := os.UserHomeDir()
    snap := &MtimeSnapshot{entries: make(map[string]time.Time)}

    dirs := []string{
        filepath.Join(home, profile.ConfigPath),
        profile.CachePath(home),
        profile.StatePath(home),
        profile.SharePath(home),
    }

    for _, dir := range dirs {
        if err := snap.walkDir(dir); err != nil {
            return nil, err
        }
    }
    return snap, nil
}

func (s *MtimeSnapshot) DetectConflicts() []string {
    var conflicts []string
    for path, originalMtime := range s.entries {
        info, err := os.Stat(path)
        if err != nil {
            continue
        }
        if !info.ModTime().Equal(originalMtime) {
            conflicts = append(conflicts, path)
        }
    }
    sort.Strings(conflicts)
    return conflicts
}
```

No `/tmp/abx_mtimes_$VOL_ID` file. The snapshot lives in a Go struct for the duration
of the session and is GC'd when `runSession` returns.

---

## 10. Exclusion Engine

The 200-line Bash fnmatch engine is replaced by `bmatcuk/doublestar/v4`.

```go
// internal/exclusion/matcher.go

type Matcher struct {
    patterns []string
}

// BuildMatcher composes patterns from three sources in priority order:
//  1. Hardcoded security patterns (always applied, highest priority)
//  2. Local .abxignore in the workspace
//  3. Remote URL patterns (fetched over HTTPS)
func BuildMatcher(ctx context.Context, workdir, remoteURL string) (*Matcher, error) {
    patterns := hardcoded.Patterns()   // .ssh, .aws, .env, .gnupg, **/*key, **/*.pem

    if local, err := loadLocalIgnore(workdir); err == nil {
        patterns = mergePatterns(patterns, local)
    }

    if remoteURL != "" {
        if remote, err := fetchRemotePatterns(ctx, remoteURL); err != nil {
            slog.WarnContext(ctx, "remote patterns unavailable, continuing without",
                "url", remoteURL, "error", err)
        } else {
            patterns = mergePatterns(patterns, remote)
        }
    }

    return &Matcher{patterns: patterns}, nil
}

func (m *Matcher) Match(path string) bool {
    for _, pattern := range m.patterns {
        if ok, _ := doublestar.Match(pattern, path); ok {
            return true
        }
    }
    return false
}

func (m *Matcher) HasPatterns() bool { return len(m.patterns) > 0 }
```

```go
// internal/exclusion/walk.go

// Walk returns only paths under root that are NOT excluded.
// Symlinks are resolved before pattern matching — closes the symlink bypass.
func (m *Matcher) Walk(root string) ([]string, error) {
    var included []string
    err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return nil  // skip unreadable entries
        }
        abs := filepath.Join(root, path)

        // Resolve symlinks before matching
        real, err := filepath.EvalSymlinks(abs)
        if err != nil {
            return nil  // broken symlink — skip
        }
        relReal, _ := filepath.Rel(root, real)

        if m.Match(path) || m.Match(relReal) {
            if d.IsDir() {
                return fs.SkipDir
            }
            return nil
        }
        if !d.IsDir() {
            included = append(included, path)
        }
        return nil
    })
    return included, err
}
```

---

## 11. Strict Network Mode

The dev branch implements strict network via `docker network create --internal`, which
blocks all external routing at the kernel level — not at DNS resolution. The Go
implementation calls the SDK directly:

```go
// internal/container/network.go

func (m *Manager) setupStrictNetwork(ctx context.Context, session *Session) error {
    id, err := m.runtime.NetworkCreate(ctx, "abox-strict-"+session.ID, true)
    if err != nil {
        return fmt.Errorf("creating strict network: %w", err)
    }
    session.NetworkID = id
    return nil
}

func resolveNetworkMode(session *Session, cfg *config.Config) container.NetworkMode {
    if cfg.NoInternet {
        return "none"
    }
    if session.NetworkID != "" {
        return container.NetworkMode(session.NetworkID)
    }
    return ""   // default bridge
}
```

An `--internal` Docker network prevents containers from communicating with any external
network regardless of how routing or DNS resolves. This is materially stronger than the
old `--add-host` approach.

---

## 12. Audit Subcommand

```go
// internal/audit/audit.go

type Result struct {
    Checks []Check
}

type Check struct {
    Name    string
    Status  Status   // OK | Warn | Critical
    Message string
}

type Status int
const (
    OK       Status = iota
    Warn
    Critical
)

func Run(ctx context.Context, workdir string, rt runtime.ContainerRuntime) (*Result, error) {
    result := &Result{}

    result.add(checkSensitiveFiles(workdir))
    result.add(checkAbxIgnore(workdir))
    result.add(checkRuntime(ctx, rt))
    result.add(checkWorkdirSafety(workdir))
    result.add(checkSeccompProfile())

    return result, nil
}
```

The audit command returns a typed `Result` that the CLI renders. Replacing the `echo`
chain in `audit.sh` with a typed result enables future programmatic use (e.g. `--json`
output for scripting) without changing the logic.

---

## 13. Logging

```go
// internal/logging/logging.go

func Setup(verbose, jsonOutput bool) {
    level := slog.LevelInfo
    if verbose {
        level = slog.LevelDebug
    }

    // File handler for verbose mode — matches dev branch ~.local/state/abx/abx.log
    var handlers []slog.Handler

    if verbose {
        logDir := filepath.Join(xdgStateHome(), "abx")
        os.MkdirAll(logDir, 0o700)
        f, err := os.OpenFile(
            filepath.Join(logDir, "abx.log"),
            os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
        if err == nil {
            handlers = append(handlers, slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
        }
    }

    // Stderr handler — JSON when requested or not a TTY
    if jsonOutput || !isTerminal(os.Stderr) {
        handlers = append(handlers, slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
    } else {
        handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
    }

    slog.SetDefault(slog.New(multiHandler(handlers)))
}
```

`log/slog` is stdlib since Go 1.21. Text to stderr for interactive TTY. JSON to stderr
for `--json-logs`, CI, or when stderr is not a terminal. File handler at
`~/.local/state/abx/abx.log` when `--verbose` — matching the dev branch path.

---

## 14. Error Handling

Every boundary wraps with context:

```go
return nil, fmt.Errorf("creating bootstrap container: %w", err)
return nil, fmt.Errorf("syncing workspace into sandbox: %w", err)
```

The final chain reaching `main` is readable without a stack trace:
`"syncing workspace into sandbox: streaming .claude to volume: context deadline exceeded"`

Error types are used only when callers branch on kind:

```go
type UnsafeWorkdirError struct {
    Path   string
    Reason string
}

func (e *UnsafeWorkdirError) Error() string {
    return fmt.Sprintf("refusing to run in %s: %s", e.Path, e.Reason)
}
```

No `log.Fatal`. No `panic`. No `os.Exit` except in `main.go` (for exit code propagation).

---

## 15. Testing Strategy

### 15.1 Unit tests — table-driven, interface-mocked

```go
// internal/exclusion/matcher_test.go

func TestMatcher_Match(t *testing.T) {
    tests := []struct {
        name    string
        pattern string
        path    string
        want    bool
    }{
        {"exact",           ".env",        ".env",               true},
        {"glob star",       "*.pem",       "server.pem",         true},
        {"doublestar",      "**/*.key",    "secrets/prod.key",   true},
        {"root-anchored",   "/.ssh",       ".ssh",               true},
        {"dir skip",        "node_modules/", "node_modules/left-pad/index.js", true},
        {"symlink resolved","**.ssh**",    ".ssh/id_rsa",        true},
        {"no match",        ".env",        "main.go",            false},
        // Fuzz-derived cases — ported from exclusion-fuzz.sh findings
        {"brace expand",    "{.env,.aws}", ".aws",               true},
        {"question mark",   "?.env",      "a.env",              true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := &Matcher{patterns: []string{tt.pattern}}
            assert.Equal(t, tt.want, m.Match(tt.path))
        })
    }
}
```

### 15.2 Mocking the runtime

```go
//go:generate go run go.uber.org/mock/mockgen \
//   -destination=internal/runtime/mock_runtime_test.go \
//   -package=runtime_test \
//   github.com/r-dson/abox/internal/runtime ContainerRuntime
```

```go
// internal/container/volumes_test.go

func TestNewSession_CreatesAllVolumes(t *testing.T) {
    ctrl := gomock.NewController(t)
    mock := mock_runtime.NewMockContainerRuntime(ctrl)

    // Four volume creates in parallel — order undefined, use AnyTimes with regex
    mock.EXPECT().VolumeCreate(gomock.Any(), gomock.MatchRegexp("abox-config-.*"), gomock.Any()).Return(nil)
    mock.EXPECT().VolumeCreate(gomock.Any(), gomock.MatchRegexp("abox-cache-.*"),  gomock.Any()).Return(nil)
    mock.EXPECT().VolumeCreate(gomock.Any(), gomock.MatchRegexp("abox-state-.*"),  gomock.Any()).Return(nil)
    mock.EXPECT().VolumeCreate(gomock.Any(), gomock.MatchRegexp("abox-share-.*"),  gomock.Any()).Return(nil)

    // Bootstrap sequence
    mock.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).Return("bootstrap-id", nil)
    mock.EXPECT().ContainerStart(gomock.Any(), "bootstrap-id").Return(nil)
    mock.EXPECT().ContainerWait(gomock.Any(), "bootstrap-id").Return(int64(0), nil)

    session, err := NewManager(mock).NewSession(context.Background(),
        testProfile(), testConfig(), exclusion.EmptyMatcher())
    require.NoError(t, err)
    assert.NotEmpty(t, session.ID)
}
```

### 15.3 Sync unit tests — mirrors dev branch sync-unit-test.sh

The `dev` branch `sync-unit-test.sh` verifies 16 properties including: no intermediate
tar files, atomicity, hardcoded exclusion coverage, conflict detection logic. These
become proper Go tests:

```go
// internal/sync/transfer_test.go

func TestSyncIn_NoIntermediateTarFiles(t *testing.T) {
    // After SyncIn, /tmp must contain no abox_sync_*.tar files
    before, _ := filepath.Glob("/tmp/abox_sync_*.tar")

    mock := setupMockRuntime(t)
    s := New(mock)
    s.SyncIn(context.Background(), testSession(), testProfile(), t.TempDir(), exclusion.EmptyMatcher())

    after, _ := filepath.Glob("/tmp/abox_sync_*.tar")
    assert.Equal(t, len(before), len(after), "sync must not create intermediate tar files in /tmp")
}

func TestSyncIn_TransactionalWrite(t *testing.T) {
    // CopyToContainer must be called with a staging path (.abx-tmp), followed
    // by ContainerExec mv — verify call order with InOrder mock
    ctrl := gomock.NewController(t)
    mock := mock_runtime.NewMockContainerRuntime(ctrl)

    gomock.InOrder(
        mock.EXPECT().CopyToContainer(gomock.Any(), gomock.Any(),
            gomock.MatchRegexp(`.*\.abx-tmp$`), gomock.Any()).Return(nil),
        mock.EXPECT().ContainerExec(gomock.Any(), gomock.Any(),
            gomock.Eq([]string{"mv", "-T", gomock.Any(), gomock.Any()})).Return(int64(0), nil),
    )
    ...
}
```

### 15.4 Editor registry tests — mirrors editor-registry-test.sh (66 tests)

```go
// internal/config/registry_test.go

func TestEditorRegistry_AllEditors(t *testing.T) {
    registry, err := LoadEditorRegistry()
    require.NoError(t, err)

    expectedEditors := []string{"aider", "claude", "codex", "copilot", "gemini", "goose", "opencode", "vibe"}
    assert.Equal(t, expectedEditors, registry.Names())

    // Verify cursor is absent (removed in dev branch)
    _, err = registry.Get("cursor")
    assert.NoError(t, err)  // falls back to opencode — not an error
}

func TestEditorRegistry_FieldsCorrect(t *testing.T) {
    tests := []struct {
        editor     string
        imageTag   string
        cmdName    string
        configPath string
        envVars    []string
        legacy     string
    }{
        {"claude",   "ghcr.io/r-dson/abox:claude",   "claude",   ".claude",        []string{"ANTHROPIC_API_KEY"}, ""},
        {"opencode", "ghcr.io/r-dson/abox:opencode", "opencode", ".config/opencode",[]string{},                  ".opencode"},
        {"aider",    "ghcr.io/r-dson/abox:aider",    "aider",    ".aider.conf.yml", []string{"OPENAI_API_KEY","ANTHROPIC_API_KEY"}, ""},
        // … all 8 editors
    }

    registry, _ := LoadEditorRegistry()
    for _, tt := range tests {
        t.Run(tt.editor, func(t *testing.T) {
            p, err := registry.Get(tt.editor)
            require.NoError(t, err)
            assert.Equal(t, tt.imageTag,   p.ImageTag)
            assert.Equal(t, tt.cmdName,    p.CmdName)
            assert.Equal(t, tt.configPath, p.ConfigPath)
            assert.Equal(t, tt.envVars,    p.EnvVars)
            assert.Equal(t, tt.legacy,     p.LegacyPath)
        })
    }
}
```

### 15.5 Integration tests — testcontainers-go

```go
//go:build integration

func TestFullSession_WorkspaceExclusionsApplied(t *testing.T) {
    ctx := context.Background()

    // Real workspace with a secret file
    workspace := t.TempDir()
    os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main"), 0o644)
    os.WriteFile(filepath.Join(workspace, ".env"),    []byte("SECRET=value"), 0o600)

    // .abxignore excludes .env
    os.WriteFile(filepath.Join(workspace, ".abxignore"), []byte(".env\n"), 0o644)

    matcher, _ := exclusion.BuildMatcher(ctx, workspace, "")

    // Verify .env is excluded, main.go is not
    included, err := matcher.Walk(workspace)
    require.NoError(t, err)
    assert.Contains(t, included, "main.go")
    assert.NotContains(t, included, ".env")
}
```

### 15.6 Coverage enforcement

```yaml
# .coverage.yaml
profile: coverage.out
local-prefix: github.com/r-dson/abox
threshold:
  file:    70
  package: 80
  total:   85
```

Coverage gate runs in CI on every push. The threshold enforces the metric that the
dev branch entirely lacked — every `sync-unit-test.sh` PASS becomes a genuine unit
test counted toward the 85% total threshold.

---

## 16. Build and Distribution

### 16.1 Embedded assets

```go
// internal/container/spec.go
//go:embed ../../config/seccomp/abox-default.json
var seccompProfile []byte

// internal/config/registry.go
//go:embed ../../config/editors.json
var editorsJSON []byte
```

Both the seccomp profile (from `config/seccomp.json` in the dev branch) and the editor
registry are baked into the binary. No runtime file path resolution. No
`_resolve_editors_json()`. No `/etc/abox/seccomp.json` system path required.

The `make install` in the dev branch installs both to system paths
(`/usr/local/share/abx/editors.json`, `/etc/abox/seccomp.json`). The Go binary makes
those paths unnecessary — the files travel with the binary.

### 16.2 GoReleaser

```yaml
# .goreleaser.yaml
version: 2

before:
  hooks:
    - go mod tidy
    - go generate ./...
    - go test ./...                    # unit tests must pass before release

builds:
  - id: abx
    main: ./cmd/abx
    binary: abx
    env:
      - CGO_ENABLED=0                  # fully static; no libc dependency
    ldflags:
      - -s -w
      - -X github.com/r-dson/abox/internal/version.Version={{.Version}}
      - -X github.com/r-dson/abox/internal/version.Commit={{.Commit}}
      - -X github.com/r-dson/abox/internal/version.Date={{.Date}}
    goos:   [linux, darwin]
    goarch: [amd64, arm64]

archives:
  - format: tar.gz
    name_template: "abx_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "abx_{{ .Version }}_checksums.txt"
  algorithm: sha256

signs:
  - artifacts: checksum
    args: ["--batch", "--local-user={{ .Env.GPG_FINGERPRINT }}",
           "--output=${signature}", "--detach-sign", "${artifact}"]

sboms:
  - artifacts: archive

brews:
  - name: abx
    repository:
      owner: r-dson
      name: homebrew-tap
    install: bin.install "abx"
    test: system "#{bin}/abx version"
```

`CGO_ENABLED=0` produces a fully static binary. No shared library dependencies. No glibc
version sensitivity. The same binary runs on Ubuntu 20.04 and Fedora 40 and Alpine without
recompilation.

### 16.3 Installer — simplified

Because `editors.json` is embedded in the binary, the installer no longer needs to download
and place it separately. It downloads one file, verifies one checksum, and installs it.

```bash
#!/bin/sh
set -e

REPO="r-dson/abox"
VERSION="${ABX_VERSION:-latest}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH="amd64";; aarch64|arm64) ARCH="arm64";; *) exit 1;; esac

BASE="https://github.com/${REPO}/releases/${VERSION}/download"
ARCHIVE="abx_${VERSION}_${OS}_${ARCH}.tar.gz"

curl -fsSL "${BASE}/${ARCHIVE}"              -o /tmp/abx.tar.gz
curl -fsSL "${BASE}/abx_${VERSION}_checksums.txt" -o /tmp/abx.sha256

cd /tmp
grep "${ARCHIVE}" abx.sha256 | sha256sum --check --status
echo "Checksum verified."

tar -xzf abx.tar.gz abx
install -m 755 abx "${INSTALL_DIR:-/usr/local/bin}/abx"
rm -f abx.tar.gz abx.sha256 abx
echo "abx installed."
```

One download. One checksum. Zero system directories required.

---

## 17. GitHub Actions Pipeline

### 17.1 CI — every push and PR

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod, cache: true }

      - name: Lint
        uses: golangci/golangci-lint-action@v6
        with: { version: latest }

      - name: Unit tests
        run: go test -race -coverprofile=coverage.out ./...

      - name: Coverage gate
        run: go run github.com/vladopajic/go-test-coverage/v2@latest --config=.coverage.yaml

  integration:
    runs-on: ubuntu-latest
    needs: test
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test -tags=integration -timeout=10m ./...
```

### 17.2 Release — on semver tag

```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write
      id-token: write

    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: crazy-max/ghaction-import-gpg@v6
        with:
          gpg_private_key: ${{ secrets.GPG_PRIVATE_KEY }}
          passphrase: ${{ secrets.GPG_PASSPHRASE }}
      - uses: goreleaser/goreleaser-action@v6
        with: { version: latest, args: release --clean }
        env:
          GITHUB_TOKEN:              ${{ secrets.GITHUB_TOKEN }}
          GPG_FINGERPRINT:           ${{ steps.gpg.outputs.fingerprint }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

### 17.3 Docker image pipeline — unchanged from dev branch

`publish.yml` (Trivy, SBOM, Cosign, multi-arch, selective rebuild) and
`sync-editors.yml` (daily cron, selective rebuild) carry forward exactly as implemented
in the `dev` branch. The Go binary release and the Docker image pipeline are independent:
editor version bumps trigger only image rebuilds. A new Go binary release is cut when
the CLI or core logic changes, versioned with semver tags.

---

## 18. Linting

```yaml
# .golangci.yml
linters:
  enable:
    - errcheck        # no silently ignored errors
    - gosimple
    - govet
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - revive          # replaces golint
    - exhaustive      # all switch cases handled
    - wrapcheck       # errors wrapped with context at package boundary
    - contextcheck    # context.Context threaded correctly
    - noctx           # no HTTP requests without context

linters-settings:
  wrapcheck:
    ignorePackageGlobs:
      - "github.com/r-dson/abox/*"
```

---

## 19. Migration Path

The user-facing surface is 100% backward-compatible. The binary is a drop-in replacement.

| Bash artifact | Go replacement | Notes |
|---|---|---|
| `bin/abx` (bundled script) | `abx` static binary | Same flags, same subcommands, same env vars |
| `~/.config/abx/config.json` | Read natively by Viper | No change to file format |
| `~/.config/abx.conf` | `abx config migrate` one-shot | Reads old format, writes new JSON, removes old file |
| `config/editors.json` | Embedded at compile time | Maintained identically; sync automation unchanged |
| `ABOX_EDITOR`, `ABOX_RUNTIME`, `ABOX_MEMORY`, `ABOX_CPUS` | Viper `AutomaticEnv()` with `ABX_` prefix | Same names with `ABX_` prefix; legacy bare names via explicit binding |
| `.abxignore` patterns | `doublestar.Match()` | Identical pattern syntax; correctness improved |
| `.abxenv` workspace env file | `loadDotEnv()` | Identical format and semantics |
| `docker/Dockerfile` + `Dockerfile.sync` | Unchanged | Out of scope |
| `docker/entrypoint.sh` | Unchanged | Out of scope |
| `config/sync_versions.py` | Unchanged | Runs in CI only |
| `config/seccomp.json` | Embedded via `//go:embed` | No longer needs `/etc/abox/` system install |
| `cursor` editor | Absent | Removed in dev branch; Go rewrite does not add it back |

---

## 20. Implementation Timeline

| Phase | Scope | Weeks |
|---|---|---|
| 1 | Module setup, Go 1.26 workspace, CLI skeleton (Cobra), config system (Viper + embedded registry) | 1 |
| 2 | Runtime interface, Docker/Podman implementations, Detect() | 1 |
| 3 | Container spec builder, volume management, session lifecycle, bootstrap | 1–2 |
| 4 | Sync system: streaming tar, transactional staging, SyncIn/SyncOut, conflict detection | 2 |
| 5 | Exclusion engine (doublestar), symlink resolution, remote pattern fetch | 1 |
| 6 | Strict network (internal Docker network), seccomp embed, SSH agent | 0.5 |
| 7 | Audit subcommand, `.abxenv` parsing, `abx config migrate` | 0.5 |
| 8 | Test coverage to 85% threshold; port 66 registry tests + 16 sync tests + fuzz cases | 1–2 |
| 9 | GoReleaser, CI pipelines, Homebrew formula, installer script, GPG signing | 1 |
| **Total** | | **9–12 weeks** |

**Phase 4 (sync) remains highest risk.** The streaming tar + transactional staging logic
must be tested against real Docker volumes, not only mocks. Allocate integration test
cycles against a real daemon before declaring Phase 4 complete.

**Phase 8 is non-negotiable.** The dev branch reached 295 assertions only after dedicated
remediation effort. The Go coverage gate enforces 85% total — do not let Phase 8 slip to
the end.
