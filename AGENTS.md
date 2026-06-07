# AGENTS.md

Agent instructions for ABox. Treat this as the agent-facing companion to `README.md`: it contains the build, test, style, safety, and review rules needed to work in this repository.

Follow the most specific instruction that applies. Explicit user instructions override this file. If a future nested `AGENTS.md` exists closer to the edited file, follow that file for its subtree.

---

## 1. Project Snapshot

- **Project:** ABox (`abx`), a Go CLI that runs AI coding editors inside isolated Docker/Podman containers.
- **Module:** `github.com/r-dson/abox`.
- **Primary language:** Go.
- **Current Go version:** `go 1.26` in `go.mod`; GitHub Actions uses `actions/setup-go` with `go-version-file: go.mod`.
- **Workspace:** no `go.work` is currently present. If one is added, read it before editing.
- **Legacy code:** `src/` and `tests/` contain the older Bash implementation/tests and are intentionally excluded from Go package matching with `ignore` in `go.mod`.
- **Configuration/data:** editor registry and security profile data live under `config/`, `internal/config/`, and `config/seccomp/`.
- **Container assets:** Docker image definitions and entrypoints live under `docker/`.

---

## 2. Required Workflow

Before editing:

1. Read the relevant package, adjacent tests, `go.mod`, `go.work` if present, `.golangci.yml`, CI workflow(s), and nearby docs.
2. Check the module Go version and CI Go version before using version-gated features.
3. Keep changes scoped to the request; do not silently modernize, reformat, or refactor unrelated code.
4. Preserve public APIs unless a breaking change is explicitly requested.
5. Prefer existing package patterns over new architecture.
6. Add or update tests for behavior changes, bug fixes, concurrency changes, security changes, and public API changes.
7. If requirements are ambiguous, ask before choosing architecture, external contracts, security posture, or irreversible changes.

Before finishing, state:

- Validation commands run.
- Validation commands not run, with exact command and reason.
- Any rule intentionally not followed, with reason.

---

## 3. Common Commands

Use repository-defined commands when practical.

### Go validation

```bash
go mod verify
go test ./...
go tool golangci-lint run ./...
go tool govulncheck ./...
```

If `govulncheck` is not available, add it as a tracked tool first or state that it could not be run:

```bash
go get -tool golang.org/x/vuln/cmd/govulncheck
go mod tidy
```

### CI-equivalent checks

CI runs:

```bash
go mod verify
go tool golangci-lint run ./...
go test -race -coverprofile=coverage.out $(go list ./... | grep -v -E '(cmd/abx$|internal/runtime$)')
go build -trimpath -ldflags="-s -w" ./cmd/abx
./abx version
```

The CI coverage gate requires total coverage of at least 70% for the included packages. `cmd/abx` and `internal/runtime` are excluded because they require live runtime behavior or are main-only.

### Make targets

```bash
make go-build   # CGO_ENABLED=0 go build -trimpath with version ldflags, output ./abx
make go-test    # go test -count=1 ./...
make go-lint    # go tool golangci-lint run ./...
make go-cover   # go test -coverprofile=coverage.out ./...
```

Legacy/container targets exist and may require Docker or Podman:

```bash
make build      # build editor container image
make test       # run legacy Bash/container test suite after image build
```

### Extra checks

For race-sensitive changes:

```bash
go test -race ./...
```

For performance-sensitive changes:

```bash
go test -bench=. -benchmem ./...
```

For release-impacting changes, also inspect `.goreleaser.yml` and `.github/workflows/release.yml`.

---

## 4. Tools and Dependencies

### Go tools

This repository uses Go tool tracking. Do not use `tools.go` blank imports.

Add tools with:

```bash
go get -tool <import-path>
```

Run tools with:

```bash
go tool <tool-name> <args>
```

Tracked/expected tools:

```bash
go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint
go get -tool golang.org/x/vuln/cmd/govulncheck
```

Note: `go.mod` currently tracks `github.com/golangci/golangci-lint/cmd/golangci-lint`. Do not switch to a `/v2` tool path unless the dependency/configuration is intentionally upgraded.

### Modules

- Run `go mod tidy` after dependency or import changes.
- Prefer the standard library when adequate.
- Do not add dependencies for trivial helpers.
- Use `replace` only for local development, emergency forks, or documented overrides.
- Use `ignore` in `go.mod` for unrelated trees that must not match under `./...`.

---

## 5. Go Version Gates

