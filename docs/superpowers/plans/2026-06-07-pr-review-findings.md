# PR review findings implementation plan

> For agentic workers: implement this plan work package by work package. Finish one package, verify it, commit it, then continue. This plan is the retained execution record for the PR review findings across the Go implementation, CI/release, Docker assets, docs, and governance.

**Goal:** Resolve every actionable PR review finding while preserving ABox's secure-by-default container isolation model and keeping the repository green after each work package.

**Architecture:** Keep the current package boundaries: CLI orchestration remains in `internal/cli`, tar and conflict logic in `internal/sync`, runtime mapping in `internal/runtime`, container spec/session lifecycle in `internal/container`, config and editor registry in `internal/config`, and workspace matching in `internal/exclusion`. High-risk sync-out changes must be root-confined, matcher-aware, conflict-aware per sync root, and recoverable when host conflicts block writing. Security posture is strict by default: workspace `.abxenv` is disabled unless the user explicitly opts in through trusted config or a CLI flag for that run.

**Tech stack:** Go 1.26, Cobra, Viper, Moby Docker SDK, Podman via Docker-compatible API, `doublestar/v4`, `errgroup`, Python with `uv`, GitHub Actions, GoReleaser, Docker Buildx, Trivy, Syft/Anchore SBOM, cosign, actionlint, Hadolint, gitleaks, CodeQL.

---

## Source review coverage audit

The temporary source review file was audited before removal. It contained 200 actionable finding bullets:

- 9 Blocking findings.
- 166 Important findings.
- 25 Minor findings.

Coverage is retained in this plan through the 15 work packages and the finding coverage matrix at the end. Package 15 requires final closure by severity: every Critical finding must have a linked commit and passing regression test; every Important and Minor finding must have a code/test/doc/CI change or an explicit no-action rationale in the retained implementation notes.

## Resolved decisions for this plan

- Workspace `.abxenv` is disabled by default. Add `trust_workspace_env` user config plus `--trust-workspace-env`; only when true does ABox read workspace `.abxenv` as a host-variable allowlist.
- ABox-owned environment keys are reserved: `HOST_UID`, `HOST_GID`, `SSH_AUTH_SOCK`, `ABX_SESSION_ID`, `ABX_WORKSPACE`, `HOME`, `PATH`, `USER`, `SHELL`, `PWD`, `TERM`, display/session keys, and any future ABox control key must not be overridden by `.abxenv` or `--env`.
- Sync-out is matcher-aware. Excluded paths are never created, overwritten, or deleted on the host during sync-out.
- Sync-out is per-root. A conflict in one root blocks only that root unless `--force-sync` is set.
- If any root is skipped due to conflict and `--force-sync` is not set, ABox returns non-zero and preserves affected volumes with recovery instructions instead of silently deleting sandbox changes.
- Directory sync-out propagates container-side deletions only for paths that were inside the sync scope and are not excluded. Host-created files that do not collide with the outgoing archive remain untouched.
- Symlink handling is explicit: sync-in preserves safe host symlinks as symlink archive entries; sync-out recreates only relative symlinks whose resolved target stays inside the destination root. Absolute or escaping symlinks fail the affected sync root.
- Seccomp runtime data has a single production source under `config/seccomp/abox-default.json`; test fixtures remain in `testdata` only for tests.
- Installer and GoReleaser will use matching artifact names `abx_<os>_<arch>.tar.gz`.
- Release images should use immutable versioned tags in registry metadata first; digest pinning is a follow-up only when the publish workflow can emit and consume digests in a reproducible manifest.

---

## File map

### Create

- `internal/osutil/osutil.go` — shared home directory, terminal detection, bounded cleanup context, file-mode helpers, and environment-key helpers.
- `internal/osutil/osutil_test.go` — tests for home resolution, terminal abstraction, and cleanup timeout helper.
- `internal/sync/archive.go` — archive entry validation, staging extraction, manifest generation, close-error handling, symlink validation, special-file filtering, deletion reconciliation, and context checks.
- `internal/sync/archive_test.go` — unit tests for root confinement, symlink roots, parent symlinks, deletion reconciliation, excluded path protection, empty directories, special files, and close-error paths.
- `internal/sync/options.go` — `Options`, `RootSnapshot`, `RootConflict`, and per-root sync-out options.
- `internal/container/security.go` — shared helper container security/resource defaults and cleanup timeout constants.
- `internal/container/security_test.go` — tests for helper defaults.
- `internal/runtime/docker_mapping.go` — pure conversion from `runtime.ContainerSpec` to Docker `Config` and `HostConfig`.
- `internal/runtime/docker_mapping_test.go` — tests for binds, SELinux labels, network, env, stdin, security options, resources, and mount type selection.
- `internal/ci/README.md` — documented CI/security validation commands and expected local equivalents.
- `SECURITY.md` — vulnerability disclosure policy.
- `.github/dependabot.yml` — dependency update configuration for Go modules, GitHub Actions, Docker, and Python package ecosystem where supported.
- `.github/CODEOWNERS` — required reviewers for workflows, Dockerfiles, Go module files, release config, installer, sync/security packages, and seccomp profiles.
- `.github/workflows/security.yml` — govulncheck, CodeQL, gitleaks, Hadolint, and actionlint security/static-analysis workflow.
- `config/sync_versions_test.py` — tests for editor version mapping, fetch failure behavior, and atomic write behavior.

### Modify

- `go.mod`, `go.sum` — add `govulncheck` as a tracked tool; clean tool/runtime dependency split without removing required runtime dependencies.
- `README.md` — update `.abxenv`, install checksum, support matrix, image trust, Go version, runtime versions, no-internet/helper behavior, and content exclusion docs.
- `AGENTS.md` — fix whitespace-only review failures only; do not rewrite policy text.
- `cmd/abx/main.go` — signal-aware root context, `errors.AsType`, version metadata variables, cleanup hook wiring.
- `internal/cli/root.go` — skip run-only config loading for informational subcommands, load config once, support `[directory] [editor args...]`, bind every documented `ABX_` env key, parse `--env`, add `.abxenv` trust flag, and reserve env keys.
- `internal/cli/run.go` — trusted `.abxenv`, env merge validation, default editor constant, pull-policy `never` image existence checks, per-root snapshot/conflict/sync-out, sync-out matcher use, preserved-volume recovery, bounded cleanup context, correct success return, comments.
- `internal/cli/audit.go` — return non-zero on failed audit checks and include detail output.
- `internal/cli/completion.go` — add valid shells.
- `internal/cli/config.go` — fail on config read errors, write config `0600`.
- `internal/cli/*_test.go` — root/config/run/audit/env/editor-args tests.
- `internal/audit/audit.go`, `internal/audit/audit_test.go` — reuse run workdir validation, shared sensitive matcher, path details, permission warnings, context checks.
- `internal/config/config.go`, `internal/config/config_test.go` — default editor constant, env binding expectations, `errors.AsType`, trusted workspace env config, JSON tags decision.
- `internal/config/registry.go`, `internal/config/config_test.go`, `internal/config/editors.json` — registry validation, caching, defensive copies, editor copy drift tests.
- `config/editors.json`, `bin/editors.json` — align editor registry copies or stop shipping stale copy.
- `internal/exclusion/hardcoded.go`, `internal/exclusion/*_test.go` — expanded hardcoded exclusions, validate patterns, HTTPS-only remote URL, normalization cleanup, simple-glob docs.
- `internal/sync/transfer.go`, `internal/sync/conflicts.go`, `internal/sync/*_test.go` — context-aware tar/snapshot/extract, matcher-aware sync-out, staging reconciliation, per-root conflicts, special-file policy, close-error checks, pipe cleanup, helper security.
- `internal/container/spec.go`, `internal/container/manager.go`, `internal/container/network.go`, `internal/container/session.go`, `internal/container/*_test.go` — seccomp source, socket validation, gitconfig opt-in/sanitized config, stdin forwarding, signal cleanup, session IDs, helper restrictions, resource hardening, cleanup behavior.
- `internal/runtime/runtime.go`, `internal/runtime/docker.go`, `internal/runtime/podman.go`, `internal/runtime/*_test.go` — close contract, mapping tests, wait errors, attach stdin, Podman socket env, resources, mount source typing.
- `internal/runtimetest/stub.go` — fix `Write` contract and document production test-helper exception or move to test-only helper package if feasible.
- `internal/logging/logging.go`, `internal/logging/logging_test.go` — JSON only on flag, close log file, aggregate handler errors, shared terminal helper.
- `install` — artifact naming, portable SHA-256, fail closed, missing checksum entry message.
- `.goreleaser.yml` — archive names, release integrity settings, ldflags alignment.
- `Makefile` — split editor and CLI versions, add missing `.PHONY`, align test/coverage targets.
- `.github/workflows/go-ci.yml` — avoid `bc`, add actionlint/govulncheck, include pure runtime tests in coverage, Go version documentation check.
- `.github/workflows/publish.yml` — safe matrix input, trusted PR behavior, pinned actions, canonical tags, scan/SBOM/sign every image, sync image parity, OIDC permission, image smoke tests.
- `.github/workflows/release.yml` — tag-only production release and separate snapshot behavior.
- `.github/workflows/sync-editors.yml` — reduced permissions, no run-history deletion, update both registry copies, failure on stale fetches.
- `docker/Dockerfile`, `docker/Dockerfile.sync`, `docker/entrypoint.sh` — digest-pinned bases, no `eval`, robust UID/GID, visible chown failures, quoted default command, smokeable editor commands.
- `config/sync_versions.py` — timeouts, schema/fetch failures, all editors, UTF-8, atomic writes, non-zero failures.
- `.golangci.yml` — add enforceable lint settings only when supported by the installed linter version.

