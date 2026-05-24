# ABox Go Rewrite — Implementation Plan

> **Design reference:** `abox-go-design-document.md` (Version 2.0, dev branch baseline)
> **Branch:** `dev/go-rewrite` from `dev`

**Goal:** Replace the Bash implementation with a static Go binary that eliminates global mutable state, shell-based Docker orchestration, and runtime file resolution, while preserving 100% CLI backward compatibility.
**Architecture:** Module-per-concern under `internal/`, Docker SDK for all container operations, `go:embed` for config files, Cobra/Viper for CLI/config, doublestar for exclusion patterns. Single static binary (`CGO_ENABLED=0`). Go project at repo root alongside existing Bash code.
**Tech Stack:** Go 1.26+, Docker Moby SDK v28+, Cobra v2, Viper v2, doublestar v4, errgroup, slog, GoReleaser, mockgen

---

## File Map

### Create (new Go project alongside existing Bash codebase)

```
cmd/abx/main.go                              # Entry point (≤20 lines)
internal/cli/root.go                          # Cobra root + flag wiring
internal/cli/run.go                           # `abx [dir]` command + runSession()
internal/cli/audit.go                         # `abx audit [dir]`
internal/cli/config.go                        # `abx config set-editor|set-exclude-url|migrate|list-editors`
internal/cli/version.go                       # `abx version`
internal/cli/completion.go                    # `abx completion bash|zsh|fish`
internal/config/config.go                     # Config struct + Viper Load()
internal/config/registry.go                   # EditorRegistry — embedded editors.json
internal/config/registry_test.go              # Port of 66 editor-registry-test.sh assertions
internal/runtime/runtime.go                   # ContainerRuntime interface
internal/runtime/docker.go                    # Docker implementation (Moby SDK)
internal/runtime/podman.go                    # Podman (same client, different socket)
internal/runtime/detect.go                    # Auto-detect Docker → Podman
internal/runtime/mock_runtime_test.go         # Generated mock (mockgen)
internal/container/manager.go                 # Session orchestration
internal/container/volumes.go                 # Volume lifecycle, parallel creation
internal/container/volumes_test.go            # Volume creation + bootstrap tests
internal/container/network.go                 # Strict network (internal Docker network)
internal/container/spec.go                    # Typed container spec builder + seccomp embed
internal/container/session.go                 # Session struct + Cleanup()
internal/sync/syncer.go                       # Syncer interface + constructor
internal/sync/transfer.go                     # Streaming tar in/out via Docker API
internal/sync/transfer_test.go                # Port of 16 sync-unit-test.sh assertions
internal/sync/atomic.go                       # Transactional write: stage → atomic rename
internal/sync/conflicts.go                    # mtime snapshot + conflict detection
internal/sync/conflicts_test.go               # Snapshot/detect tests
internal/exclusion/matcher.go                 # Pattern loading + BuildMatcher()
internal/exclusion/matcher_test.go            # Table-driven tests + fuzz cases
internal/exclusion/walk.go                    # fs.WalkDir with symlink resolution
internal/exclusion/hardcoded.go               # Always-excluded security patterns
internal/audit/audit.go                       # Pre-run workspace security checks
internal/logging/logging.go                   # slog setup (text/JSON/file)
internal/version/version.go                   # Version, Commit, Date (set via ldflags)
config/seccomp/abox-default.json              # go:embed target (copy from config/seccomp.json)
go.mod                                        # Module definition
go.sum                                        # Dependency checksums
.goreleaser.yaml                              # Build + release config
.golangci.yml                                 # Linter config
.coverage.yaml                                # Coverage threshold config
```

### Modify (existing files)

- `config/editors.json` — No change; Go embeds it as-is
- `config/seccomp.json` — Copy to `config/seccomp/abox-default.json` for go:embed path
- `Makefile` — Add Go build targets alongside existing Bash targets
- `.github/workflows/ci.yml` — New: Go CI pipeline
- `.github/workflows/release.yml` — New: GoReleaser on semver tag
- `install` — Replace with simplified single-binary installer
- `README.md` — Update to reflect Go binary