Do not introduce syntax or APIs unsupported by `go.mod`, CI, Dockerfiles, or deployment tooling.

### Go 1.26+

Use only when the module and CI support Go 1.26+:

- Use `new(expr)` instead of pointer helper functions for literal/computed values.
- Use `errors.AsType[T](err)` for typed error extraction when applicable.
- Use `go fix ./...` only for intentional modernization work.
- When taking over legacy Go code or doing an explicit upgrade, run `go fix ./...` before manual rewrites so the standard tool handles mechanical modernization first.

### Go 1.25+

Use only when the module and CI support Go 1.25+:

- Use `sync.WaitGroup.Go` for fire-and-wait goroutines that do not return errors.
- Use `testing/synctest` for deterministic time/concurrency tests.
- Do not add `go.uber.org/automaxprocs` or tune `runtime.GOMAXPROCS` for ordinary container deployments.

### Go 1.24+

Use only when the module and CI support Go 1.24+:

- Track tools with `go get -tool`.
- Run tools with `go tool`.
- Use `t.Context()` and `b.Context()` in tests/benchmarks.
- Use `b.Loop()` in benchmarks.
- Use JSON `omitzero` when zero-value omission is required.

---

## 6. Static Analysis and Formatting

Required linters are configured in `.golangci.yml`. Do not bypass failures without documenting the reason.

Expected checks:

- `errcheck`: check returned errors.
- `gosimple`: simplify idioms.
- `govet`: catch correctness issues.
- `staticcheck`: catch bugs and deprecated APIs.
- `unused`: remove dead code.
- `gofmt`: format Go files.
- `goimports`: fix imports.
- `misspell`: fix spelling.
- `revive`: enforce naming/style docs.
- `exhaustive`: handle enum-like switches.
- `wrapcheck`: wrap external/lower-layer errors.
- `noctx`: require context-aware calls.

Use `gofmt`/`goimports` on touched Go files only unless a broader formatting pass is explicitly requested.

---

## 7. Repository Layout and Package Boundaries

### Directories

- Put application implementation under `/internal` by default.
- Put command entry points under `/cmd/<name>`.
- Put public libraries outside `/internal` only when intended for external import.
- Do not create global `/tests` or `/test` directories for Go unit tests.
- Keep legacy Bash files under `src/` and shell tests under `tests/` unless intentionally working on the legacy/container flow.

Suggested Go layout:

```text
/cmd/<binary>/main.go
/internal/<domain>/...
/internal/<feature>/...
/internal/platform/...
/internal/testutil/...
```

### Files

Use lower snake case:

```text
user_service.go
user_repository.go
http_handler.go
```

Avoid vague files: `misc.go`, `helpers.go`, `utils.go`.

Split files by responsibility:

- `domain.go` or `<package>.go`: core types/interfaces.
- `repository.go` or `db.go`: persistence.
- `service.go`: orchestration/business rules.
- `handler.go` or `http.go`: transport.
- `client.go`: outbound clients.
- `config.go`: configuration.
- `errors.go`: errors.
- `*_test.go`: tests.

### Dependencies

- No circular dependencies.
- Higher-level packages may import lower-level packages.
- Lower-level packages must not import higher-level packages.
- Define small consumer-side interfaces to break dependency cycles.
- Extract shared domain types only when multiple packages need the same type.
- Avoid dumping-ground packages: `common`, `shared`, `utils`.

### Constructors, exports, and interfaces

- Keep constructors private (`newService`) when they are only initialized by a parent package.
- Export only types, structs, and functions that are part of the intended package contract.
- Return concrete types by default.
- Return an interface when the concrete implementation is intentionally hidden and callers need only that interface.
- If an exported constructor creates an unexported implementation type, prefer returning the small interface rather than leaking implementation details.
- Define interfaces at the consumer side when possible.
- Keep interfaces small.
- Do not create interfaces only for mocks unless the package boundary benefits.

---

## 8. Idiomatic Go Rules

### Errors

Wrap lower-layer or external errors with operation context and `%w`:

```go
if err := client.Start(ctx, id); err != nil {
    return fmt.Errorf("start container %q: %w", id, err)
}
```

Never call `fmt.Errorf("...: %w", err)` unless `err != nil`.

Use `errors.Is` for sentinels.

Use `errors.AsType[T]` on Go 1.26+ for typed errors instead of manual pointer-to-pointer `errors.As` setup:

```go
if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
    // handle pathErr
}
```

