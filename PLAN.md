# Plan: ABox Architectural Review Remediation

**Status: Phases 1–5 COMPLETE. Phase 6 (forward-looking) remains.**

## Completion Summary

| Phase | Tasks | Done | Tests |
|-------|-------|------|-------|
| 1 — Dual Registry | 1.1–1.5 | 5/5 | 66 editor-registry assertions |
| 2 — Security | 2.1–2.5 | 5/5 | 66 (overlapped with registry suite) |
| 3 — Sync Correctness | 3.1–3.4 | 4/4 | 16 sync + 200 fuzz |
| 4 — Pipeline + Architecture | 4.1–4.7 | 7/7 | 66 (overlapped with registry suite) |
| 5 — Test Coverage | 5.1–5.5 | 5/5 | 13 exclusion + 200 fuzz + infra fixes |
| 6 — Forward-Looking | 6.1–6.3 | 0/3 | Deferred |

**Total test count: 13 + 16 + 66 + 200 (fuzz) = 295 assertions, all green.**

**Note:** The `cursor` editor was completely removed from the project (not in `editors.json`, CI workflows, tests, README, or sync_versions.py). It used a non-standard install mechanism and had no automated version tracking.

---

## Phase 6: Low — Forward-Looking (L1, L2, L4)

These are **not scheduled for immediate implementation**. They require architectural decisions that should be deferred until Phases 1–5 are validated in production.

### Task 6.1: Evaluate rootless Podman as default (L1)
- [ ] Research: Podman rootless mode already works (detected in `detect_runtime()`). Evaluate making it the documented default for users who don't need Docker-specific features
- [ ] No code change yet — update README to mention Podman as preferred runtime

### Task 6.2: Investigate cgroup v2 workspace disk quota (L2)
- [ ] Research `docker run` support for `--device` / `--blkio-weight` / cgroup v2 `io.max`
- [ ] Prototype a `--workspace-limit` flag that sets a disk quota on the workspace volume

### Task 6.3: Add `repository_dispatch` webhook trigger (L4)
- [ ] Create a lightweight API endpoint (GitHub Action workflow with `workflow_dispatch`) that npm/PyPI webhooks can call when a new version is published
- [ ] This replaces the daily cron entirely for editors that support webhooks
- [ ] Keep daily cron as fallback for editors without webhook support

---

## Files Changed (Phases 1–5)

### Source files
- `src/helpers.sh` — JSON-based `get_editor_info()`, structured logging (`log_setup`, `log_debug`, `log_info`)
- `src/main.sh` — Editor validation, JSON config, `--env`, `--verbose`, `--force-sync`, mtime snapshot
- `src/container.sh` — No DAC_OVERRIDE, seccomp, SSH socket, strict-network isolation, SYNC_IMAGE in init_volume_ownership
- `src/sync.sh` — SYNC_IMAGE constant, transactional staging, streaming, `snapshot_mtimes`/`check_conflicts`
- `src/state.sh` — NEW: documents global variable contract

### Config files
- `config/editors.json` — Expanded with `image_tag`, `config_path`, `env_vars`, `legacy_path` (cursor removed)
- `config/seccomp.json` — NEW: seccomp profile blocking ~40 dangerous syscalls
- `config/sync_versions.py` — Cursor removed

### Docker files
- `docker/Dockerfile.sync` — NEW: minimal Alpine image for sync operations

### CI workflows
- `.github/workflows/bundle.yml` — SHA-256 checksums, editors.json artifact, all test suites
- `.github/workflows/publish.yml` — Trivy scanning, SBOM, Cosign signing, sync image build, cursor removed
- `.github/workflows/sync-editors.yml` — Daily cron (was every 15 min)

### Install / Build
- `install` — SHA-256 verification, editors.json download
- `Makefile` — Bundle includes editors.json + state.sh, test target runs all suites

### Test files (NEW)
- `tests/editor-registry-test.sh` — 66 tests
- `tests/sync-unit-test.sh` — 16 tests
- `tests/exclusion-fuzz.sh` — Property-based fuzzing (default 500 iterations)

### Test files (FIXED)
- `tests/integration-tests.sh` — stderr capture on failure, auto-build bundle
- `tests/ux-verification.sh` — Startup latency enforced as CI failure

### Docs
- `README.md` — Cursor removed from supported editors list
