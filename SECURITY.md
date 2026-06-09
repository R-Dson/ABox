# Security Policy

## Reporting Vulnerabilities

To report a security vulnerability in ABox, please open a private security advisory at:

https://github.com/R-Dson/ABox/security/advisories/new

Do not file public issues for security vulnerabilities.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Security Model

ABox isolates AI coding editors inside Docker or Podman containers to protect host credentials, configuration, and filesystem from unintended access.

### Container Isolation

- All editor processes run inside containers with dropped capabilities and seccomp profiles.
- Network access is disabled by default for helper/bootstrap containers.
- PIDs are limited to prevent fork bombs.
- Filesystem access is scoped to explicitly mounted volumes.

### Sync Boundary

- Sync-in copies host files into container volumes before the session.
- Sync-out reconciles container changes back to the host after the session.
- Host-created files during a session are preserved and never deleted by sync-out.
- Conflicting changes are detected and reported; the host file is preserved.

### Credential Protection

- `.abxenv` workspace files are disabled by default.
- Exclusion patterns protect common credential stores (`.aws/credentials`, `.ssh/id_*`, `.kube/config`, etc.).
- Git config forwarding strips host-side credential sections.
- SSH agent forwarding is opt-in.

### Supply Chain

- Go dependencies are verified with `go mod verify`.
- Container base images are pinned by digest.
- CI runs `govulncheck`, Trivy image scanning, and CodeQL analysis.
- SBOMs are generated and signed for all published container images.
- Third-party GitHub Actions are version-pinned.

## Audit

Run `abx audit <workspace>` to scan for sensitive files before starting a session.
