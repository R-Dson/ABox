# ABox (Agent Sandbox)

`abx` is a secure sandbox for running OpenCode agents in an isolated containerized environment, protecting your host system while giving the agent all the tools it needs.

## Features

- **Security First**: Runs in an isolated container with dropped capabilities and path validation.
- **Airlock Pattern**: Ephemeral volumes for auth keys ensure host files remain untouched.
- **Runtime Agnostic**: Automatically detects and uses Docker or Podman.
- **Easy Integration**: Seamlessly handles authentication keys and workspace mounting.
- **Cross-platform**: Works on Linux, macOS (via Docker Desktop), and Windows (via WSL2).
- **Auto-updates**: Automatically rebuilds with the latest OpenCode version via GitHub Actions.

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


You can also pass additional arguments to opencode:

```bash
abx --help                    # Show opencode help
abx /path/to/project --help   # Same thing
abx --other-opencode-flag     # Pass flags to opencode
```

## Configuration

`abx` looks for configuration in `~/.config/opencode` and keys in `~/.local/share/opencode`. These are safely mounted into the sandbox via an ephemeral airlock.

## Development

Run tests:

```bash
make test
```

## License

MIT
