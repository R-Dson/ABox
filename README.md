# ABox (Agent Sandbox)

`abx` is a secure sandbox for running OpenCode agents in an isolated containerized environment, protecting your host system while giving the agent all the tools it needs.

## Features

- **Security First**: Runs in an isolated container with dropped capabilities and path validation.
- **Multi-Editor Support**: Choose between OpenCode, Claude Code, Aider, Copilot, Goose, and more.
- **Airlock Pattern**: Ephemeral volumes for auth keys ensure host files remain untouched.
- **Runtime Agnostic**: Automatically detects and uses Docker or Podman.
- **Easy Integration**: Seamlessly handles authentication keys and workspace mounting.
- **Cross-platform**: Works on Linux, macOS, and Windows (via WSL2).
- **Auto-updates**: Automated builds for all supported editors via GitHub Actions.

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

### Passing Arguments

You can pass additional arguments to the underlying editor:

```bash
abx --help
abx --other-flag
```

## Configuration

- **Global Config**: `~/.config/abx.conf` stores your default editor.
- **Editor Auth**: `abx` maps editor-specific config files or folders (like `~/.claude`, `~/.opencode` or `~/.aider.conf.yml`) safely into the sandbox.

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