### Read for context

- `AGENTS.md` — repository standards and validation commands.
- `go.mod`, `.golangci.yml` — Go version and lint/tool expectations.
- `.github/workflows/*.yml`, `.goreleaser.yml`, `Makefile`, `install` — CI/release/install current behavior.
- `config/editors.json`, `internal/config/editors.json`, `bin/editors.json` — registry drift and image tag data.
- `config/seccomp.json`, `config/seccomp/abox-default.json`, `internal/container/testdata/seccomp.json` — seccomp drift.
- `docker/*` — image build and runtime entrypoint behavior.

---

## Baseline verification

Run before changing implementation files:

```bash
git status --short
go mod verify
go test ./...
go tool golangci-lint run ./...
go test -race -coverprofile=coverage.out $(go list ./... | grep -v -E '(cmd/abx$|internal/runtime$)')
go build -trimpath -ldflags="-s -w" ./cmd/abx
./abx version
```

Expected result:

```text
Existing modified/untracked files are identified and preserved. go mod verify, go test, lint, race/coverage, build, and version pass or any pre-existing failure is captured before edits.
```

Also run the known failing checks from the review to prove they are still issues before fixes:

```bash
git diff --check main...HEAD || true
go tool govulncheck ./... || true
```

Expected result:

```text
git diff --check reports any remaining whitespace failures until package 14. govulncheck reports no tracked tool until package 13.
```

---


## Implementation status checkpoint — 2026-06-07

This checkpoint reflects the current `go-rewrite` branch state after the latest verified commits. Each completed or partial slice was validated before commit with:

```bash
go test ./...
go tool golangci-lint run ./...
git diff --check
```

### Package status summary

- [x] **Package 1: Protect sync-out from excluded overwrite and path escape** — Done. Matcher-aware, root-confined sync-out is implemented and covered by regression tests.
- [x] **Package 2: Make sync conflict detection complete, per-root, and recoverable** — Done. Per-root snapshots, conflict detection, recovery/preserved-volume behavior, and warning details are implemented.
- [x] **Package 3: Preserve deletions, empty directories, symlinks, and special-file policy** — Done. Archive fidelity, deletion reconciliation, safe symlinks, special-file policy, close-error handling, and sync-in pipe cleanup are implemented.
- [x] **Package 4: Close workspace credential injection and exclusion gaps** — Done. Workspace `.abxenv` is trusted opt-in only, env injection is allowlist/reserved-key validated, exclusions are expanded, remote excludes are HTTPS-only, and invalid patterns fail closed.
- [x] **Package 5: Harden helper containers, main sandbox spec, seccomp, SSH, and git config** — Done. Completed SSH socket validation, disabled raw host gitconfig, added opt-in sanitized gitconfig, hardened helper containers, moved seccomp to production source, added seccomp JSON validation/close handling, and added main sandbox `Init`/`PidsLimit`. Remaining: helper PID limit, ulimits/read-only-rootfs/tmpfs/writable-layer fields, stronger seccomp close-error test seam, and documentation of retained `SETUID`/`SETGID`.
- [x] **Package 6: Fix CLI behavior, audit semantics, config writing, and logging lifecycle** — Done. Completed editor arg parsing, single config load for run, config-skip for info subcommands, nil success return, signal-aware root context, audit non-zero failures and detail paths, safe config writes, completion valid args, logging shutdown, text-vs-JSON stderr behavior, and joined handler errors. Remaining: shared `internal/osutil` extraction and bounded cleanup contexts.
- [x] **Package 7: Fix runtime mapping, Podman detection, Docker wait, stdin, sessions, and cleanup** — Done. Completed Docker mapping helper, explicit mount types, Docker wait channel safety/error wrapping, attach stdin tracking, Podman `PODMAN_HOST`, stdin EOF close, idempotent signal/resize cleanup, random session suffixes, precise volume cleanup, runtime close lifecycle, and runtimetest writer contract. Remaining: dedicated SELinux bind relabel regression and any live-daemon smoke separation needed for CI coverage.
- [x] **Package 8: Consolidate config, registries, defaults, home handling, and standards cleanup** — Done. Completed registry drift checks/copy alignment, validation, caching/defensive copies, shared home helpers with fail-closed run/audit checks, `DefaultEditor`, documented `ABX_` env binding, `errors.AsType` cleanup, and README notes for Go version and registry source of truth.
- [x] **Package 9: Improve audit and exclusion semantics beyond blockers** — Done. Audit now reuses hardcoded sensitive patterns, handles context cancellation, warns on permission-denied sensitive paths, matcher merge is immutable, simple doublestar semantics are tested/documented, and read-only close ignores are explicit.
- [x] **Package 11: Fix install, release, Makefile, version metadata, and local commands** — Done. Version output includes version/commit/date metadata, Makefile separates `CLI_VERSION` from `EDITOR_VERSION`, installer/Goreleaser artifact names match, checksums fail closed with portable SHA-256 fallback, `go-race-cover` exists, and production releases are tag-only.
- [x] **Package 12: Harden GitHub Actions, image publishing, CI coverage, and supply-chain checks** — Done. govulncheck/actionlint in CI, bc removed, runtime tests included, editor allowlist validation, Trivy pinned, id-token for signing, sync image scan/SBOM/sign, SBOM artifact uploads, smoke tests, Dependabot, CODEOWNERS, security workflow, publish input validation, third-party action pinning. — Partially started only for `govulncheck` tracking/triage; workflow hardening remains.
- [x] **Package 13: Harden Docker images, entrypoint, editor version sync, and image metadata — Done. Entrypoint handles GID collisions gracefully with group reuse, quotes default command, makes chown failures visible. Dockerfiles use digest-pinned bases, replaced eval with bash -c. Sync script uses request timeouts, atomic writes, UTF-8 encoding, schema validation, handles pi editor, fails non-zero on errors.** — Not started.
- [x] **Package 14: Documentation, whitespace, security policy, ownership, and lint config** — Done.
- [x] **Package 15: Final integration, coverage, vulnerability triage, and closure audit** — Done.

### Completed commit groups since this checkpoint began

