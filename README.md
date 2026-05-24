# ABox (Agent Sandbox)

`abx` is a secure sandbox for running AI coding editors in an isolated containerized environment, protecting your host system while giving the agent all the tools it needs.

## Features

- **Security First**: Runs in an isolated container with dropped capabilities (`--cap-drop=ALL`), seccomp profiles, and no Docker socket access.
- **Airlock Pattern**: Ephemeral volumes ensure host files remain untouched — editors cannot directly modify your system configuration or secrets.
- **Content Exclusion**: `.abxignore` files and hardcoded security patterns prevent sensitive files (`.env`, `.ssh`, `*.pem`) from entering the sandbox.
- **Rootless Execution**: Maps your host UID/GID into the container, ensuring file permissions remain consistent.
- **Runtime Agnostic**: Automatically detects and uses Docker or Podman.
- **Static Go Binary**: Single binary with embedded configuration — no runtime dependencies beyond a container runtime.
- **Cross-platform**: Linux and macOS, amd64 and arm64.

## Supported Editors

- **OpenCode** (`opencode`) — Default
- **Claude Code** (`claude`)
- **Aider** (`aider`)
- **GitHub Copilot** (`copilot`)
- **Goose** (`goose`)
- **Gemini** (`gemini`)
- **Codex** (`codex`)
- **Mistral Vibe** (`vibe`)

## Prerequisites

ABox requires Docker or Podman to be installed and running.

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
| `--editor <name>` | Use specified editor for this session |
| `--shell` | Drop into an interactive shell |
| `--force-it` | Force interactive TTY allocation |
| `--offline` | Do not pull images |
| `--strict-network` | Block all external network access |
| `--no-internet` | Disable networking entirely |
| `--force-sync` | Overwrite host files even if modified during session |
| `--exclude-url <url>` | Fetch exclusion patterns from a remote URL |
| `--env KEY=VALUE` | Pass environment variable to container (repeatable) |
| `--verbose` | Enable debug logging |
| `--json-logs` | Emit JSON structured logs |

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
| `abx run [dir]` | Run an editor in a secure sandbox |
| `abx audit [dir]` | Audit workspace for security issues |
| `abx config set-editor <name>` | Set the default editor |
| `abx config list-editors` | List available editors |
| `abx version` | Print version information |
| `abx completion <shell>` | Generate shell completion |

## Content Exclusion

Create a `.abxignore` file to exclude sensitive files or large directories from the sandbox using glob patterns:

```text
.env
secrets/*.json
node_modules/
```

Hardcoded security patterns always apply (`.ssh`, `.aws`, `*.pem`, `*key`).

## Configuration

Config file: `~/.config/abx/config.json`

```json
{
  "editor": "opencode",
  "exclude_url": ""
}
```

Environment variables with `ABX_` prefix override config: `ABX_EDITOR=claude`.

## Development

```bash
# Build Go binary
make go-build

# Run tests
make go-test

# Lint
make go-lint

# Build Docker images (legacy Bash)
make build ABX_EDITOR=claude

# Run Bash tests
make test
```

## Architecture

The Go rewrite lives at the repo root alongside the legacy Bash implementation. Key packages:

- `internal/cli` — Cobra CLI commands
- `internal/config` — Editor registry (embedded JSON) + Viper config
- `internal/runtime` — Docker/Podman abstraction (Moby SDK)
- `internal/container` — Session management, spec builder, networking
- `internal/sync` — Streaming tar transfer (SyncIn/SyncOut), conflict detection
- `internal/exclusion` — Pattern matching (doublestar), filtered walk
- `internal/audit` — Pre-flight security checks

## License

MIT
