# ABox — Architectural Review 2026

> A full-stack analysis of the design, security model, automation pipeline, and operational
> characteristics of the `abx` agent sandbox, with concrete recommendations for improvement.

---

## Executive Summary

ABox is a well-intentioned, practically useful tool. The core security premise — ephemeral volumes
as an airlock between agent and host — is sound, and the implementation shows clear thinking about
threat models. However, the project is built almost entirely in Bash, deployed as a single bundled
script, and relies on a sync-over-tar strategy that has real correctness and performance failure
modes. At the current scale (one contributor, nine editors, a cron-triggered version sync), the
architecture is appropriate. The moment it grows — more users, more editors, more concurrent
sessions, or stricter compliance requirements — several structural decisions will become bottlenecks
or outright bugs.

This review covers six domains: the runtime and security model, the sync strategy, the shell
architecture, the image and build pipeline, the configuration and editor management system, and
the testing surface. Each section identifies what works, what the risks are, and what a concrete
improvement path looks like.

---

## 1. Runtime and Security Model

### What works

The security model is the strongest part of the project. Dropping all Linux capabilities and
selectively re-adding only `CHOWN`, `SETUID`, `SETGID`, and `DAC_OVERRIDE` is the correct
approach — most container security failures come from leaving capabilities intact that are never
needed. The `--security-opt=no-new-privileges` flag closes the `sudo`/`setuid` escalation path.
The entrypoint's host UID/GID remapping via `usermod`/`gosu` is the right pattern for avoiding
permission mismatches without running as root. The `--no-internet` and `--strict-network` flags
show awareness of SSRF and metadata endpoint risks.

### Issues and risks

**1.1 No seccomp or AppArmor profile**

`--cap-drop=ALL` with selective re-adds protects against capability abuse, but syscall-level
restrictions are absent. An agent that calls `ptrace`, `perf_event_open`, `keyctl`, or any number
of kernel interfaces that require no special capability can still do unexpected things inside the
container. Docker's default seccomp profile blocks ~40 syscalls; a custom profile tuned to what
editors actually need (mostly file I/O, network, process management) would meaningfully reduce the
attack surface.

```bash
# Recommended addition to run_container()
--security-opt seccomp=/etc/abox/seccomp.json
```

A seccomp profile for the `claude` editor, for instance, has no legitimate reason to permit
`init_module`, `kexec_load`, `mount`, `pivot_root`, or any of the kernel-facing syscalls.

**1.2 The four re-added capabilities are broader than necessary**

`DAC_OVERRIDE` allows bypassing file permission checks entirely. This is added to handle volume
ownership — but the ownership fix is done in a separate bootstrap container that runs as root
anyway. The main editor container should not need `DAC_OVERRIDE`. Remove it from the primary
`run_container()` call and keep it only in the `init_volume_ownership()` helper. Similarly,
`SETUID` and `SETGID` are present to allow `gosu` in the entrypoint to drop privileges. Once
that drop has happened, those capabilities are no longer exercisable by the unprivileged `agent`
user — but there is no mechanism to drop them post-entrypoint. Ambient capabilities would be a
cleaner model here.

**1.3 The `--strict-network` mode has limited effect**

Blocking `169.254.169.254`, `metadata`, and `metadata.google.internal` via `--add-host` is a
good start, but it's incomplete. It doesn't cover AWS IMDSv2 (`169.254.170.2`), Azure IMDS
(`169.254.169.254` on a different path, but also `168.63.129.16`), or any provider-specific
endpoints that aren't hostname-based. More importantly, `--add-host` only controls DNS; an agent
that resolves and caches an IP before the container starts, or uses the raw IP, bypasses it.

A proper strict network mode should use a user-defined Docker network with explicit egress rules,
or route through a local forward proxy that enforces an allowlist. The current implementation
gives a false sense of protection for cloud-hosted developers.

**1.4 SSH keys mounted read-only, but still present**

`container.sh` mounts `~/.ssh` as `-v $HOME/.ssh:/home/agent/.ssh:ro,z`. This is done to allow
`git` operations over SSH inside the container. The problem is that the entire `.ssh` directory
is mounted — private keys, known_hosts, config, agent socket references. A compromised or
misbehaving agent can read any key in that directory. The intended use case (git over SSH)
requires only a specific key. The mount should be scoped to the minimum: ideally, only the
key associated with the user's git remote, or better, an SSH agent socket forwarded via
`SSH_AUTH_SOCK` so the private key material never enters the container at all.