### Unchanged (carry forward from dev branch)

- `docker/Dockerfile` — Editor images
- `docker/Dockerfile.sync` — Alpine sync image
- `docker/entrypoint.sh` — Container entrypoint
- `.github/workflows/publish.yml` — Trivy, SBOM, Cosign, multi-arch
- `.github/workflows/sync-editors.yml` — Daily cron, selective rebuild
- `config/sync_versions.py` — CI-only version sync

---

## Phase 1: Module Setup + CLI Skeleton (Week 1)

### Task 1.1: Initialize Go module and directory structure

- [ ] Create `go.mod` with `module github.com/r-dson/abox`, Go 1.26
- [ ] Create directory tree: `cmd/abx/`, `internal/{cli,config,runtime,container,sync,exclusion,audit,logging}/`
- [ ] Create placeholder `config/seccomp/abox-default.json` (copy from `config/seccomp.json`)
- [ ] Run `go mod tidy` — verify empty build succeeds
- [ ] Commit: `build(go): initialize module and directory structure`

### Task 1.2: Minimal main.go + Cobra root command

- [ ] **RED:** Write `cmd/abx/main_test.go` that calls `cli.NewRootCmd("test")` and verifies `Use == "abx"` and `SilenceUsage == true`
- [ ] Run: `go test ./cmd/abx/...` — expect compilation error (package doesn't exist)
- [ ] **GREEN:** Create `cmd/abx/main.go` — wire `cli.NewRootCmd()` and call `Execute()`
- [ ] Create `internal/cli/root.go` — `NewRootCmd(version string) *cobra.Command` with `Use: "abx"`, `SilenceUsage: true`, `SilenceErrors: true`, persistent `--verbose` and `--json-logs` flags, `PersistentPreRunE` calling `logging.Setup()`
- [ ] Run: `go test ./cmd/abx/...` — pass
- [ ] Verify: `go build -o /tmp/abx ./cmd/abx && /tmp/abx --help` prints usage
- [ ] Commit: `feat(cli): add Cobra root command with --verbose and --json-logs flags`

### Task 1.3: Editor registry — embedded editors.json

- [ ] **RED:** Create `internal/config/registry_test.go` with table-driven tests verifying all 8 editors (names, image tags, cmd names, config paths, env vars, legacy paths) and cursor absence
- [ ] Run: `go test ./internal/config/...` — expect compilation error
- [ ] **GREEN:** Create `internal/config/registry.go`:
  - `//go:embed ../../config/editors.json` → `var editorsJSON []byte`
  - `EditorProfile` struct with JSON tags matching current schema
  - `CachePath(home)`, `StatePath(home)`, `SharePath(home)` derived path methods
  - `EditorRegistry` struct with `LoadEditorRegistry() (*EditorRegistry, error)` and `Get(name string) (EditorProfile, error)` (falls back to opencode)
  - `Names() []string` sorted
- [ ] Run: `go test ./internal/config/...` — all 66+ assertions pass
- [ ] Commit: `feat(config): embed editors.json and implement EditorRegistry`

### Task 1.4: Config system — Viper + JSON config

- [ ] **RED:** Write `internal/config/config_test.go` — test `Load()` reads a temp JSON config file, merges env var `ABX_EDITOR` over config, applies defaults (`editor: opencode`, `pull_policy: missing`, `memory_limit: 4g`, `cpu_limit: 2.0`)
- [ ] Run: `go test ./internal/config/...` — expect compilation error
- [ ] **GREEN:** Create `internal/config/config.go`:
  - `Config` struct with mapstructure tags
  - `Load(v *viper.Viper) (*Config, error)` — reads `~/.config/abx/config.json`, sets `ABX_` env prefix, defaults
- [ ] Run: `go test ./internal/config/...` — pass
- [ ] Commit: `feat(config): implement Viper-based config loading with env merge`

### Task 1.5: Version command + version package

- [ ] **RED:** Write test verifying `abx version` outputs version string
- [ ] **GREEN:** Create `internal/version/version.go` with `var Version, Commit, Date string` (set via ldflags)
- [ ] Create `internal/cli/version.go` — `newVersionCmd()` that prints version info
- [ ] Register in root command
- [ ] Run: `go test ./internal/cli/...` — pass
- [ ] Commit: `feat(cli): add version command`

### Task 1.6: Logging module

- [ ] **RED:** Write `internal/logging/logging_test.go` — test that `Setup(true, false)` creates log file, `Setup(false, true)` sets JSON handler, `Setup(false, false)` sets text handler
- [ ] **GREEN:** Create `internal/logging/logging.go` — `Setup(verbose, jsonOutput bool)`, file handler at `~/.local/state/abx/abx.log` when verbose, `multiHandler` for fan-out, XDG path resolution
- [ ] Run: `go test ./internal/logging/...` — pass
- [ ] Commit: `feat(logging): implement slog setup with file and JSON handlers`

---

## Phase 2: Runtime Abstraction (Week 2)

### Task 2.1: ContainerRuntime interface

- [ ] **RED:** Write `internal/runtime/runtime_test.go` — verify the interface type assertion `var _ ContainerRuntime = (*dockerRuntime)(nil)` compiles
- [ ] **GREEN:** Create `internal/runtime/runtime.go`:
  - `ContainerSpec` struct (Container, Host, Name fields)
  - `ContainerRuntime` interface with: `VolumeCreate`, `VolumeRemove`, `NetworkCreate`, `NetworkRemove`, `ContainerCreate`, `ContainerStart`, `ContainerWait`, `ContainerRemove`, `ContainerAttach`, `ContainerExec`, `CopyToContainer`, `CopyFromContainer`, `ImagePull`, `ImageExists`, `Ping`
- [ ] Run: `go test ./internal/runtime/...` — pass
- [ ] Commit: `feat(runtime): define ContainerRuntime interface and ContainerSpec type`

### Task 2.2: Docker implementation

- [ ] **RED:** Write `internal/runtime/docker_test.go` — test `NewDocker()` with a mock HTTP server (or skip if no Docker daemon, guarded by `//go:build integration`)
- [ ] **GREEN:** Create `internal/runtime/docker.go`:
  - `dockerRuntime` struct wrapping `*dockerclient.Client`
  - `NewDocker(ctx) (*dockerRuntime, error)` — `NewClientWithOpts(FromEnv, WithAPIVersionNegotiation)`, ping check
  - Implement all `ContainerRuntime` methods using Moby SDK
  - `NetworkCreate` with `Internal: true` for strict mode
  - `CopyToContainer`/`CopyFromContainer` wrapping SDK methods directly
- [ ] Run: `go test ./internal/runtime/...` — pass (unit tests only)
- [ ] Commit: `feat(runtime): implement Docker runtime via Moby SDK`

### Task 2.3: Podman implementation

- [ ] Create `internal/runtime/podman.go` — identical to Docker except socket resolution: `podmanSocket()` checks `$DOCKER_HOST`, then `/run/user/{uid}/podman/podman.sock`
- [ ] Run: `go test ./internal/runtime/...` — pass
- [ ] Commit: `feat(runtime): implement Podman runtime via Docker-compatible socket`

### Task 2.4: Runtime detection

- [ ] **RED:** Write `internal/runtime/detect_test.go` — test `Detect()` respects `ABOX_RUNTIME` env var, falls back Docker → Podman
- [ ] **GREEN:** Create `internal/runtime/detect.go` — `Detect(ctx) (ContainerRuntime, error)`, checks `ABOX_RUNTIME` env, tries Docker, tries Podman, returns descriptive error
- [ ] Run: `go test ./internal/runtime/...` — pass
- [ ] Commit: `feat(runtime): implement auto-detect with ABOX_RUNTIME override`

### Task 2.5: Generate mock for ContainerRuntime

- [ ] Add `go.uber.org/mock` to go.mod
- [ ] Add `//go:generate go run go.uber.org/mock/mockgen -destination=mock_runtime_test.go -package=runtime_test github.com/r-dson/abox/internal/runtime ContainerRuntime` to `runtime.go`
- [ ] Run `go generate ./internal/runtime/`
- [ ] Verify generated mock compiles
- [ ] Commit: `build(runtime): generate ContainerRuntime mock via mockgen`

---

## Phase 3: Container Management (Weeks 2–3)

### Task 3.1: Session struct + Cleanup

- [ ] **RED:** Write `internal/container/session_test.go` — test `Cleanup()` calls `VolumeRemove` for all volumes and `NetworkRemove` for network, verify no-op for empty fields
- [ ] **GREEN:** Create `internal/container/session.go`:
  - `Session` struct: `ID, ConfigVol, CacheVol, StateVol, ShareVol, WorkspaceVol, NetworkID string; runtime ContainerRuntime`
  - `const SyncImage = "ghcr.io/r-dson/abox:sync"`
  - `Cleanup(ctx)` method — removes network if set, then all non-empty volumes
  - `volumeNames() []string` helper
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement Session struct with volume and network cleanup`

### Task 3.2: Volume creation — parallel with errgroup

- [ ] **RED:** Write `internal/container/volumes_test.go` — mock expects 4 `VolumeCreate` calls + bootstrap sequence, verify all volumes named correctly
- [ ] **GREEN:** Create `internal/container/volumes.go`:
  - `Manager` struct wrapping `ContainerRuntime`
  - `NewSession(ctx, profile, cfg, matcher) (*Session, error)` — creates timestamp ID, creates volumes in parallel via `errgroup.WithContext`, calls `bootstrapOwnership`, creates strict network if configured, returns session with cleanup on error
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement parallel volume creation with errgroup`

### Task 3.3: Volume ownership bootstrap

- [ ] **RED:** Add to volumes_test — mock expects `ContainerCreate` with `SyncImage` and `user: 0:0`, `ContainerStart`, `ContainerWait` returning 0
- [ ] **GREEN:** Implement `bootstrapOwnership(ctx, session)` in `volumes.go`:
  - Creates ephemeral container with `SyncImage` (not editor image), `user: 0:0`, `CapDrop: ALL`, `CapAdd: CHOWN`
  - Runs `chown -R uid:gid` on all volume mount paths
  - Waits for exit code 0
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement volume ownership bootstrap using sync image`

### Task 3.4: Container spec builder

- [ ] **RED:** Write `internal/container/spec_test.go` — test `BuildSpec()` produces correct `CapDrop: [ALL]`, `CapAdd: [CHOWN, SETUID, SETGID]`, no `DAC_OVERRIDE`, seccomp flag, workspace bind, env vars from profile
- [ ] **GREEN:** Create `internal/container/spec.go`:
  - `//go:embed ../../config/seccomp/abox-default.json` + `sync.OnceValue` temp file
  - `BuildSpec(profile, session, workdir, cfg, opts) ContainerSpec`
  - `buildEnv()` — HOST_UID, HOST_GID, profile env vars, `--env` keys, SSH_AUTH_SOCK
  - `buildBinds()` — volume mounts, workspace, gitconfig, SSH socket preferred over .ssh dir
  - `buildCmd()` — editor command with optional shell/args
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement typed container spec builder with seccomp embed`

### Task 3.5: Network module — strict mode

- [ ] **RED:** Write `internal/container/network_test.go` — mock expects `NetworkCreate` with `Internal: true`, verify `resolveNetworkMode` returns correct mode for none/strict/default
- [ ] **GREEN:** Create `internal/container/network.go`:
  - `setupStrictNetwork(ctx, session)` — creates internal network, stores ID
  - `resolveNetworkMode(session, cfg)` — returns "none", network ID, or "" (default bridge)
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement strict network mode via internal Docker network`

### Task 3.6: Manager — container lifecycle

- [ ] **RED:** Write `internal/container/manager_test.go` — mock expects Create → Start → Attach (TTY) → Wait → exit code propagation
- [ ] **GREEN:** Create `internal/container/manager.go`:
  - `NewManager(rt) *Manager`
  - `Run(ctx, session, profile, workdir, cfg, opts) (int, error)` — builds spec, creates container, starts, attaches stdin/stdout for TTY, waits for exit code
  - Proper signal handling: context cancellation triggers container stop
- [ ] Run: `go test ./internal/container/...` — pass
- [ ] Commit: `feat(container): implement container lifecycle with TTY attach and signal handling`

---

## Phase 4: Sync System (Weeks 3–5) — Highest Risk

### Task 4.1: Syncer constructor + SyncIn orchestration

- [ ] **RED:** Write `internal/sync/transfer_test.go` — mock expects parallel `CopyToContainer` for config/cache/state/share, then workspace
- [ ] **GREEN:** Create `internal/sync/syncer.go`:
  - `Syncer` struct wrapping `ContainerRuntime`
  - `New(rt) *Syncer`
- [ ] Create `internal/sync/transfer.go`:
  - `SyncIn(ctx, session, profile, workdir, matcher) error` — defines transfers list, fans out via errgroup
  - `mountVolumeContainer(ctx, volumeName) (containerID string, cleanup func(), error)` — helper that creates ephemary sync container with volume mounted
- [ ] Run: `go test ./internal/sync/...` — pass
- [ ] Commit: `feat(sync): implement SyncIn orchestration with parallel transfers`

### Task 4.2: Streaming tar transfer — host to volume

- [ ] **RED:** Write test verifying `transferDirToVolume` calls `CopyToContainer` with a staging path (`.abx-tmp`), then `ContainerExec` with `mv -T`
- [ ] **GREEN:** Implement `transferDirToVolume(ctx, srcDir, volumeName, dstPath) error`:
  - Creates tar stream via `io.Pipe()` — goroutine writes `archive/tar`, main goroutine passes reader to `CopyToContainer`
  - Writes to staging path `dstPath + ".abx-tmp"`
  - Atomic rename via `ContainerExec(ctx, id, ["mv", "-T", stagingPath, dstPath])`
  - Skips if source dir doesn't exist (first run)
- [ ] Run: `go test ./internal/sync/...` — pass
- [ ] Commit: `feat(sync): implement streaming tar transfer with transactional staging`

### Task 4.3: Workspace sync with exclusion filtering

- [ ] **RED:** Write test: workspace with `main.go` and `.env`, `.abxignore` excludes `.env` — verify tar contains `main.go` but not `.env`
- [ ] **GREEN:** Implement `transferWorkspaceToVolume(ctx, workdir, volumeName, matcher) error`:
  - `tarFiltered(workdir, io.Writer, matcher) error` — `fs.WalkDir`, resolves symlinks via `filepath.EvalSymlinks`, skips excluded paths, writes non-excluded files to tar
  - Streams to volume container via `CopyToContainer`
- [ ] Run: `go test ./internal/sync/...` — pass
- [ ] Commit: `feat(sync): implement workspace sync with exclusion-filtered streaming`

### Task 4.4: SyncOut — volume back to host

- [ ] **RED:** Write test: mock `CopyFromContainer` returns tar with a file, verify file extracted to host path with correct content and ownership check
- [ ] **GREEN:** Implement `SyncOut(ctx, session, profile, workdir, matcher) error`:
  - For each volume: `CopyFromContainer` → extract tar to staging dir → atomic `os.Rename`
  - For workspace: `CopyFromContainer` → extract with exclusion filtering → atomic rename
  - Ownership verification: refuse to overwrite if uid/gid changed
- [ ] Run: `go test ./internal/sync/...` — pass
- [ ] Commit: `feat(sync): implement SyncOut with atomic host writes`

### Task 4.5: Mtime conflict detection

- [ ] **RED:** Write `internal/sync/conflicts_test.go` — snapshot files, modify one, verify `DetectConflicts()` returns changed path; snapshot unchanged files, verify empty result
- [ ] **GREEN:** Create `internal/sync/conflicts.go`:
  - `MtimeSnapshot` struct with `entries map[string]time.Time`
  - `SnapshotMtimes(profile, workdir) (*MtimeSnapshot, error)` — walks config/cache/state/share dirs
  - `DetectConflicts() []string` — compares current mtimes, returns sorted changed paths
- [ ] Run: `go test ./internal/sync/...` — pass
- [ ] Commit: `feat(sync): implement mtime-based conflict detection`

### Task 4.6: Integration test — full sync round-trip

- [ ] Create `internal/sync/integration_test.go` (build tag: `//go:build integration`)
- [ ] Test full SyncIn → container run → SyncOut cycle with real Docker daemon against temp workspace with secrets
- [ ] Verify secrets excluded, code preserved, no intermediate temp files in `/tmp`
- [ ] Commit: `test(sync): add integration test for full sync round-trip`

---

## Phase 5: Exclusion Engine (Week 5)

### Task 5.1: Hardcoded security patterns

- [ ] **RED:** Write `internal/exclusion/matcher_test.go` — verify `.ssh`, `.aws`, `.env`, `**/*.pem`, `**/*key` match; verify `main.go` does not match
- [ ] **GREEN:** Create `internal/exclusion/hardcoded.go` — `Patterns() []string` returns the `ABX_SECURITY_EXCLUSIONS` equivalent list
- [ ] Commit: `feat(exclusion): define hardcoded security exclusion patterns`

### Task 5.2: Matcher — pattern loading and matching

- [ ] **RED:** Write table-driven tests for `Match()`: exact, glob star, doublestar, brace expand, question mark, no match
- [ ] **GREEN:** Create `internal/exclusion/matcher.go`:
  - `Matcher` struct with `patterns []string`
  - `BuildMatcher(ctx, workdir, remoteURL) (*Matcher, error)` — merges hardcoded + local `.abxignore` + remote URL patterns
  - `Match(path) bool` — iterates patterns, calls `doublestar.Match()`
  - `HasPatterns() bool`
  - `loadLocalIgnore(workdir) ([]string, error)` — reads `.abxignore`
  - `fetchRemotePatterns(ctx, url) ([]string, error)` — HTTP GET, split lines, warn on failure
  - `mergePatterns(base, additional) []string` — deduplicates
- [ ] Run: `go test ./internal/exclusion/...` — pass
- [ ] Commit: `feat(exclusion): implement pattern loading and doublestar matching`

### Task 5.3: Walk — filtered directory traversal

- [ ] **RED:** Write `internal/exclusion/walk_test.go` — temp dir with nested files, `.abxignore` excluding `**/secret*`, verify Walk returns only non-excluded paths
- [ ] **GREEN:** Create `internal/exclusion/walk.go`:
  - `Walk(root string) ([]string, error)` — `fs.WalkDir`, resolves symlinks via `filepath.EvalSymlinks`, matches both original and resolved paths, `SkipDir` on excluded directories
  - `EmptyMatcher()` returns a matcher with only hardcoded patterns
- [ ] Run: `go test ./internal/exclusion/...` — pass
- [ ] Commit: `feat(exclusion): implement filtered directory walk with symlink resolution`

---

## Phase 6: Network + Security (Week 6, half)

### Task 6.1: Seccomp embed and application

- [ ] Already handled in Task 3.4 (embedded in `spec.go`). Verify with test that `seccompTempPath()` returns a valid JSON file containing `"defaultAction": "SCMP_ACT_ERRNO"`
- [ ] Commit: included in Task 3.4

### Task 6.2: SSH agent socket forwarding

- [ ] Already handled in Task 3.4 (`buildEnv` + `buildBinds`). Verify with test that when `SSH_AUTH_SOCK` points to a real socket, binds include socket mount and env includes `SSH_AUTH_SOCK=/tmp/ssh-agent.sock`
- [ ] Commit: included in Task 3.4

---

## Phase 7: CLI Subcommands (Week 6, half)

### Task 7.1: Run command — wire everything together

- [ ] **RED:** Write `internal/cli/run_test.go` — mock runtime, verify `runSession()` calls SyncIn → Run → SyncOut in order, exits with container exit code
- [ ] **GREEN:** Create `internal/cli/run.go`:
  - `RunOptions` struct
  - `newRunCmd() *cobra.Command` — flags: `--editor`, `--shell`, `--force-it`, `--offline`, `--strict-network`, `--no-internet`, `--force-sync`, `--exclude-url`, `--env` (repeatable)
  - `runSession(ctx, workdir, opts)` — full orchestration: config → registry → runtime → exclusion matcher → session → snapshot → SyncIn → Run → conflict check → SyncOut → exit code
  - `loadDotEnv(workdir) []string` — parses `.abxenv`
  - `applyFlags(cfg, opts)` — CLI flags override config
  - `validateWorkdir(workdir)` — blocks `$HOME`, `/`, non-existent dirs
- [ ] Run: `go test ./internal/cli/...` — pass
- [ ] Commit: `feat(cli): implement run command with full session orchestration`

### Task 7.2: Audit command

- [ ] **RED:** Write `internal/audit/audit_test.go` — verify `Run()` returns typed `Result` with checks for sensitive files, runtime, workdir safety
- [ ] **GREEN:** Create `internal/audit/audit.go`:
  - `Result`, `Check`, `Status` types
  - `Run(ctx, workdir, rt) (*Result, error)` — runs checks, returns typed result
  - Individual check functions: `checkSensitiveFiles`, `checkAbxIgnore`, `checkRuntime`, `checkWorkdirSafety`, `checkSeccompProfile`
- [ ] Create `internal/cli/audit.go` — `newAuditCmd()` renders `Result` to stdout
- [ ] Run: `go test ./internal/audit/... ./internal/cli/...` — pass
- [ ] Commit: `feat(cli): implement audit subcommand`

### Task 7.3: Config subcommand

- [ ] **RED:** Write test for `set-editor` writing JSON config
- [ ] **GREEN:** Create `internal/cli/config.go`:
  - `newConfigCmd()` with subcommands:
    - `set-editor <name>` — writes `editor` field to JSON config
    - `set-exclude-url <url>` — writes `exclude_url` field
    - `migrate` — reads legacy `~/.config/abx.conf`, writes JSON, removes old file
    - `list-editors` — prints all editors from embedded registry
- [ ] Run: `go test ./internal/cli/...` — pass
- [ ] Commit: `feat(cli): implement config set-editor, set-exclude-url, migrate, list-editors`

### Task 7.4: Completion command

- [ ] Create `internal/cli/completion.go` — standard Cobra completion generation for bash/zsh/fish/powershell
- [ ] Commit: `feat(cli): add shell completion command`

---

## Phase 8: Test Coverage (Weeks 7–8) — Non-Negotiable

### Task 8.1: Port editor-registry tests (66 assertions)

- [ ] Expand `internal/config/registry_test.go` to cover all 8 editors with all fields
- [ ] Verify unknown editor falls back to opencode
- [ ] Verify cursor is absent (falls back to opencode, not error)
- [ ] Run: `go test -v ./internal/config/...` — all pass
- [ ] Commit: `test(config): port 66 editor-registry assertions from Bash`

### Task 8.2: Port sync tests (16 assertions)

- [ ] Expand `internal/sync/transfer_test.go`:
  - No intermediate tar files in `/tmp`
  - Transactional write uses staging path
  - `SyncImage` constant used, not editor image
  - Streaming (no `os/exec` calls)
  - Atomic rename via `ContainerExec`
- [ ] Run: `go test -v ./internal/sync/...` — all pass
- [ ] Commit: `test(sync): port 16 sync-unit assertions from Bash`

### Task 8.3: Port exclusion fuzz cases

- [ ] Expand `internal/exclusion/matcher_test.go` with fuzz-derived cases (brace expand, question mark, doublestar edge cases)
- [ ] Add Go fuzz test: `func FuzzMatch(f *testing.F)` with corpus from Bash fuzz findings
- [ ] Run: `go test -fuzz=FuzzMatch -fuzztime=30s ./internal/exclusion/`
- [ ] Commit: `test(exclusion): port fuzz test cases from Bash`

### Task 8.4: Coverage gate

- [ ] Create `.coverage.yaml` with 85% total threshold, 80% package, 70% file
- [ ] Add coverage step to CI: `go test -race -coverprofile=coverage.out ./... && go run github.com/vladopajic/go-test-coverage/v2@latest --config=.coverage.yaml`
- [ ] Add missing tests to reach threshold
- [ ] Commit: `test: add coverage gate with 85% total threshold`

---

## Phase 9: Build + Distribution (Week 8–9)

### Task 9.1: GoReleaser config

- [ ] Create `.goreleaser.yaml`:
  - Build: `CGO_ENABLED=0`, linux/darwin, amd64/arm64
  - ldflags for version/commit/date
  - Archives, checksums (SHA-256), GPG signs, SBOMs
  - Homebrew tap formula
- [ ] Run: `goreleaser release --snapshot --clean` — verify build succeeds
- [ ] Commit: `build: add GoReleaser configuration`

### Task 9.2: Linter config

- [ ] Create `.golangci.yml` with: errcheck, gosimple, govet, staticcheck, unused, gofmt, goimports, misspell, revive, exhaustive, wrapcheck, contextcheck, noctx
- [ ] Run: `golangci-lint run ./...` — fix any findings
- [ ] Commit: `build: add golangci-lint configuration`

### Task 9.3: CI pipeline

- [ ] Create `.github/workflows/ci.yml`:
  - On push/PR: lint + unit tests + coverage gate
  - On push to main: integration tests with Docker
- [ ] Create `.github/workflows/release.yml`:
  - On semver tag: GoReleaser release + GPG sign + Homebrew
- [ ] Commit: `ci: add Go CI and release pipelines`

### Task 9.4: Updated installer

- [ ] Rewrite `install` script — single binary download, SHA-256 verification, no editors.json download needed
- [ ] Commit: `build: simplify installer for single Go binary`

### Task 9.5: Update README

- [ ] Update `README.md` to reflect Go binary, updated install instructions, updated flags
- [ ] Note: existing Bash codebase remains in repo for reference; Docker files unchanged
- [ ] Commit: `docs: update README for Go rewrite`

### Task 9.6: Makefile Go targets

- [ ] Add to `Makefile`: `go-build`, `go-test`, `go-lint`, `go-install` targets alongside existing Bash targets
- [ ] Commit: `build: add Go targets to Makefile`

---

## Execution Notes

**Risk: Phase 4 (sync)** remains the highest-risk phase. Streaming tar + transactional staging must be tested against a real Docker daemon. Allocate integration test time before declaring Phase 4 complete.

**Non-negotiable: Phase 8 (test coverage)** enforces the 85% total threshold. Do not let this slip to the end.

**Ordering constraint:** Phases 1–3 must be sequential. Phases 4 and 5 can proceed in parallel after Phase 3. Phase 7 requires Phases 4–6. Phase 8 requires all implementation phases. Phase 9 is independent of implementation phases (can start after Phase 1).

**Each task includes:** failing test → verify red → minimal implementation → verify green → commit. No exceptions.

**Repository layout:** Go project lives at repo root (`go.mod` alongside `Makefile`). Existing Bash `src/`, `tests/`, `bin/` coexist during rewrite. When Go is production-ready, Bash files are removed in a single cleanup commit.