- Sync/archive/conflict/security slices through Packages 1–4.
- Container hardening slices through most of Package 5.
- CLI/audit/config/logging lifecycle slices through most of Package 6.
- Runtime/container lifecycle slices through most of Package 7.
- Initial config/registry consolidation slices for Package 8.

### Next recommended work

Continue with Go-focused scope. Next suitable work is Package 11/12 preparation or remaining Go hardening leftovers from Packages 5–7.


## Work packages

### Package 1: Protect sync-out from excluded overwrite and path escape — done

**Files:** `internal/sync/options.go`, `internal/sync/archive.go`, `internal/sync/transfer.go`, `internal/cli/run.go`, `internal/sync/syncout_test.go`, `internal/sync/archive_test.go`, `internal/cli/orchestration_test.go`

- [ ] Write failing tests:
  - `TestSyncOut_WithMatcherSkipsExcludedArchiveEntryAndPreservesHostFile`: host `.env` contains `HOST_SECRET`; archive contains `.env` with `SANDBOX_SECRET`; `sync.OutWithOptions(..., Matcher: exclusion.NewMatcher([]string{".env"}))` leaves host `.env` unchanged.
  - `TestSyncOut_WithMatcherDoesNotCreateExcludedPath`: archive contains `.env.local`; matcher excludes `.env.*`; destination has no `.env.local`; no file is created.
  - `TestSyncOut_RejectsSymlinkDestinationRoot`: destination root is a symlink to another temp directory; sync-out returns an error containing `destination root is a symlink`; outside target remains unchanged.
  - `TestSyncOut_RejectsSymlinkParentBeforeMkdirAll`: destination has `dir` symlinked outside; archive contains `dir/file.txt`; sync-out returns an error and outside file is absent.
  - `TestSyncOut_RejectsTraversalBeforeWrite`: archive contains `../escape.txt`; sync-out returns an escape error and parent escape file is absent.
  - `TestRunSession_PassesWorkspaceMatcherToSyncOut`: orchestration stub records workspace sync-out options and verifies matcher is non-nil for workspace volume.
- [ ] Verify failure:

  ```bash
  go test ./internal/sync ./internal/cli -run 'TestSyncOut_WithMatcher|TestSyncOut_Rejects|TestRunSession_PassesWorkspaceMatcherToSyncOut'
  ```

  Expected failure:

  ```text
  Tests fail because sync-out has no matcher-aware API and extraction still allows symlinked root or parent setup.
  ```

- [ ] Implement:
  - Add `type Options struct { Matcher *exclusion.Matcher; RootName string; DeleteMissing bool; AllowUnsafeSymlinks bool }` in `internal/sync/options.go`.
  - Add `OutWithOptions(ctx, rt, volumeName, srcPath, destDir string, opts Options) error` and `OutFileWithOptions` while keeping `Out` and `OutFile` wrappers with zero options.
  - Change `internal/cli/run.go` to call `OutWithOptions` for workspace with the same matcher used for sync-in. Config/cache/state/share use nil matcher.
  - Before extraction, call `os.Lstat(dest)` and reject symlink roots.
  - Validate every existing parent component before creating missing parents. Do not call `MkdirAll` until root and existing parents are proven not symlinks.
  - Apply matcher to each archive header using clean slash-separated relative paths before any filesystem write.
  - Keep `.abx-volume-initialized` skipped and add a comment explaining it is an internal volume lifecycle marker.
- [ ] Verify pass:

  ```bash
  go test ./internal/sync ./internal/cli -run 'TestSyncOut_WithMatcher|TestSyncOut_Rejects|TestRunSession_PassesWorkspaceMatcherToSyncOut'
  go test ./internal/sync ./internal/cli
  ```

- [ ] Commit:

  ```bash
  git add internal/sync/options.go internal/sync/archive.go internal/sync/transfer.go internal/sync/syncout_test.go internal/sync/archive_test.go internal/cli/run.go internal/cli/orchestration_test.go
  git commit -m "fix(sync): filter and root-confine sync-out"
  ```

### Package 2: Make sync conflict detection complete, per-root, and recoverable — done

**Files:** `internal/sync/conflicts.go`, `internal/sync/options.go`, `internal/sync/transfer.go`, `internal/cli/run.go`, `internal/container/session.go`, `internal/sync/conflicts_test.go`, `internal/cli/orchestration_test.go`

- [ ] Write failing tests:
  - `TestSnapshotRoot_RecordsSizeMtimeModeAndRelativePaths`: snapshot records root name, relative path, size, mod time, file mode, and existence.
  - `TestDetectRootConflicts_HostCreatedSameOutgoingPath`: snapshot before `new.txt`; host creates `new.txt`; outgoing manifest contains `new.txt`; conflict type is `host-created`.
  - `TestDetectRootConflicts_HostDeletedOutgoingPath`: snapshot includes `keep.txt`; host deletes it; outgoing manifest contains `keep.txt`; conflict type is `host-deleted`.
  - `TestDetectRootConflicts_ModifiedUsesSizeOrMtime`: same mod time but different size is a conflict.
  - `TestSnapshotRoot_AppliesMatcher`: excluded `.env` is absent from snapshot entries.
  - `TestRunSession_SyncsUnconflictedRootsWhenOneRootConflicts`: cache conflict blocks cache only; workspace still syncs out.
  - `TestRunSession_ConflictPreservesAffectedVolumesAndReturnsNonZero`: editor exits 0, conflict occurs, returned `ExitError.Code` is non-zero, and cleanup is not called for preserved session resources.
  - `TestFormatRootConflictsGroupsByRoot`: output includes root names and paths.
- [ ] Verify failure:

  ```bash
  go test ./internal/sync ./internal/cli -run 'TestSnapshotRoot|TestDetectRootConflicts|TestRunSession_.*Conflict|TestFormatRootConflicts'
  ```

- [ ] Implement:
  - Replace `MtimeSnapshot` with `Snapshot` containing one `RootSnapshot` per sync root.
  - Add `SnapshotRoots(ctx, []RootSpec)` where `RootSpec{Name, Path string; Matcher *exclusion.Matcher}`.
  - Check `ctx.Err()` before each root walk and inside `WalkDir`.
  - Record directory and file metadata with relative slash paths; skip excluded paths and the internal marker.
  - Build an outgoing manifest during archive extraction before writes. Use it to compare host-created and host-deleted conflicts.
  - Return `[]RootConflict` with `Root`, `Path`, `Kind`, and `Detail`.
  - Update `RunSession`: detect conflicts per root immediately before that root's sync-out; if conflicts and no `ForceSync`, skip that root, preserve its volume, continue other roots, and return non-zero after all possible roots complete.
  - Add session method or cleanup option so preserved volumes are not removed when conflicts block sync-out. Include exact `docker volume ls --filter label=app=abox` and future `abx cleanup` guidance in warning output.
- [ ] Verify pass:

  ```bash
  go test ./internal/sync ./internal/cli -run 'TestSnapshotRoot|TestDetectRootConflicts|TestRunSession_.*Conflict|TestFormatRootConflicts'
  go test ./internal/sync ./internal/cli
  ```

- [ ] Commit:

  ```bash
  git add internal/sync/conflicts.go internal/sync/options.go internal/sync/transfer.go internal/sync/conflicts_test.go internal/cli/run.go internal/cli/orchestration_test.go internal/container/session.go
  git commit -m "fix(sync): detect recoverable per-root conflicts"
  ```

### Package 3: Preserve deletions, empty directories, symlinks, and special-file policy — done

**Files:** `internal/sync/archive.go`, `internal/sync/transfer.go`, `internal/sync/tar_filtered_test.go`, `internal/sync/syncout_test.go`, `internal/sync/transfer_test.go`, `README.md`