```bash
# Current - exposes all key material
-v $HOME/.ssh:/home/agent/.ssh:ro,z

# Better - forward the agent socket only
-e SSH_AUTH_SOCK=/tmp/ssh-agent.sock
-v $SSH_AUTH_SOCK:/tmp/ssh-agent.sock:ro
```

**1.5 No resource limit on the workspace volume**

Memory is capped at 4GB and CPUs at 2, but the workspace volume has no size limit. An agent
can write arbitrarily large files into `abox-workspace-<id>`, which is backed by host disk.
Docker volume drivers support `--opt o=size=10g` for tmpfs, but named volumes on `local` driver
don't support size limits by default. Either switch to a tmpfs-backed volume for the workspace
with a configurable cap, or add a disk quota via a cgroup v2 `io.max` rule.

**1.6 Container naming is predictable**

Container names follow the pattern `abox-<editor>-<basename>-<timestamp>`. The basename comes
from the workspace directory name, which is user-controlled. A malicious workspace directory
named `; rm -rf /` won't execute because the name is passed as a string — but a name containing
spaces or Docker-special characters can cause `--name` to fail silently or behave unexpectedly.
The timestamp (Unix seconds) in the name also means two rapid invocations in the same second
from the same directory will collide.

---

## 2. The Sync Strategy

This is the most architecturally fragile part of the system and the area with the most concrete
failure modes under real-world conditions.

### What works

Using ephemeral volumes as an airlock is the right design. The agent works against a copy of
the data, not the live host directory. The bidirectional sync (in before run, out after) is
clean in principle.

### Issues and risks

**2.1 Sync is not atomic — partial writes are invisible**

`sync_to_vols` runs a series of `cp -r` operations inside a container. If the container is
killed mid-sync (OOM, signal, Docker daemon restart), the volume is left in a partially-written
state. There is no checksumming, no manifest, and no detection of partial state. The next time
`sync_from_vols` runs, it will happily tar back whatever is in the volume — potentially a mix
of old and new data — to the host. For config directories that store auth tokens or session
state, this can corrupt credentials silently.

The fix is transactional staging: write to a temp directory inside the volume, then atomically
rename (which is guaranteed atomic by the kernel for same-filesystem moves):

```bash
# Instead of cp directly into /vol/config:
cp -r /host/config/. /vol/config.tmp/
mv /vol/config.tmp /vol/config  # atomic on same fs
```

**2.2 `sync_from_vols` uses `tar | tar` — no conflict detection**

When syncing back, the tool simply tars everything from the volume and unpacks it over the host
directory. If the host files were modified during the session (by another process, another
editor, or a parallel `abx` session), those changes are silently overwritten. There is no
`mtime`-based merge, no last-write-wins detection, and no warning. For configuration files like
`~/.config/opencode/config.json`, this is a real data loss scenario.

At minimum, before `sync_from_vols` runs, the tool should compare mtimes of host files against
the pre-session snapshot and warn on conflicts. Ideally it would offer a three-way merge
strategy or at minimum preserve both versions.

**2.3 The sync container is a cold-pull dependency**

`sync_to_vols` and `sync_from_vols` both spin up a container using `$IMAGE_NAME` to run shell
commands. This means:

- If the image isn't cached locally, sync operations pull the full editor image just to run
  `cp` and `tar`. A `debian:bookworm-slim` base would be far lighter.
- In `--offline` mode, sync still requires the image to be present locally. If the image was
  never pulled, sync fails with an opaque error.
- Two extra container startups per `abx` session add latency. Measured against a 1.5s startup
  target (from UX tests), these pre- and post-sync containers can easily add 2–4 seconds on
  cold cache.

The sync operations should use a minimal utility image (Alpine or distroless with just `tar`
and `sh`) rather than the full editor image. Even better, the sync could be done on the host
directly using `rsync` or pure Bash `cp`, since the volumes are accessible via
`docker cp` or host-path inspection, removing the extra container round-trips entirely.

**2.4 `sync_workspace` creates a tar on the host filesystem at `/tmp/`**

```bash
local temp_tar="/tmp/abx_sync_$$.tar"
```

