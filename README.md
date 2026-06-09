# ABox (Agent Sandbox)

`abx` runs AI coding editors inside isolated containers so agents can work on a project without direct access to your host configuration, credentials, or broader filesystem.

## Features

- **Security First**: Runs containers with dropped capabilities (`--cap-drop=ALL`), `no-new-privileges`, seccomp, and no Docker socket mount.
- **Airlock Pattern**: Syncs editor data and workspace files through ephemeral volumes instead of giving the editor direct access to host config paths.
- **Content Exclusion**: Applies hardcoded secret patterns plus local `.abxignore` and optional remote exclusion patterns before workspace sync.
- **Fail Fast Defaults**: Invalid config, unsafe workspaces, failed snapshots, and malformed resource limits stop startup before container side effects.
- **Rootless-Friendly Execution**: Maps your host UID/GID into containers so synced files keep usable ownership.
- **Runtime Agnostic**: Automatically detects Docker or Podman, with `ABOX_RUNTIME=docker|podman` override support.
- **Static Go Binary**: Single binary with embedded editor registry and seccomp profile.
- **Cross-platform**: Linux and macOS, amd64 and arm64.

## Supported Editors

- **OpenCode** (`opencode`) — Default
- **Claude Code** (`claude`)
- **Aider** (`aider`)
- **GitHub Copilot** (`copilot`)
- **Goose** (`goose`)
- **Gemini** (`gemini`)
- **Codex** (`codex`)
- **Pi** (`pi`)
- **Mistral Vibe** (`vibe`)

## Prerequisites

ABox requires:

- Docker or Podman installed and running.
- Go 1.26 or newer for local development and validation.

| Platform | Architecture | Runtime | Rootless | Notes |
|----------|-------------|---------|----------|-------|
| Linux | amd64, arm64 | Docker, Podman | Yes | Primary target. Podman rootless is supported. |
| macOS | amd64 (Intel), arm64 (Apple Silicon) | Docker Desktop | N/A | UID/GID collisions with macOS `staff` group (GID 20) are handled gracefully. |

By default, ABox does **not** pull container images automatically. Pull/build the required images ahead of time, or set `pull_policy` to `missing` or `always` when you want ABox to pull images.

Container images are published to `ghcr.io/r-dson/abox` with signed SBOMs. Pin to versioned tags (`<editor>-<version>`) for reproducible builds; unversioned tags (`<editor>`) follow the latest release.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/R-Dson/ABox/main/install | bash
```

Or download a binary from the [latest release](https://github.com/R-Dson/ABox/releases/latest).

## Usage

Navigate to your project directory and run:

```bash
abx
```

Or specify a directory:

```bash
abx /path/to/project
```

### Command-Line Options

| Flag | Description |
|------|-------------|
| `--editor <name>` | Use the specified editor for this session |
| `--shell` | Drop into an interactive shell instead of launching the editor |
| `--force-it` | Force interactive TTY allocation |
| `--offline` | Do not pull images |
| `--strict-network` | Use an internal container network |
| `--no-internet` | Disable container networking and reject host network fetches such as `--exclude-url` |
| `--force-sync` | Overwrite host files even if modified during the session |
| `--ssh-agent` | Forward the host SSH agent socket into the container when it is a real Unix socket |
| `--trust-workspace-env` | Allow workspace `.abxenv` to request host environment variables for this run |
| `--exclude-url <url>` | Fetch additional exclusion patterns from an HTTPS remote URL |
| `--env KEY=VALUE` | Pass an environment variable to the container (repeatable) |
| `--verbose` | Enable debug logging to `~/.local/state/abx/abx.log` |
| `--json-logs` | Emit JSON structured logs to stderr |

### Switching Editors

```bash
# Temporary
abx --editor claude