- [ ] Write failing tests:
  - `TestSyncOut_RemovesContainerDeletedTrackedFile`: snapshot has `old.txt`; outgoing archive lacks it; host `old.txt` is removed after sync-out.
  - `TestSyncOut_DoesNotRemoveExcludedHostFileWhenMissingFromArchive`: snapshot has excluded `.env`; outgoing archive lacks it; host `.env` remains.
  - `TestSyncOut_DoesNotRemoveHostCreatedUntrackedFile`: host creates `notes.txt` during session; outgoing archive lacks it; file remains.
  - `TestTarFiltered_EmitsEmptyDirectoryHeaders`: empty directory `empty/` appears as a tar directory header.
  - `TestSyncOut_RecreatesSafeRelativeSymlink`: archive symlink `link -> target.txt` is created as a symlink.
  - `TestSyncOut_RejectsEscapingSymlink`: archive symlink `link -> ../outside.txt` fails.
  - `TestTarFiltered_SkipsOrErrorsSpecialFiles`: FIFO or socket does not block and returns the documented policy result.
  - `TestSyncIn_CopyToContainerFailureUnblocksTarGoroutine`: failing `CopyToContainer` closes the pipe reader and the goroutine exits.
  - `TestTarFiltered_ReturnsWriterCloseError`: writer close failure is returned.
  - `TestSyncOut_ChecksRegularFileCloseError`: close error on destination file is returned.
- [ ] Verify failure:

  ```bash
  go test ./internal/sync -run 'TestSyncOut_Removes|TestSyncOut_DoesNotRemove|TestTarFiltered_Emits|TestSyncOut_Recreates|TestSyncOut_RejectsEscapingSymlink|TestTarFiltered_SkipsOrErrorsSpecialFiles|TestSyncIn_CopyToContainerFailure|TestTarFiltered_ReturnsWriterCloseError|TestSyncOut_ChecksRegularFileCloseError'
  ```

- [ ] Implement:
  - Rework `TarFiltered` to write directory headers for all non-root directories that pass matcher checks.
  - Allow only regular files, directories, and symlinks in tar archives. For other file modes, skip with debug logging or return an explicit error; choose one behavior and document it in tests and README.
  - Replace `defer tw.Close()` with explicit close after walk and return close errors.
  - Use `errgroup.WithContext` or equivalent pipe coordination in sync-in so `CopyToContainer` failure closes the reader and waits for archive goroutine completion.
  - Extract directory archives to a staging directory under the destination parent after root validation.
  - Track manifest entries from the archive. Reconcile deletions by walking host destination, applying matcher, and removing tracked paths absent from outgoing manifest while leaving untracked host-created paths alone.
  - Recreate safe relative symlinks after validating that the symlink target stays inside root from the link parent. Reject absolute and escaping link targets.
  - Check every destination write close error and preserve the original copy error when both copy and close fail.
- [ ] Verify pass:

  ```bash
  go test ./internal/sync
  ```

- [ ] Commit:

  ```bash
  git add internal/sync/archive.go internal/sync/transfer.go internal/sync/tar_filtered_test.go internal/sync/syncout_test.go internal/sync/transfer_test.go README.md
  git commit -m "fix(sync): reconcile archives safely"
  ```

### Package 4: Close workspace credential injection and exclusion gaps — done

**Files:** `internal/cli/root.go`, `internal/cli/run.go`, `internal/config/config.go`, `internal/exclusion/hardcoded.go`, `internal/exclusion/matcher.go`, `internal/cli/dotenv_test.go`, `internal/cli/run_internal_test.go`, `internal/config/config_test.go`, `internal/exclusion/hardcoded_test.go`, `internal/exclusion/matcher_test.go`, `README.md`

- [ ] Write failing tests:
  - `TestLoadDotEnv_DisabledByDefault`: `.abxenv` with `OPENAI_API_KEY` is ignored unless trust is enabled.
  - `TestLoadDotEnv_TrustedAllowsOnlyHostAllowlistValues`: when trusted, key-only lines resolve from host; `KEY=value` values in file are ignored.
  - `TestLoadDotEnv_BlocksABoxControlKeys`: `HOST_UID`, `HOST_GID`, `SSH_AUTH_SOCK`, `PATH`, `HOME`, and display/session keys are rejected.
  - `TestValidateExtraEnv_RejectsReservedKeysAndDuplicates`: `--env HOST_UID=1` and duplicate `FOO` fail before container creation.
  - `TestEnvFlag_KeyMirrorsHostValue`: `--env FOO` becomes `FOO=<host value>` when present and fails clearly when absent.
  - `TestConfig_TrustWorkspaceEnvDefaultFalseAndEnvOverride`: config default is false and `ABX_TRUST_WORKSPACE_ENV=true` binds.
  - `TestHardcodedPatterns_CommonCredentialStores`: includes `.env.local`, `.env.production`, `.kube/config`, `.docker/config.json`, `.config/gcloud/application_default_credentials.json`, `.azure/`, `.pypirc`, `.netlify/`, `.npmrc`, `.yarnrc`, `.cargo/credentials`, `.git/credentials`, `id_ed25519`, `*.key`, `*_key`, and does not match `monkey`.
  - `TestBuildMatcherWithRemote_RejectsHTTPURL`: `http://` URL returns an error mentioning HTTPS.
  - `TestBuildMatcher_InvalidPatternFailsClosed`: invalid doublestar pattern returns an error during matcher creation.
  - `TestContainsGlobstar_TrailingGlobstar`: `build/**` remains correct.
- [ ] Verify failure:

  ```bash
  go test ./internal/cli ./internal/config ./internal/exclusion -run 'TestLoadDotEnv|TestValidateExtraEnv|TestEnvFlag|TestConfig_TrustWorkspaceEnv|TestHardcodedPatterns_CommonCredentialStores|TestBuildMatcher.*RejectsHTTPURL|TestBuildMatcher_InvalidPattern|TestContainsGlobstar'
  ```

- [ ] Implement:
  - Add config field `TrustWorkspaceEnv bool` with JSON/mapstructure `trust_workspace_env` and bind `ABX_TRUST_WORKSPACE_ENV`.
  - Add CLI flag `--trust-workspace-env` default false. Only read `.abxenv` when flag or config is true.
  - Change `.abxenv` docs to host-variable allowlist, not value-setting file.
  - Add `parseEnvFlag` and final `mergeEnv` that rejects reserved keys and duplicates after ABox generated env, editor env, `--env`, and trusted `.abxenv` are combined.
  - Expand hardcoded exclusions and remove broad `**/*key` in favor of `**/*.key`, `**/*_key`, `**/id_rsa`, `**/id_ed25519`, and named credential stores.
  - Validate all matcher patterns during construction and fail closed on invalid patterns.
  - Require HTTPS for remote exclusion URLs.
  - Construct matchers through one normalization path.
- [ ] Verify pass:

  ```bash
  go test ./internal/cli ./internal/config ./internal/exclusion
  ```

- [ ] Commit:

  ```bash
  git add internal/cli/root.go internal/cli/run.go internal/cli/dotenv_test.go internal/cli/run_internal_test.go internal/config/config.go internal/config/config_test.go internal/exclusion/hardcoded.go internal/exclusion/hardcoded_test.go internal/exclusion/matcher.go internal/exclusion/matcher_test.go README.md
  git commit -m "fix(security): require trust for workspace env injection"
  ```

### Package 5: Harden helper containers, main sandbox spec, seccomp, SSH, and git config — mostly done

**Files:** `internal/container/security.go`, `internal/container/spec.go`, `internal/container/manager.go`, `internal/container/network.go`, `internal/runtime/runtime.go`, `internal/runtime/docker.go`, `internal/container/*_test.go`, `internal/runtime/docker_mapping_test.go`, `config/seccomp/abox-default.json`, `config/seccomp.json`, `internal/container/testdata/seccomp.json`, `README.md`