This writes the entire workspace into `/tmp` before streaming to the container. For a large
project (say, 2GB of compiled artifacts), this means 2GB written to `/tmp`, then 2GB streamed
again. That's a 4GB I/O amplification before the container even starts. Since `.abxignore`
exists to exclude large dirs like `node_modules`, a well-configured user won't hit this — but
it's a footgun that should be addressed structurally. Streaming directly via pipe (`find | tar
-c | docker run -i ... tar -x`) avoids the intermediate file entirely, which is what the
no-exclusion path already does correctly.

**2.5 Hardcoded security exclusions can be bypassed by symlink**

```bash
patterns+=(".ssh" ".aws" ".env" ".gnupg" "**/*key" "**/*.pem")
```

The `is_excluded()` function operates on paths as strings returned by `find`. A symlink named
`.env` pointing to `/etc/shadow` would be synced if the target doesn't match an exclusion
pattern. The `find` invocation uses `-type f`, which follows symlinks and reports the symlink
as its target's type. Exclusion checks should be applied to both the path and the realpath of
any symlink found during workspace traversal.

---

## 3. Shell Architecture

### What works

Splitting logic into `helpers.sh`, `container.sh`, `sync.sh`, `exclusion.sh`, and `audit.sh`
is a good first step toward modularity. The Makefile's `bundle` target (concatenating the
modules into a single deployable script) is a clever solution to the single-binary distribution
requirement.

### Issues and risks

**3.1 The bundle is a liability, not an asset**

The build process concatenates shell files:

```makefile
@cat src/helpers.sh >> bin/abx
@cat src/exclusion.sh >> bin/abx
...
@tail -n +9 src/main.sh | grep -v '^source ' | ...
```

This `grep -v` pattern strips `source` statements to avoid double-sourcing, but it's fragile.
Any line containing the word `source` in a comment or string literal will be stripped. More
structurally, the bundle is a snapshot — it encodes the exact source state at build time, which
means the CI must run `make bundle` and publish a release on every meaningful change. There is
no hash, no content-addressed identity, and no way for a deployed `abx` to self-verify its
integrity.

Modern shell tool distribution uses a content-addressed approach: the release artifact includes
a checksum file, and the installer verifies it. The current installer downloads and installs
with no verification whatsoever. A MITM on the CDN or a GitHub release compromise would deliver
a malicious script with no detection.

```bash
# The install script currently does:
curl -fsSL "$RELEASE_URL" -o "$BINARY_NAME"
chmod +x "$BINARY_NAME"

# Should instead:
curl -fsSL "$RELEASE_URL" -o "$BINARY_NAME"
curl -fsSL "$RELEASE_URL.sha256" -o "$BINARY_NAME.sha256"
sha256sum --check "$BINARY_NAME.sha256"
```

**3.2 `set -e` is absent from `main.sh` (intentionally or not)**

`main.sh` uses `set -o pipefail` but not `set -e`. Several intermediate functions use local
variables assigned by command substitution — if those fail silently, the script continues with
empty strings. For example:

```bash
IFS='|' read -r IMAGE_NAME COMMAND_NAME CONFIG_REL_PATH ... <<< "$(get_editor_info "$EDITOR_NAME")"
```

If `get_editor_info` returns nothing (unrecognized editor), all variables are set to empty
strings. The container run then invokes `docker run ... "" ""` which fails with a Docker error
message rather than a useful user-facing error. There should be an explicit validation step
after `get_editor_info` returns.

**3.3 Global variable pollution**

`init_volumes()` exports `CONFIG_VOL`, `CACHE_VOL`, `STATE_VOL`, `SHARE_VOL`, `WORKSPACE_VOL`,
and `VOL_ID` as shell globals via `export`. `run_container()` references `CLI_NO_INTERNET`,
`CLI_STRICT_NETWORK`, `INTERACTIVE_FLAGS`, `PULL_POLICY`, `ENV_FLAGS`, `WORKSPACE_MOUNT`,
`TARGET_DIR`, `EDITOR_NAME`, and `TIMESTAMP` — all globals. This means the modules are not
truly independent; they share an implicit global state contract that is undocumented and
breakable. Adding a new module that shadows any of these names will cause silent bugs.

A minimal improvement is to define a single associative array (`declare -A ABX_STATE`) and pass
it by reference, or to at minimum document the expected global contract in a dedicated
`state.sh` module.

**3.4 The bundling approach precludes runtime updates**

Because `abx` is a single static script, updating it requires re-downloading the entire bundle.
There is no partial update, no hot-swap of a module, and no version pinning of the runtime
separate from the editors. If a security fix is needed in `exclusion.sh`, every user must
re-run the installer. A plugin/module discovery pattern (sourcing from `~/.config/abx/modules/`)
would allow targeted updates and user extensibility without changing the core bundle.

**3.5 No structured logging**

All output is `echo` to stdout. Errors go to `>&2` via `log_error()`. There is no log level,
no timestamps on output lines, no machine-readable output, and no log file. When `abx` fails
mid-session, diagnosing what happened requires reading the terminal output — which is gone if
the user wasn't watching. A `--verbose` flag that enables trace logging to `~/.local/state/abx/abx.log`
would make bug reports and self-diagnosis significantly easier.

---

## 4. Image and Build Pipeline

### What works

The single `Dockerfile` with a build-arg `INSTALL_CMD` is elegant — one template covers all
editors. The GitHub Actions matrix that parallelizes per-editor builds is correct. The
`sync-editors.yml` cron job that auto-bumps versions and triggers rebuilds is a nice piece
of operational automation that keeps images fresh without manual intervention.

### Issues and risks

**4.1 The image is large and rebuilds entirely for each version bump**

The Dockerfile installs the entire base layer (curl, git, python3, nodejs, npm, uv, bun, GitHub
CLI, Homebrew) before the editor-specific `INSTALL_CMD`. Because Homebrew is installed as a
layer before the editor, any Homebrew update or change to the base setup invalidates all
subsequent layers, forcing a full rebuild even if only the editor package version changed.

Layer ordering should be restructured so that the frequently-changing layer (editor install) is
the last meaningful layer, and the stable base (system packages, Homebrew) is cached aggressively:

```dockerfile
# Stable layers (rarely change)
FROM debian:bookworm-slim
RUN apt-get install -y ... # system deps
RUN curl ... homebrew ...  # homebrew