### Names

- Use `_` for intentionally unused callback/interface parameters.
- If a parameter is used inside the function body, keep its name even if the function is otherwise a no-op stub.
- Avoid package/type stutter: prefer `container.Spec` over `container.ContainerSpec`.
- Export only identifiers required across package boundaries.
- Add doc comments for exported identifiers.

### Pointers

On Go 1.26+, use:

```go
name := new("Jane")
age := new(42)
createdAt := new(time.Now().UTC())
```

Do not add `StringPtr`, `IntPtr`, or similar helpers.

### JSON

Do not use pointers only to control JSON omission.

Use `omitzero` when supported:

```go
type Record struct {
    CreatedAt time.Time `json:"created_at,omitzero"`
}
```

Use pointers only when the domain distinguishes unset from explicit zero.

### General

- Use `any` in new code.
- Use generics only when they reduce duplication without reducing clarity.
- Preallocate slices/maps when size is known.
- Copy slices/maps when crossing API boundaries if mutation would leak state.
- Return errors for ordinary failures; do not panic.
- Use panics only for impossible programmer errors, init invariants, and tests.

---

## 9. Context, I/O, and Boundaries

### Context

Functions must accept `context.Context` as the first parameter when they perform I/O, blocking work, RPCs, database calls, subprocess execution, container runtime calls, filesystem sync work, or long-running work.

Rules:

- Do not store contexts in structs.
- Do not pass nil contexts.
- Do not use `context.Background()` inside request-scoped code.
- Use `t.Context()` in tests on Go 1.24+.
- Create timeouts/deadlines at operation boundaries.
- Always call cancel functions for derived contexts.

### HTTP clients

- Use `http.NewRequestWithContext`.
- Do not use package-level `http.Get`, `http.Post`, etc. in production.
- Configure client timeouts.
- Always close response bodies.
- Check status codes explicitly.

### HTTP servers

Configure timeouts:

```go
srv := &http.Server{
    Addr:              addr,
    Handler:           handler,
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       15 * time.Second,
    WriteTimeout:      30 * time.Second,
    IdleTimeout:       60 * time.Second,
}
```

Do not expose `net/http/pprof` publicly without auth and network controls.

### Filesystem and subprocesses

- Check file operation errors.
- Check close errors for writes when durability matters.
- For read-only close errors intentionally ignored, use `defer func() { _ = f.Close() }()`.
- Use `os.Root` for root-confined filesystem access when available.
- Use `exec.CommandContext` with explicit args.
- Do not shell-interpolate untrusted input.
- Normalize and validate paths.

---

## 10. Concurrency

### Primitive choice

Use `sync.WaitGroup.Go` for no-error fire-and-wait goroutines:

```go
var wg sync.WaitGroup
for _, job := range jobs {
    job := job
    wg.Go(func() {
        process(job)
    })
}
wg.Wait()
```

Functions passed to `WaitGroup.Go` must not panic.

Use `errgroup.WithContext` when goroutines can fail, need cancellation, or should stop together:

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(8)

for _, job := range jobs {
    job := job
    g.Go(func() error {
        if err := process(ctx, job); err != nil {
            return fmt.Errorf("process job %s: %w", job.ID, err)
        }
        return nil
    })
}

if err := g.Wait(); err != nil {
    return fmt.Errorf("process jobs: %w", err)
}
```

Use channels for ownership transfer, streaming, and coordination.

Use mutexes for shared in-memory state.

Do not do blocking I/O while holding a lock.

### Lifecycle

Every goroutine must have a clear exit path.

Rules:

- Pass context to blocking or long-running goroutines.
- Stop tickers and timers.
- Close channels from the sender side.
- Bound concurrency for unbounded inputs.
- Do not leak goroutines in services or tests.

### Atomics

Use typed atomics for simple counters/flags only when clearer than a mutex.

---

## 11. Testing

### Location and package style

Tests live next to the package under test:

```text
/internal/user/service.go
/internal/user/service_test.go
```

Prefer black-box tests:

```go
package user_test
```

Use same-package tests only for required internal coverage:

```go
package user
```

### Structure

- Use table-driven tests for multiple scenarios.
- Use `t.Helper()` in helpers.
- Check every returned error. In tests, wrap repeated setup in helpers that call `t.Helper()` and fail immediately with useful context.
- Failure messages must include got/want values.
- Prefer standard-library assertions unless the repo already uses an assertion library consistently.
- Use `t.Context()` on Go 1.24+ for code that accepts context.

### Fixtures

- Store fixtures under package-local `testdata/`.
- Use `//go:embed` or explicit file reads.