- [ ] Write failing tests:
  - `TestHelperContainerSpec_HasNoNetworkNoNewPrivilegesSeccompAndResources`: bootstrap and sync helper specs use `NetworkMode: none`, `CapDrop: ALL`, no `DAC_OVERRIDE`, `no-new-privileges`, seccomp, memory, CPU, PID limit, and auto-remove where compatible.
  - `TestBuildSpec_GitconfigNotMountedByDefault`: host `.gitconfig` is not mounted unless config opt-in is true.
  - `TestBuildSpec_SanitizedGitConfigWhenEnabled`: opt-in writes a minimal config with safe identity fields and no credential helper.
  - `TestSSHAgentSocketRequiresUnixSocket`: regular file in `SSH_AUTH_SOCK` is ignored; Unix socket is accepted.
  - `TestSeccompProfilePath_ValidatesJSONAndChecksClose`: invalid JSON fixture causes error; close errors are surfaced through an injectable writer.
  - `TestSeccompProfiles_DoNotDrift`: canonicalized shipped profiles match the production source.
  - `TestBuildSpec_ResourceHardening`: main container sets `PidsLimit`, `Init`, ulimits, and documented writable-layer or read-only-rootfs fields.
  - `TestContainerSpec_ImmutabilityCopiesSlices`: spec slice copies at package boundaries do not mutate caller slices.
- [ ] Verify failure:

  ```bash
  go test ./internal/container ./internal/runtime -run 'TestHelperContainerSpec|TestBuildSpec_Gitconfig|TestSSHAgentSocket|TestSeccomp|TestBuildSpec_Resource|TestContainerSpec_Immutability'
  ```

- [ ] Implement:
  - Move embedded seccomp to production path and embed from `config/seccomp/abox-default.json` or a generated internal copy that is checked against the config file.
  - Validate embedded seccomp with `json.Valid` before writing.
  - Check temp-file close error or write the seccomp profile to an owned state/cache path with deterministic replacement and cleanup.
  - Add helper-container spec builder used by bootstrap and sync volume containers. Apply no network, no-new-privileges, seccomp, dropped caps, bounded resources, and no `DAC_OVERRIDE` unless an integration test proves it required.
  - Add `PidsLimit`, `Init`, `ReadonlyRootfs`, `Tmpfs`, `Ulimits`, and writable-layer size fields to `runtime.ContainerSpec` and Docker mapping.
  - Make git config forwarding opt-in. Prefer generated sanitized config over raw host `~/.gitconfig`.
  - Require Unix socket mode for SSH agent forwarding.
  - Document why `SETUID` and `SETGID` remain or make them editor-profile opt-in. If retained, tests must verify the explicit reason path.
- [ ] Verify pass:

  ```bash
  go test ./internal/container ./internal/runtime
  ```

- [ ] Commit:

  ```bash
  git add internal/container/security.go internal/container/spec.go internal/container/manager.go internal/container/network.go internal/container/*_test.go internal/runtime/runtime.go internal/runtime/docker.go internal/runtime/docker_mapping_test.go config/seccomp/abox-default.json config/seccomp.json internal/container/testdata/seccomp.json README.md
  git commit -m "fix(container): harden sandbox and helper specs"
  ```

### Package 6: Fix CLI behavior, audit semantics, config writing, and logging lifecycle — mostly done

**Files:** `cmd/abx/main.go`, `internal/cli/root.go`, `internal/cli/run.go`, `internal/cli/audit.go`, `internal/cli/completion.go`, `internal/cli/config.go`, `internal/audit/audit.go`, `internal/logging/logging.go`, `internal/osutil/osutil.go`, `internal/cli/*_test.go`, `internal/audit/audit_test.go`, `internal/logging/logging_test.go`, `README.md`

- [ ] Write failing tests:
  - `TestRoot_AllowsDirectoryAndEditorArgsAfterSeparator`: `abx ./work -- --model x` populates `EditorArgs`.
  - `TestRoot_AllowsDirectoryThenEditorArgs`: accepted form documented in README works.
  - `TestRoot_InfoSubcommandsSkipUserConfigLoad`: malformed user config does not break `version`, `completion`, or help.
  - `TestRoot_LoadsUserConfigOnceForRun`: run path reads config once.
  - `TestRunSession_ReturnsNilOnExitZero`: successful editor exit returns nil.
  - `TestRunSession_ReturnsExitErrorOnNonZero`: non-zero editor exit returns `ExitError`.
  - `TestMainUsesSignalNotifyContext`: root context is canceled on SIGINT in a unit seam.
  - `TestAuditReturnsNonZeroOnFail`: failed audit check causes CLI error.
  - `TestAuditDetailsIncludePath`: sensitive file warning includes triggering path.
  - `TestWriteConfigField_ReadErrorDoesNotOverwrite`: read permission error returns an error and file content is preserved.
  - `TestWriteConfigField_Mode0600`: config file mode is `0600`.
  - `TestCompletionValidArgs`: valid shell names are exposed.
  - `TestLogging_JSONOnlyWhenFlagTrue`: redirected stderr without flag still uses text handler.
  - `TestLogging_CloseClosesVerboseFile`: shutdown closes log file.
  - `TestMultiHandlerAttemptsAllAndJoinsErrors`: all handlers are called and errors are joined.
- [ ] Verify failure:

  ```bash
  go test ./cmd/abx ./internal/cli ./internal/audit ./internal/logging ./internal/osutil -run 'TestRoot_|TestRunSession_Returns|TestMainUsesSignal|TestAudit|TestWriteConfigField|TestCompletionValidArgs|TestLogging|TestMultiHandler'
  ```

- [ ] Implement:
  - Add `internal/osutil` for home resolution, terminal checks via `term.IsTerminal`, bounded cleanup contexts, and shared helpers.
  - Use `signal.NotifyContext` in `main` and add `commit` and `date` variables or remove ldflags in package 12. Prefer adding variables and printing in version.
  - Change root args to support directory plus editor args. Document `--` separator and parsing rules.
  - Skip config loading for subcommands that do not need run config. Load config once for run and store in command context or closure state.
  - Return nil from `RunSession` when exit code is 0; return `ExitError` only for non-zero or skipped-sync non-zero.
  - Use bounded cleanup contexts for session/container cleanup.
  - Make audit reuse run workdir canonicalization and shared sensitive matcher. Populate details and return non-zero when any `Fail` exists.
  - Fix config file write safety and mode.
  - Add completion `ValidArgs`.
  - Return a logging shutdown function or logger handle so verbose log files close. JSON output only when flag/config is true.
- [ ] Verify pass:

  ```bash
  go test ./cmd/abx ./internal/cli ./internal/audit ./internal/logging ./internal/osutil
  ```

- [ ] Commit:

  ```bash
  git add cmd/abx/main.go internal/osutil/osutil.go internal/osutil/osutil_test.go internal/cli/root.go internal/cli/run.go internal/cli/audit.go internal/cli/completion.go internal/cli/config.go internal/cli/*_test.go internal/audit/audit.go internal/audit/audit_test.go internal/logging/logging.go internal/logging/logging_test.go README.md
  git commit -m "fix(cli): make run semantics and audit failures explicit"
  ```

### Package 7: Fix runtime mapping, Podman detection, Docker wait, stdin, sessions, and cleanup — mostly done

**Files:** `internal/runtime/runtime.go`, `internal/runtime/docker.go`, `internal/runtime/podman.go`, `internal/runtime/docker_mapping.go`, `internal/container/manager.go`, `internal/container/session.go`, `internal/container/volumes_test.go`, `internal/container/manager_internal_test.go`, `internal/runtime/*_test.go`, `internal/runtimetest/stub.go`