# Volatile layer (changes every version bump)
ARG INSTALL_CMD
RUN eval "$INSTALL_CMD"
```

This is approximately how it's structured now, but Homebrew installation is not stable — it
fetches the latest Homebrew itself every build, which can change the layer hash even if the
editor version didn't change. Pinning the Homebrew installation to a specific commit hash or
using a pre-built Homebrew base image would restore layer cache effectiveness.

**4.2 Multi-arch builds but no attestation**

The publish workflow builds `linux/amd64` and `linux/arm64` images, which is correct for 2026
where ARM is mainstream (Apple Silicon, AWS Graviton, etc.). However, there are no SBOM
(Software Bill of Materials) attestations, no image signing (Cosign/Sigstore), and no
provenance attestation. This means users have no way to verify that the image they pull from
`ghcr.io` was built from the public source code and not tampered with in the registry. For a
security-focused tool, this is a meaningful gap.

```yaml
# Add to publish step in publish.yml:
- name: Generate SBOM
  uses: anchore/sbom-action@v0
  with:
    image: ${{ steps.meta.outputs.tags }}

- name: Sign image
  uses: sigstore/cosign-installer@v3
  run: cosign sign --yes ${{ steps.meta.outputs.tags }}
```

**4.3 No image vulnerability scanning in CI**

The pipeline builds and pushes images but runs no vulnerability scanner (Trivy, Grype, Snyk).
A `debian:bookworm-slim` base with Python, Node, npm, Bun, and a full Homebrew installation
is a large attack surface. High-severity CVEs in any of those layers will go undetected until
a user reports unusual behavior or an external scanner runs against the GHCR registry.

```yaml
- name: Scan image
  uses: aquasecurity/trivy-action@master
  with:
    image-ref: ${{ steps.meta.outputs.tags }}
    severity: HIGH,CRITICAL
    exit-code: '1'   # fail the build on high/critical