# Permanent
abx config set-editor claude
```

### Available Editors

```bash
abx config list-editors
```

## Subcommands

| Command | Description |
|---------|-------------|
| `abx [flags] [directory]` | Run an editor in a secure sandbox |
| `abx audit [dir]` | Audit a workspace for security issues |
| `abx config set-editor <name>` | Set the default editor |
| `abx config list-editors` | List available editors |
| `abx version` | Print version information |
| `abx completion <shell>` | Generate shell completion |

## Security Defaults

ABox is secure-by-default and fails early when safety checks fail.

- Workspace paths are resolved through symlinks before validation and mounting.
- `$HOME` and `/` are rejected as workspaces.
- Host symlink overwrites during sync-out are rejected.
- Workspace symlinks are archived as symlinks instead of dereferencing target contents.
- SSH agent forwarding is disabled unless `--ssh-agent` or `forward_ssh_agent=true` is set.
- Image pull policy defaults to `never` to avoid executing mutable tags implicitly.
- `--no-internet` rejects `--exclude-url` and forces image pull policy to `never`.
- Invalid memory limits, CPU limits, pull policies, seccomp materialization, and mtime snapshots fail before session creation.

## Content Exclusion

Create a `.abxignore` file to exclude sensitive files or large directories from the sandbox using simple doublestar glob patterns:

```text
.env
secrets/*.json
node_modules/
```

`.abxignore` is not full gitignore syntax. A leading `/` is treated as part of the pattern, `!` negation is not supported, and backslash escapes follow doublestar behavior. Directory patterns ending in `/` match that directory name at any depth.

Hardcoded security patterns always apply, including `.ssh`, `.aws`, `.env`, `.gnupg`, `.netrc`, `.npmrc`, `*.pem`, `*.p12`, `*.pfx`, `*key`, and `*_key`.

You can also load additional exclusion patterns from a remote URL:

```bash
abx --exclude-url https://example.com/abxignore
```

Remote exclusion downloads are bounded by size and timeout. They cannot be used with `--no-internet`.

## Configuration

Config file: `~/.config/abx/config.json`

```json
{
  "editor": "opencode",
  "exclude_url": "",
  "pull_policy": "never",
  "memory_limit": "4g",
  "cpu_limit": 2.0,
  "strict_network": false,
  "no_internet": false,
  "forward_ssh_agent": false,
  "forward_git_config": false,
  "trust_workspace_env": false,
  "verbose": false,
  "json_logs": false
}
```

Environment variables with the `ABX_` prefix override config values:

```bash
ABX_EDITOR=claude abx
ABX_PULL_POLICY=missing abx
```

Supported overrides include `ABX_EDITOR`, `ABX_EXCLUDE_URL`, `ABX_NO_INTERNET`, `ABX_STRICT_NETWORK`, `ABX_PULL_POLICY`, `ABX_MEMORY_LIMIT`, `ABX_CPU_LIMIT`, `ABX_FORWARD_SSH_AGENT`, `ABX_FORWARD_GIT_CONFIG`, `ABX_TRUST_WORKSPACE_ENV`, `ABX_VERBOSE`, and `ABX_JSON_LOGS`.

`pull_policy` supports:

| Value | Behavior |
|-------|----------|
| `never` | Do not pull images automatically |
| `missing` | Pull only when an image is not present locally |
| `always` | Pull before each run |

### Environment Injection

Workspace `.abxenv` files are disabled by default. Use `--trust-workspace-env`, `trust_workspace_env=true`, or `ABX_TRUST_WORKSPACE_ENV=true` only for workspaces you trust.

When enabled, create `.abxenv` in the workspace to request selected host environment variables for the container:

```text
ANTHROPIC_API_KEY
OPENAI_API_KEY
```

ABox resolves values from the host environment. Values written directly in `.abxenv` are ignored; the file is an allowlist of host variable names. Dangerous runtime keys such as `PATH`, `HOME`, `USER`, `SHELL`, `PWD`, and display/session variables are blocked.

### Editor registry source of truth

The canonical editor registry is `config/editors.json`. The embedded Go registry at `internal/config/editors.json` and the legacy bundle copy at `bin/editors.json` must match it after JSON normalization. The Go test suite includes a drift check for these copies.

## Development

```bash
# Build Go binary
make go-build

# Run tests
make go-test

# Lint
make go-lint

# Install locally
make go-install
```

Legacy Bash/image workflows remain available for compatibility:

```bash
# Build Docker image for one editor
make build ABX_EDITOR=claude

# Run legacy Bash tests
make test
```

## Architecture

The Go rewrite lives at the repo root alongside the legacy Bash implementation. Key packages:

- `internal/cli` — Cobra CLI commands, config loading, validation, and session orchestration
- `internal/config` — Embedded editor registry and Viper-backed user config
- `internal/runtime` — Docker/Podman abstraction using the Moby SDK
- `internal/container` — Session volumes, network mode, spec building, TTY attach, resize, and signal forwarding
- `internal/sync` — Streaming tar sync-in/sync-out, file-config sync, conflict detection, and safe extraction
- `internal/exclusion` — Hardcoded, local, and remote exclusion patterns using doublestar
- `internal/audit` — Pre-flight workspace safety checks

## License

MIT