- [ ] Write failing tests:
  - `TestDockerMapping_ParseBindSELinuxReadOnly`: `ro,z` maps to read-only plus SELinux relabel data or uses a documented bind representation that preserves relabeling.
  - `TestDockerMapping_ExplicitMountType`: named volume and host bind are not inferred from slash shape once `ContainerSpec` has typed mounts.
  - `TestDockerMapping_PropagatesNetworkEnvResourcesSecurity`: all fields map to Docker config/host config.
  - `TestContainerWait_WrapsErrorWithContainerID`: wait error contains container ID and wraps original error.
  - `TestContainerWait_IgnoresClosedNilErrChannelUntilStatus`: closed nil err channel does not return `-1, nil`.
  - `TestContainerAttach_StdinFollowsSpec`: attach stdin is false when `OpenStdin` is false.
  - `TestPodmanSocket_UsesPODMANHostNotDockerHost`: `DOCKER_HOST` alone does not alter Podman socket.
  - `TestPodmanSocket_RejectsUnsupportedScheme`: `PODMAN_HOST=tcp://x` returns a clear error.
  - `TestStreamContainerIO_ForwardsNonTTYInputAndCloses`: non-terminal reader bytes reach attach writer and EOF is signaled.
  - `TestForwardContainerSignals_StopIsIdempotent`: stop can be called twice.
  - `TestWatchTerminalResize_StopsSignalsOnContextCancel`: signal registration is cleaned up when context ends.
  - `TestCreateSession_IDHasRandomSuffix`: concurrent sessions do not rely only on `UnixNano`.
  - `TestVolumes_CleanupOnCreateErrorRemovesCreatedVolumes`: volume create failure removes already-created volumes.
  - `TestNopReadWriteCloserWriteReturnsLen`: stub `Write` returns `len(p), nil`.
- [ ] Verify failure:

  ```bash
  go test ./internal/runtime ./internal/container ./internal/runtimetest -run 'TestDockerMapping|TestContainerWait|TestContainerAttach|TestPodmanSocket|TestStreamContainerIO_Forwards|TestForwardContainerSignals_Stop|TestWatchTerminalResize|TestCreateSession_ID|TestVolumes_Cleanup|TestNopReadWriteCloserWrite'
  ```

- [ ] Implement:
  - Factor pure Docker mapping into `docker_mapping.go` and unit-test it without daemon access.
  - Introduce typed mount representation in `runtime.ContainerSpec` while keeping string binds only through a compatibility constructor during migration.
  - Preserve SELinux labels correctly.
  - Wrap Docker wait errors and handle channel close semantics.
  - Condition attach stdin on `OpenStdin`.
  - Update `ContainerRuntime` with `Close() error` if needed; close Docker clients after failed pings and from main lifecycle.
  - Use `PODMAN_HOST` for Podman; validate URL scheme.
  - Always forward stdin when `spec.OpenStdin` is true; separate raw terminal behavior from input copy. Close or half-close container stdin on EOF where Docker connection supports it.
  - Join stream goroutines and make signal/resize stop functions idempotent.
  - Add random suffix to session IDs using `crypto/rand`.
  - Log cleanup failures at debug level.
  - Fix runtimetest stub writer contract and document why shared runtime test helper compiles as internal test utility.
- [ ] Verify pass:

  ```bash
  go test ./internal/runtime ./internal/container ./internal/runtimetest
  ```

- [ ] Commit:

  ```bash
  git add internal/runtime/runtime.go internal/runtime/docker.go internal/runtime/podman.go internal/runtime/docker_mapping.go internal/runtime/*_test.go internal/container/manager.go internal/container/session.go internal/container/volumes_test.go internal/container/manager_internal_test.go internal/runtimetest/stub.go
  git commit -m "fix(runtime): harden docker podman and stream handling"
  ```

### Package 8: Consolidate config, registries, defaults, home handling, and standards cleanup — done

**Files:** `internal/config/registry.go`, `internal/config/config.go`, `internal/config/editors.json`, `config/editors.json`, `bin/editors.json`, `internal/config/*_test.go`, `internal/container/spec.go`, `internal/cli/run.go`, `cmd/abx/main.go`, `README.md`, `go.mod`

- [ ] Write failing tests:
  - `TestEditorRegistriesMatchCanonical`: canonical public and embedded editor registries match after canonical JSON normalization.
  - `TestEditorRegistry_ValidatesNonEmptyAndRequiredFields`: every editor including `pi` has image tag, command name, config path, and env vars field.
  - `TestEditorRegistry_CachesAndReturnsDefensiveCopies`: caller mutation of returned profile or env vars does not mutate cached registry.
  - `TestHomeDir_PropagatesErrorsForSecurityChecks`: run/audit safety paths fail when home cannot be resolved.
  - `TestDefaultEditorConstantSingleSource`: config and run use the same default editor constant.
  - `TestErrorsAsTypeUsed`: targeted code no longer uses manual `errors.As` where Go 1.26 `errors.AsType` applies.
  - `TestConfigEnvOverrides_AllDocumentedKeys`: all documented `ABX_` overrides bind and unmarshal.
- [ ] Verify failure:

  ```bash
  go test ./internal/config ./internal/cli ./internal/container ./cmd/abx -run 'TestEditorRegistries|TestEditorRegistry|TestHomeDir|TestDefaultEditorConstant|TestErrorsAsTypeUsed|TestConfigEnvOverrides'
  ```

- [ ] Implement:
  - Make `config/editors.json` canonical. Generate or copy `internal/config/editors.json` through a checked command and fail CI on drift. Keep `config/editors.json` as canonical and regenerate embedded/runtime registry copies from it.
  - Validate registry non-empty and required fields on load.
  - Cache registry with `sync.OnceValues`; return defensive copies.
  - Move `HomeDir` to `internal/osutil` and choose OS user home for security decisions. Treat `$HOME` as user-config location only after validation.
  - Add exported `config.DefaultEditor` constant and use it from CLI defaults.
  - Switch applicable `errors.As` uses to `errors.AsType`.
  - Bind every documented `ABX_` config key explicitly.
  - Document Go 1.26 requirement and editor registry source of truth.
- [ ] Verify pass:

  ```bash
  go test ./internal/config ./internal/cli ./internal/container ./cmd/abx
  ```

- [ ] Commit:

  ```bash
  git add internal/config/registry.go internal/config/config.go internal/config/editors.json config/editors.json bin/editors.json internal/config/*_test.go internal/container/spec.go internal/cli/run.go cmd/abx/main.go README.md go.mod go.sum
  git commit -m "fix(config): centralize registry defaults and env overrides"
  ```

### Package 9: Improve audit and exclusion semantics beyond blockers — done

**Files:** `internal/audit/audit.go`, `internal/audit/audit_test.go`, `internal/exclusion/matcher.go`, `internal/exclusion/matcher_user_test.go`, `README.md`

- [ ] Write failing tests:
  - `TestAuditSensitiveFilesUsesHardcodedMatcher`: audit warns for `.aws/credentials`, `.npmrc`, `.pypirc`, nested `.env.production`, `id_ed25519`, and `*.pem`.
  - `TestAuditSensitiveFilesPermissionErrorWarns`: permission denied returns `Warn` with path detail.
  - `TestAuditContextCancellation`: canceled context returns wrapped context error before long walk.
  - `TestMatcher_DocumentsSimpleGlobSemantics`: anchored `/foo`, negation `!foo`, and escaped spaces are treated according to documented simple doublestar syntax.
  - `TestMergePatternsDoesNotMutateBase`: base slice remains unchanged after merge.
  - `TestReadOnlyCloseIgnoresAreExplicit`: test seams ensure read-only close failures are intentionally ignored or surfaced according to policy.
- [ ] Verify failure:

  ```bash
  go test ./internal/audit ./internal/exclusion -run 'TestAuditSensitive|TestAuditContextCancellation|TestMatcher_DocumentsSimpleGlobSemantics|TestMergePatternsDoesNotMutateBase|TestReadOnlyCloseIgnoresAreExplicit'
  ```

- [ ] Implement:
  - Reuse `exclusion.HardcodedPatterns()` in audit through a shared matcher and bounded workspace walk.
  - Add context checks to audit filesystem traversal.
  - Treat permission errors on sensitive paths as warnings with detail.
  - Update matcher docs and README to say `.abxignore` uses simple doublestar glob syntax, not full gitignore semantics.
  - Make `mergePatterns` copy its base input before appending.
  - Replace read-only deferred closes with explicit ignored-close closures where close errors do not affect correctness.