```

**4.4 `sync-editors.yml` runs every 15 minutes — this is excessive**

The version sync cron runs every 15 minutes, 24/7. That's 96 workflow runs per day, 672 per week,
per repository — most of which will find no changes and do nothing, but still consume GitHub
Actions minutes. npm, PyPI, and GitHub releases don't publish updates at that frequency; even
the most actively developed editors (Claude Code, Codex) release a few times per week at most.

A daily schedule (`cron: '0 6 * * *'`) would reduce workflow runs by 96x while missing no
meaningful update by more than 24 hours. If faster updates are genuinely needed, a
repository_dispatch webhook from the upstream package registry is far more efficient than polling.

**4.5 The version-bump commit uses `[skip ci]` but still triggers sync-editors**

```yaml
git commit -m "chore: update editor versions [skip ci]"
```

The `[skip ci]` annotation prevents the `bundle.yml` and `publish.yml` workflows from triggering
on the version-bump commit itself (correct). But `sync-editors.yml` is on a schedule, not a push
trigger, so it will re-run in 15 minutes regardless, find that versions are already up to date,
and do nothing. This is fine but means the automation loop has no backpressure — if
`publish.yml` fails, `sync-editors.yml` will keep trying to trigger it every 15 minutes without
any awareness of the failure.

---

## 5. Configuration and Editor Management

### What works

`editors.json` as a single source of truth for all editor metadata (version, install command,
command name) is clean. `sync_versions.py` querying npm/PyPI/GitHub to stay current is
pragmatic. The `get_editor_info()` function in `helpers.sh` returning a pipe-delimited string
that encodes everything the runtime needs about an editor is compact.

### Issues and risks

**5.1 `get_editor_info()` is hardcoded and out of sync with `editors.json`**

This is one of the most significant structural bugs in the codebase. `editors.json` is the
canonical editor registry and is kept up to date by automation. But `helpers.sh` contains a
second, completely separate, manually-maintained editor registry:

```bash
get_editor_info() {
    local editor=$1
    case "$editor" in
        aider)   echo "ghcr.io/r-dson/abox:aider|aider|.aider.conf.yml|OPENAI_API_KEY,ANTHROPIC_API_KEY|" ;;
        claude)  echo "ghcr.io/r-dson/abox:claude|claude|.claude|ANTHROPIC_API_KEY|" ;;
        ...
    esac
}
```

These two registries will drift. If a new editor is added to `editors.json`, it will be
built and pushed to GHCR — but `get_editor_info()` will fall through to the `opencode|*`
default case, silently running the wrong editor. If an editor's config path changes
(say `claude` moves from `~/.claude` to `~/.config/claude`), `editors.json` won't capture it
because it doesn't track that field — only `helpers.sh` does.

The fix is to consolidate everything into `editors.json`:

```json
{
  "claude": {
    "version": "2.1.150",
    "install_cmd": "npm install -g @anthropic-ai/claude-code@{version}",
    "cmd_name": "claude",
    "image_tag": "ghcr.io/r-dson/abox:claude",
    "config_path": ".claude",
    "env_vars": ["ANTHROPIC_API_KEY"],
    "legacy_path": ""
  }
}
```

And have `get_editor_info()` read from that file at runtime:

```bash
get_editor_info() {
    local editor="$1"
    local config="$SCRIPT_DIR/../config/editors.json"
    jq -r ".editors[\"$editor\"] | \"\(.image_tag)|\(.cmd_name)|\(.config_path)|\(.env_vars | join(\",\"))|\(.legacy_path)\"" "$config"
}
```

This eliminates the dual-registry problem entirely. The bundled `bin/abx` would need to
embed the JSON at bundle time (already done for the script itself).

**5.2 API key handling is static per editor**

The `env_vars` field in `get_editor_info()` lists which API keys to pass through. For Claude
it's `ANTHROPIC_API_KEY`, for Gemini it's `GOOGLE_API_KEY`. But this is a fixed list — a user
who wants to pass a `HTTP_PROXY`, `NO_PROXY`, or a custom `OPENAI_API_BASE` for a local model
has no mechanism to do so without modifying the source.

An `--env KEY=VALUE` flag (or reading from a `.abxenv` file in the workspace) would give users
control over additional environment variables without exposing the host environment wholesale.

**5.3 `~/.config/abx.conf` is a bespoke format**

```
EDITOR=claude
EXCLUDE_URL=https://example.com/patterns
```

This is parsed with `grep | cut`, which is error-prone (no quoting support, no comments, no
multi-value keys). As configuration grows (more persistent options, per-editor defaults), this
format will not scale. TOML or even a simple JSON file would be more robust and parseable
with `jq` — which is already a dependency.

---

## 6. Testing Surface

### What works

Having three distinct test categories — unit tests for exclusion logic, integration tests for
container behavior, and UX/timing verification — is a good structural choice. The exclusion
unit tests are particularly thorough given that the fnmatch implementation is custom Bash. The
integration tests use `ABOX_RUNTIME=echo` to inject a fake runtime and verify the exact
`docker run` command that would be generated, which is clever and fast.

### Issues and risks

**6.1 Integration tests require the bundled binary to exist**

```bash
ABX_BIN="$(dirname "$0")/../bin/abx"
```

The integration tests reference `bin/abx`, which is the Makefile-bundled output. If you've
checked out the repo fresh and haven't run `make bundle`, the tests fail immediately with a
missing file rather than a useful error. The test runner should either build the bundle
automatically or fall back to sourcing the modules directly.

**6.2 The `run_test` helper swallows stderr**

```bash
if eval "$test_cmd" > /dev/null 2>&1; then
```

All stderr from test commands is discarded. When a test fails, the only output is `FAIL` with
no context about what went wrong. On CI where you can't interactively re-run, this makes
failures very hard to diagnose. Capture stderr to a temp file and display it on failure.

**6.3 No tests for the sync logic**

`sync_to_vols`, `sync_from_vols`, `sync_workspace`, and `sync_workspace_back` are entirely
untested. These functions handle the most consequential data movement in the tool. There are
no tests that:

- Verify data reaches the volume correctly
- Verify excluded files don't appear in the workspace volume
- Verify sync-back doesn't overwrite host files outside the workspace
- Verify partial sync failure leaves the host in a consistent state

Given the atomicity issues described in Section 2, this testing gap is a real risk. The sync
logic is where a bug would cause data loss — and it's where there are zero tests.

**6.4 No property-based testing for the exclusion engine**

The exclusion unit tests use a fixed set of pattern/path pairs. The custom fnmatch
implementation has several code paths (brace expansion, globstar, absolute vs relative patterns,
case-insensitive matching) that interact in subtle ways. A property-based test framework (even
a simple Bash-level fuzzer) that generates random patterns and paths and verifies the result
matches Python's `fnmatch` or `pathlib.Path.match` would catch edge cases that hand-written
tests won't.

**6.5 UX tests measure but don't enforce**

The startup latency test marks the result as PASS even when the threshold is exceeded:

```bash
if [ $AVG_TIME -lt 1500 ]; then
    echo -e "  ${GREEN}PASS${NC}: ..."