### Mocks/fakes

- Do not export mocks from production packages.
- Keep mocks in `_test.go` files or test-only packages.
- Prefer fakes or real lightweight implementations over heavy mocks.
- Use `net/http/httptest` for HTTP dependencies.
- Prefer real transient databases or repository-level integration tests over SQL driver mocks when practical.
- If a test needs many mocks, split the production package.

### Concurrency tests

Do not coordinate tests with real `time.Sleep`.

Prefer channels, `WaitGroup`, `errgroup`, or `testing/synctest`.

### Fuzzing

Use fuzz tests for parsers, decoders, validators, protocol handlers, and boundary-heavy code.

Store seed corpora under `testdata/fuzz/<FuzzName>/`.

---

## 12. Benchmarks and Performance

Use `b.Loop()` on Go 1.24+:

```go
func BenchmarkProcessRecord(b *testing.B) {
    record := makeRecord()
    b.ReportAllocs()

    for b.Loop() {
        processRecord(record)
    }
}
```

Rules:

- Measure before optimizing.
- Keep benchmarks realistic.
- Avoid clever code without benchmark/profile proof.
- Prefer simple allocation reductions: preallocation, streaming, fewer conversions.
- Do not use `unsafe` without benchmarks, tests, and documented reason.
- Use `pprof` for CPU, heap, mutex, and block profiling.

---

## 13. Security

ABox is security-sensitive because it isolates coding agents from host credentials, host configuration, and broad filesystem access. Treat changes to sync, exclusions, mounts, runtime execution, image pulling, SSH agent forwarding, networking, seccomp, and logging as high risk.

### Vulnerabilities

Run:

```bash
go tool govulncheck ./...
```

Reachable vulnerabilities block the change unless triaged and documented.

### Secrets

- Never commit secrets, tokens, private keys, or credentials.
- Do not log secrets, authorization headers, SSH agent details, environment secrets, or sensitive path contents.
- Redact sensitive fields in errors/logs.
- Load secrets from runtime environment or a secrets manager.

### Crypto

- Prefer standard-library crypto.
- Use `crypto/rand` for security-sensitive randomness.
- Do not implement custom crypto.
- Do not use deprecated or unauthenticated modes for new code.
- Document and test FIPS requirements when applicable.

### Input boundaries

- Validate external input at boundaries.
- Use parameterized SQL.
- Use `exec.CommandContext` with explicit args.
- Do not shell-interpolate untrusted input.
- Normalize and validate paths.
- Prefer `os.Root` for root-confined filesystem operations when available.

---

## 14. Database

If database code is added, use context-aware calls:

```go
db.QueryRowContext(ctx, query, id)
db.ExecContext(ctx, query, args...)
db.BeginTx(ctx, nil)
```

Do not use `Query`, `Exec`, or `Begin` in production when context-aware alternatives exist.

Use transactions for atomic multi-step changes.

Transaction rules:

- Roll back on error paths.
- Commit once.
- Preserve the original error when rollback also fails unless rollback failure is the primary failure.
- Keep transaction scopes small.

SQL rules:

- Use parameterized queries.
- Keep query construction explicit and tested.
- Document migration assumptions.

---

## 15. Logging and Observability

- Use structured logging.
- Log at system boundaries.
- Include stable IDs: request ID, resource ID, operation.
- Do not log secrets or excessive payloads.
- Avoid duplicate logging and returning of the same error unless context differs.
- Use bounded-cardinality metric labels.
- Libraries should accept loggers/hooks rather than use global loggers.

---

## 16. API Design

### Public APIs

For exported packages:

- Avoid breaking exported identifiers, signatures, struct fields, and behavior.
- Add APIs instead of changing existing ones when practical.
- Document deprecations with `Deprecated:` comments.

### Options

- Use options structs for stable data-like configuration.
- Use functional options only when they materially improve API stability or ergonomics.

---

## 17. Agent Reporting Rules

- Do not assume Go 1.26+ until `go.mod` and CI confirm it.
- Do not silently perform broad formatting, modernization, dependency updates, or architecture changes outside scope.
- Use repository-defined commands and tools when present.
- Prefer existing patterns over new architecture.
- Put highest-impact findings first.
- State validation commands run.
- State validation commands not run and why.
- State any rule intentionally not followed and why.