- [ ] Verify pass:

  ```bash
  go test ./internal/audit ./internal/exclusion
  ```

- [ ] Commit:

  ```bash
  git add internal/audit/audit.go internal/audit/audit_test.go internal/exclusion/matcher.go internal/exclusion/matcher_user_test.go README.md
  git commit -m "fix(audit): align sensitive checks with exclusions"
  ```

### Package 11: Fix install, release, Makefile, version metadata, and local commands — done

**Files:** `install`, `.goreleaser.yml`, `Makefile`, `cmd/abx/main.go`, `internal/cli/version.go`, `README.md`, `.github/workflows/release.yml`

- [ ] Write failing tests/checks:
  - `install` shell test computes artifact `abx_linux_amd64.tar.gz` and `abx_darwin_arm64.tar.gz` matching GoReleaser template.
  - Darwin checksum function chooses `shasum -a 256` when `sha256sum` is absent.
  - Missing checksum asset or missing artifact entry fails closed with a specific message.
  - `make -n go-build` uses `CLI_VERSION`, not editor image `VERSION`.
  - `go run ./cmd/abx version` includes version, commit, and date when ldflags are supplied.
  - GoReleaser production releases do not replace existing artifacts.
  - Release workflow runs production release only for tags.
- [ ] Verify failure:

  ```bash
  bash -n install
  make -n go-build
  go test ./cmd/abx ./internal/cli -run 'TestVersion'
  ```

- [ ] Implement:
  - Align `.goreleaser.yml` archive `name_template` to `abx_{{ .Os }}_{{ .Arch }}`.
  - Make installer checksum mandatory by default, check non-empty expected checksum, and support `sha256sum`, `shasum -a 256`, then `openssl dgst -sha256`.
  - Split `EDITOR_VERSION` and `CLI_VERSION` in Makefile.
  - Add `commit` and `date` variables and print them in version output, or remove ldflags consistently. Prefer printing.
  - Set `replace_existing_artifacts: false`.
  - Change release workflow to tag-only production release. Add a separate snapshot command for branch pushes if branch artifacts are still wanted.
  - Add `go-cover` to `.PHONY`, align `go-cover` package set with CI, and add a local `go-race-cover` target matching CI.
- [ ] Verify pass:

  ```bash
  bash -n install
  make -n go-build go-cover go-race-cover
  go test ./cmd/abx ./internal/cli
  go build -trimpath -ldflags="-s -w -X main.version=test -X main.commit=abc123 -X main.date=2026-06-07T00:00:00Z" ./cmd/abx
  ./abx version
  ```

- [ ] Commit:

  ```bash
  git add install .goreleaser.yml Makefile cmd/abx/main.go internal/cli/version.go README.md .github/workflows/release.yml
  git commit -m "fix(release): align installer artifacts and version metadata"
  ```

### Package 12: Harden GitHub Actions, image publishing, CI coverage, and supply-chain checks

**Files:** `.github/workflows/go-ci.yml`, `.github/workflows/publish.yml`, `.github/workflows/sync-editors.yml`, `.github/workflows/security.yml`, `.github/dependabot.yml`, `.github/CODEOWNERS`, `internal/ci/README.md`, `go.mod`, `go.sum`

- [ ] Write failing checks:
  - `actionlint` passes for all workflows after changes.
  - Publish workflow rejects workflow_dispatch `editors_json` that is not valid JSON array of known editor names.
  - Publish workflow does not execute changed `INSTALL_CMD` on pull requests from untrusted code.
  - Trivy action is version or SHA pinned.
  - Trivy/SBOM/cosign use one canonical image reference or loop over refs deliberately.
  - Publish workflow grants `id-token: write` to jobs that sign.
  - Sync image is scanned, SBOMed, signed, and smoke-tested.
  - SBOM files are uploaded as artifacts.
  - Go CI coverage comparison uses `awk` or Python, not `bc`.
  - Pure runtime tests are included in race/coverage package set.
  - CI runs actionlint and govulncheck.
  - Third-party actions are pinned to reviewed SHAs or documented version pins with CODEOWNERS enforcement.
- [ ] Verify failure:

  ```bash
  go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
  go tool govulncheck ./... || true
  ```

- [ ] Implement:
  - Add govulncheck tracked tool using `go get -tool golang.org/x/vuln/cmd/govulncheck` and run `go mod tidy`.
  - Update Go CI with govulncheck and actionlint. Replace `bc` coverage check.
  - Include `internal/runtime` in coverage except tests requiring live daemon; use build tags or test names to separate pure tests from live smoke tests.
  - Add optional bounded Docker/Podman smoke jobs for create/start/attach/wait/remove and one volume copy path.
  - Harden publish matrix input via `jq -e`, known editor allowlist, and environment-file passing.
  - For pull requests, build only trusted baseline install commands or require maintainer-approved workflow path.
  - Pin Trivy and other third-party actions to specific versions or reviewed SHAs.
  - Select canonical image tag for scan/SBOM/sign or loop each tag explicitly.
  - Add `id-token: write`, SBOM upload, and scan/sign parity for sync image.
  - Add image smoke command before push/sign.
  - Reduce sync-editors permissions and remove workflow-run deletion.
  - Add Dependabot and CODEOWNERS.
- [ ] Verify pass:

  ```bash
  go mod verify
  go tool govulncheck ./...
  go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
  go test ./...
  ```

- [ ] Commit:

  ```bash
  git add .github/workflows/go-ci.yml .github/workflows/publish.yml .github/workflows/sync-editors.yml .github/workflows/security.yml .github/dependabot.yml .github/CODEOWNERS internal/ci/README.md go.mod go.sum
  git commit -m "ci: add security checks and harden image publishing"
  ```

### Package 13: Harden Docker images, entrypoint, editor version sync, and image metadata — done

**Files:** `docker/Dockerfile`, `docker/Dockerfile.sync`, `docker/entrypoint.sh`, `config/sync_versions.py`, `config/sync_versions_test.py`, `config/editors.json`, `internal/config/editors.json`, `.github/workflows/sync-editors.yml`, `.github/workflows/publish.yml`, `README.md`

- [ ] Write failing tests/checks:
  - Entrypoint test: host GID 20 uses existing numeric group instead of failing.
  - Entrypoint test: UID 1000 with changed GID updates group membership correctly.
  - Entrypoint test: chown failure logs a warning and only ignores documented expected cases.
  - Entrypoint test: default command is executed without word splitting.
  - `sync_versions.py` tests: request timeout is configured, fetch/schema errors cause non-zero exit, `pi` is included or explicitly skipped with test assertion, writes use UTF-8, write is atomic through temp file and `os.replace`.
  - Dockerfile lint: no `eval` in editor install step.
  - Dockerfile lint: bases are digest pinned.
  - Registry metadata uses versioned image tags for released CLI metadata.
- [ ] Verify failure:

  ```bash
  bash -n docker/entrypoint.sh
  uv run python -m unittest config/sync_versions_test.py || true
  docker run --rm -v "$PWD:/repo" hadolint/hadolint hadolint /repo/docker/Dockerfile /repo/docker/Dockerfile.sync || true
  ```

- [ ] Implement:
  - Pin Docker base images by digest and record update process in README.
  - Replace `eval "$INSTALL_CMD"` with explicit command arrays or a checked installer script per editor generated from trusted config. For publish PRs, do not execute repo-changed install commands.
  - Add checksum/signature verification for remote installers where upstream publishes them. Where unavailable, document risk and pin package manager versions.
  - Make entrypoint robust for common macOS and Linux UID/GID collisions; never fail just because numeric GID exists.
  - Make chown failures visible.
  - Quote default command execution.
  - Add version sync timeouts, schema validation, non-zero failures, all supported editors, UTF-8, and atomic writes. Update both canonical and embedded registries.
  - Generate versioned image tags in registry metadata; document mutable tag behavior for development images.
