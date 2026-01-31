# ABox (Agent Sandbox)

ABox (Agent Sandbox) allows you to run OpenCode agents in a secure, isolated containerized environment, protecting your host system while giving the agent all the tools it needs.

## Features
- **Security First**: Runs in an isolated container with dropped capabilities and path validation.
- **Airlock Pattern**: Ephemeral volumes for auth keys ensure host files remain untouched.
- **Runtime Agnostic**: Automatically detects and uses Docker or Podman.
- **Easy Integration**: Seamlessly handles authentication keys and workspace mounting.
- **Cross-platform**: Works on Linux, macOS (via Docker Desktop), and Windows (via WSL2).

## Installation

You can install ABox with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/R-Dson/ABox/main/install | bash
```

## Usage

Navigate to your project directory and run:

```bash
abx
```

## Verification

To run the security audit and integration test suite:

```bash
make test
```

## Configuration

ABox looks for configuration in `~/.config/opencode` and keys in `~/.local/share/opencode`. These are safely mounted into the sandbox as needed via an ephemeral airlock.

## License
MIT