else
    echo -e "  ${YELLOW}WARN${NC}: ..."
    ((PASS_COUNT++))  # Still count as pass
fi
```

Performance regressions will never be caught by CI because every run counts as a pass. Either
enforce the threshold and fail the job, or remove the timing test entirely. A warning that
never triggers a failure is noise.

---

## Summary of Recommendations

Ordered roughly by impact and urgency:

### Critical — address before wider adoption

| # | Area | Change |
|---|------|--------|
| C1 | Security | Add seccomp profile; remove `DAC_OVERRIDE` from editor container |
| C2 | Security | Replace `~/.ssh` mount with SSH agent socket forwarding |
| C3 | Sync | Add installer SHA-256 verification |
| C4 | Architecture | Consolidate dual editor registry into `editors.json` only |
| C5 | Sync | Fix intermediate tarfile path — stream directly via pipe |

### High — address in the next iteration

| # | Area | Change |
|---|------|--------|
| H1 | Security | Implement proper `--strict-network` with egress proxy or user-defined network rules |
| H2 | Sync | Add transactional staging for volume writes (temp dir + atomic rename) |
| H3 | Sync | Add conflict detection before `sync_from_vols` overwrites host files |
| H4 | Sync | Use a minimal sync image (Alpine) instead of the full editor image |
| H5 | Pipeline | Add image signing (Cosign) and SBOM generation to `publish.yml` |
| H6 | Pipeline | Add Trivy/Grype vulnerability scanning to `publish.yml` |
| H7 | Testing | Write sync logic tests (in/out, exclusions, atomicity) |

### Medium — quality and maintainability

| # | Area | Change |
|---|------|--------|
| M1 | Architecture | Replace `~/.config/abx.conf` with JSON, parsed by `jq` |
| M2 | Architecture | Add `--env KEY=VALUE` / `.abxenv` for user-controlled env pass-through |
| M3 | Shell | Add `--verbose` flag and structured log file at `~/.local/state/abx/abx.log` |
| M4 | Shell | Replace global variable contract with a documented `ABX_STATE` associative array |
| M5 | Pipeline | Reduce `sync-editors.yml` cron from every 15 min to daily |
| M6 | Testing | Enforce startup latency threshold as a CI failure condition |
| M7 | Testing | Add property-based / fuzzing coverage for the exclusion engine |

### Low — forward-looking

| # | Area | Change |
|---|------|--------|
| L1 | Security | Explore rootless container runtimes (Podman in rootless mode) as the default path |
| L2 | Security | Add cgroup v2 workspace disk quota |
| L3 | Architecture | Investigate replacing the Bash bundle with a compiled Go or Rust binary for integrity and performance |
| L4 | Pipeline | Add `repository_dispatch` webhooks from upstream registries to replace cron polling |

---

*Review based on commit history as of May 2026. All line number references reflect the `main` branch at time of analysis.*