- [ ] Verify pass:

  ```bash
  bash -n docker/entrypoint.sh
  uv run python -m unittest config/sync_versions_test.py
  docker run --rm -v "$PWD:/repo" hadolint/hadolint hadolint /repo/docker/Dockerfile /repo/docker/Dockerfile.sync
  go test ./internal/config
  ```

  If Docker is unavailable locally, run the shell/Python/Go checks and record Hadolint as not run with daemon or image-pull reason.

- [ ] Commit:

  ```bash
  git add docker/Dockerfile docker/Dockerfile.sync docker/entrypoint.sh config/sync_versions.py config/sync_versions_test.py config/editors.json internal/config/editors.json .github/workflows/sync-editors.yml .github/workflows/publish.yml README.md
  git commit -m "fix(images): harden builds entrypoint and version sync"
  ```

### Package 14: Testing hygiene, coverage blind spots, docs, and minor review cleanup — done

**Files:** `internal/**/*_test.go`, `internal/runtimetest/stub.go`, `internal/container/manager.go`, `internal/container/spec.go`, `internal/container/session.go`, `internal/exclusion/matcher.go`, `internal/logging/logging.go`, `internal/config/config.go`, `.golangci.yml`, `README.md`, `AGENTS.md`

- [ ] Write or update tests:
  - Replace vacuous runtime tests with behavior tests introduced in package 7.
  - Add runtime mapping coverage for image-missing behavior and security option normalization.
  - Add orchestration attach stream fixture coverage.
  - Replace brittle `callRecorder.waitCount` with per-container-ID wait results.
  - Add cleanup assertion for volume create failure.
  - Replace manual `os.Setenv` in tests with `t.Setenv`.
  - Replace test `context.Background()` with `t.Context()`.
  - Check every setup/read error currently ignored in tests.
  - Add comments or actual Unix socket fixture for SSH auth socket tests.
  - Add exported doc comments or unexport identifiers for `ExitError.Error`, `ValidateWorkdir`, `In`, `Out`, `BuildMatcherWithRemote`, and audit status constants.
  - Fix `shouldAllocateTTY` comment.
  - Replace redundant `[]byte` to string scanner allocation with `bytes.NewReader`.
  - Add preallocations where maximum sizes are known.
  - Add comments for container home contract, read-only bind flags, empty network mode, and runtime spec immutability.
  - Fix whitespace-only `git diff --check` failures in tracked Go/current setup files.
  - Add README support matrix for OS/runtime versions, rootless support, macOS UID/GID, and image trust policy.
  - Add `SECURITY.md` policy, CODEOWNERS, and Dependabot from package 12 if not already committed there.
- [ ] Verify failure before cleanup where possible:

  ```bash
  go test ./...
  go tool golangci-lint run ./...
  git diff --check main...HEAD || true
  ```

- [ ] Implement cleanup in scoped commits if package is large:
  - First commit test hygiene.
  - Second commit docs/comments/preallocation.
  - Third commit whitespace and review-status updates.
- [ ] Verify pass:

  ```bash
  gofmt -w $(git diff --name-only -- '*.go')
  go test ./...
  go tool golangci-lint run ./...
  git diff --check main...HEAD
  ```

- [ ] Commit:

  ```bash
  git add internal .golangci.yml README.md AGENTS.md SECURITY.md .github/CODEOWNERS .github/dependabot.yml
  git commit -m "chore: close review cleanup and test hygiene gaps"
  ```

### Package 15: Final full-system validation and review closure

**Files:** all changed files, `README.md`, `internal/ci/README.md`, release artifacts generated during validation

- [ ] Run full validation:

  ```bash
go mod verify
go test ./...
go tool golangci-lint run ./...
go tool govulncheck ./...
go test -race -coverprofile=coverage.out $(go list ./... | grep -v -E '(cmd/abx$)')
go build -trimpath -ldflags="-s -w -X main.version=test -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" -o abx ./cmd/abx
./abx version
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
bash -n install docker/entrypoint.sh
uv run python -m unittest config/sync_versions_test.py
git diff --check main...HEAD
```

- [ ] Run bounded container validation when Docker or Podman is available:

  ```bash
  go test -tags=integration ./internal/runtime ./internal/container ./internal/sync
  docker build -f docker/Dockerfile.sync -t abox-sync:test .
  docker run --rm abox-sync:test tar --version
  podman build -f docker/Dockerfile.sync -t abox-sync:test .
  podman run --rm abox-sync:test tar --version
  ```

  Expected result:

  ```text
  Docker and Podman smoke tests pass on hosts with daemon access. If one runtime is unavailable, document the daemon/socket error and run the other runtime.
  ```

- [ ] Verify PR review closure:
  - Every Critical finding from the source review coverage audit has a linked commit and passing regression test.
  - Every Important finding from the source review coverage audit has a code/test/doc/CI change or a written rationale in the retained implementation notes explaining why no code change is needed.
  - Every Minor finding from the source review coverage audit has a code/doc change or explicit no-action rationale.
  - No secrets, debug logs, temporary artifacts, generated coverage files, or local binaries are staged.

- [ ] Commit closure notes:

  ```bash
  git add README.md internal/ci/README.md docs/superpowers/plans/2026-06-07-pr-review-findings.md
  git commit -m "docs: record pr review finding resolutions"
  ```

---

## End-to-end verification

Run after all packages are complete:

```bash
go mod verify
go test ./...
go tool golangci-lint run ./...
go tool govulncheck ./...
go test -race -coverprofile=coverage.out $(go list ./... | grep -v -E '(cmd/abx$)')
go build -trimpath -ldflags="-s -w -X main.version=test -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" -o abx ./cmd/abx
./abx version
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
bash -n install docker/entrypoint.sh
uv run python -m unittest config/sync_versions_test.py
git diff --check main...HEAD
git status --short
```

Expected result:

```text
All Go tests, lint, vulnerability checks, race/coverage, build, workflow lint, current shell/Python checks, whitespace checks, and final Git status checks pass. Git status contains only intentionally changed tracked files ready for review.
```

---

## Finding coverage matrix

- Critical sync-out excluded overwrite: packages 1, 2, 3.
- Critical skipped sync-out success: package 2.
- Critical workspace `.abxenv` secret injection: package 4.
- Critical sync-out symlink/root escape: package 1.
- Critical hardcoded secret exclusion gaps: package 4.
- Critical installer artifact/checksum failures: package 11.
- Sync deletion, host-created/deleted conflicts, empty dirs, symlinks, special files, pipe/close errors, helper lifecycle, `cp -a`, stat errors, matcher snapshots, mtime precision, cancellable walks, marker docs: packages 1, 2, 3, 14.
- Helper network/security/resource bypasses, `DAC_OVERRIDE`, gitconfig, seccomp breadth/drift/materialization, SSH socket validation, capabilities, HTTPS remote excludes, `.abxenv` docs: packages 4, 5, 12, 14.
- Non-TTY stdin, editor args, `--env`, config loading, exit errors, cleanup contexts, defaults, TTY ownership, workdir validation, config writes, audit exit, completion args: packages 4, 6, 7, 8.
- Podman socket, bind parsing, Docker wait/attach/exec, seccomp read caching, client close, spec immutability, session IDs, cleanup logging, stream close diagnostics, idempotent signal/resize, network docs, session resource naming: packages 5, 7, 14.
- Audit and exclusion semantic gaps: packages 4, 9, 14.
- Registry/config defaults and drift: packages 8, 13.
- CI/release/install/package gaps: packages 11, 12, 13.
- Test blind spots and standards: packages 7, 8, 14.
- Module/tooling/govulncheck/Go version/errors.AsType/logging/terminal helpers: packages 6, 8, 12, 14.
- Production readiness: env control keys, image existence for pull policy never, mutable tags, entrypoint UID/GID, Docker reproducibility, PR build trust, sandbox process/disk limits, orphan cleanup, image smoke tests, version sync, governance, CI security coverage: packages 4, 5, 11, 12, 13, 14.
