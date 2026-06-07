# CI and security validation notes

## Local validation commands

Run the standard local checks before committing Go changes:

```bash
go mod verify
go test ./...
go tool golangci-lint run ./...
```

For CI-equivalent race/coverage and build checks:

```bash
go test -race -coverprofile=coverage.out $(go list ./... | grep -v -E '(cmd/abx$|internal/runtime$)')
go build -trimpath -ldflags="-s -w" ./cmd/abx
./abx version
```

## govulncheck triage

`govulncheck` is tracked as a Go tool so dependency vulnerability checks are reproducible:

```bash
go tool govulncheck ./...
```

As of 2026-06-07, `go tool govulncheck ./...` reports two findings in `github.com/docker/docker@v28.5.2+incompatible`:

- `GO-2026-4887` — Moby AuthZ plugin bypass with oversized request bodies.
- `GO-2026-4883` — Moby plugin privilege validation off-by-one.

Current triage:

- The Go vulnerability database reports `Fixed in: N/A` for both advisories.
- `github.com/docker/docker v28.5.2+incompatible` is the newest tagged module version currently available to this project.
- ABox uses the Docker SDK to control a local Docker-compatible runtime; there is no client dependency upgrade available that clears these findings today.
- Treat these as inherited Docker/Moby runtime risk until upstream publishes a fixed SDK/daemon release.

Operational mitigation until an upstream fix exists:

- Keep Docker/Podman daemons local or otherwise tightly access-controlled.
- Do not expose the Docker API socket over unauthenticated networks.
- Keep host Docker/Podman runtimes patched independently of this Go module.
- Re-run `go tool govulncheck ./...` when Docker SDK releases a newer tag or the Go vulnerability database adds fixed versions.
