# ABox (Agent Sandbox)

`abx` is a secure sandbox for running OpenCode agents in an isolated containerized environment, protecting your host system while giving the agent all the tools it needs.

## Features

- **Security First**: Runs in an isolated container with dropped capabilities (`--cap-drop=ALL`). No Docker socket access prevents host-level privilege escalation.
- **Airlock Pattern**: Ephemeral volumes ensure host files remain untouched - editors cannot directly modify your system configuration or secrets.
- **Rootless Execution**: Maps your host UID/GID into the container, ensuring file permissions remain consistent and the agent never runs as root.
- **Runtime Agnostic**: Automatically detects and uses Docker or Podman.
- **Easy Integration**: Seamlessly handles authentication keys and workspace mounting.
- **Cross-platform**: Works on Linux, macOS, and Windows (via WSL2).
- **Auto-updates**: Automated builds for all supported editors via GitHub Actions.

## Supported Editors

- **OpenCode** (`opencode`) - Default
- **Claude Code** (`claude`)
- **Aider** (`aider`)
- **GitHub Copilot** (`copilot`)
- **Goose** (`goose`)
- **Gemini** (`gemini`)
- **Codex** (`codex`)
- **Cursor** (`cursor`)
- **Mistral Vibe** (`vibe`)

## Prerequisites

ABox requires Docker or Podman to be installed and running.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/R-Dson/ABox/main/install | bash
```

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

- `--editor <name>` - Use specified editor for this session only (overrides default)
- `--default-editor <name>` - Set editor as the permanent default
- `--shell` - Enters into the container's bash shell instead of running the editor
- `--force-it` - Force interactive terminal mode (allocates pseudo-TTY)
- `--offline` - Disable remote image update checks (uses local image if available)
- `<directory>` - Specify workspace directory (default: current directory)
- `<args>...` - Additional arguments are passed directly to the editor

### Switching Editors

ABox supports multiple AI editors. Use the `--editor` flag to switch temporarily:

```bash
abx --editor claude
abx --editor codex
```

Or specify a permanent default editor:

```bash
abx --default-editor claude
```


## Configuration

- **Global Config**: `~/.config/abx.conf` stores your default editor.
- **Bidirectional Sync**: All editor-specific directories (config, cache, state, and share) are synchronized using ephemeral volumes. This creates a security barrier:
  - Config: `~/.config/*`, `~/.claude`, `~/.opencode`, etc.
  - Cache: `~/.cache/*`
  - State: `~/.local/state/*`
  - Share: `~/.local/share/*`
  - Data is synced from host before running and back to host after completion.
  - **Security Benefit**: Editors cannot directly modify host files, run commands outside the sandbox, or perform directory traversal attacks to access sensitive data like SSH keys.
- **Workspace**: Your current directory is **mounted** at `/workspace` inside the container with **read-write access**.

## Content Exclusion

Create a `.abxignore` file to exclude sensitive files or large directories from the sandbox using glob patterns. When detected, ABox switches to a safe sync mode, ensuring excluded content is never mounted into the container.

Example `.abxignore`:
```text
.env
secrets/*.json
node_modules/
```

You can also use `--exclude-url <URL>` (or `--default-exclude-url` to save permanently) to fetch exclusion patterns from a remote URL. URL and local patterns are merged automatically at runtime, with stricter patterns taking priority.

## Development

Build a specific editor image:

```bash
make build ABX_EDITOR=claude
```

Run tests:

```bash
make test
```

## License

MIT
