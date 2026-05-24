# AGENTS.md

This document contains strict instructions, tooling rules, and architectural standards for writing, refactoring, and maintaining Go code in this repository. Read and apply these directives on every task.

---

## 1. Directory & File Layout Standards

### Filename Conventions
*   **Case Style:** Use `snake_case.go` for all file names. Never use `camelCase.go` or uppercase letters.
*   **Separation of Concerns:** Within a single package, avoid massive monolith files. Split cohesive logic across separate files using predictable suffixes:
    *   `domain.go` or `<package_name>.go`: Domain models, central struct definitions, and core interfaces.
    *   `db.go` or `repository.go`: Database engines, drivers, queries, and repositories.
    *   `handler.go` or `http.go`: Transport-layer routers, middleware, and request/response serialization.
    *   `service.go`: Coordinator file for business rules and cross-cutting concerns.
    *   `client.go`: Outbound HTTP, gRPC, or external API client integrations.

### Internal vs. External Boundaries
*   **Default to Private:** All packages must reside inside the `/internal` directory unless they are explicitly designed to be imported as shared libraries by external Go modules.
*   **Encapsulate Implementation Details:** Keep constructors private (`newService`) if they are only initialized by a parent package. Only export types, structs, and functions that are part of the designated package interface contract.
*   **Strict Circular Dependency Prevention:** 
    *   Do not allow sibling packages inside `/internal` to import each other bi-directionally.
    *   If package `A` imports package `B`, and package `B` needs domain models from `A`, extract those domain models to a neutral package (e.g., `/internal/domain`) or use interfaces defined in the consumer package to break the compile-time coupling.

---

## 2. Dev Environment & Tooling Rules

### Tool Management
*   **Manage CLI Tools Natively:** Do not create dummy `tools.go` files or blank-import packages to track dev tools. 
    *   To add a tool (e.g., a linter or generator), run: `go get -tool <import-path>`.
    *   To execute any project tool, run: `go tool <tool-name> <args>`.
*   **Automated Modernization:** When taking over legacy code or upgrading the codebase, run `go fix ./...` first. The modernizer engine in Go 1.26 automatically upgrades deprecated constructs (such as converting `interface{}` to `any`, and refactoring older loops or error patterns).
*   **Exclude Unrelated Paths:** Use the `ignore` directive in `go.mod` to isolate large asset directories, legacy integrations, or scratch folders so they are skipped during package matching.

### Linting & Static Analysis

Before submitting any change, run:
```bash
go tool golangci-lint run ./...
```

The project uses golangci-lint (configured in `.golangci.yml`) with these linters enabled:

| Linter | Purpose | What it catches |
|--------|---------|----------------|
| `errcheck` | Unchecked error returns | Missing `if err != nil` on `os.WriteFile`, `io.Copy`, etc. |
| `gosimple` | Code simplification | Redundant returns, unnecessary blocks |
| `govet` | Go vet correctness | Printf arg mismatches, uncopyable lock values |
| `staticcheck` | Advanced static analysis | Deprecated API usage, common bugs |
| `unused` | Dead code detection | Unused functions, variables, types |
| `gofmt` | Formatting consistency | Files not matching `gofmt` output |
| `goimports` | Import management | Missing or unused imports, wrong grouping |
| `misspell` | Spelling in comments | Common English misspellings |
| `revive` | Style & design rules | Stuttering type names, unused parameters, exported docs |
| `exhaustive` | Switch completeness | Missing enum/case handling in type switches |
| `wrapcheck` | Error wrapping | Unwrapped errors from external packages |
| `noctx` | Context propagation | HTTP calls without `context.Context` |

#### Linter Rules Enforced as Coding Standards

**Error Wrapping (`wrapcheck`):** All errors returned from external packages must be wrapped with `fmt.Errorf` and `%w`:
```go
// Incorrect:
return d.client.ContainerStart(ctx, id, opts)

// Correct:
return fmt.Errorf("starting container: %w", d.client.ContainerStart(ctx, id, opts))
```
Caveat: `fmt.Errorf("msg: %w", nil)` does NOT return nil — it returns a non-nil error. Only wrap when the error is non-nil, or use a conditional:
```go
if err := doWork(); err != nil {
    return fmt.Errorf("work failed: %w", err)
}
return nil
```

**No Unused Parameters (`revive`):** If a callback or interface method parameter is unused, name it `_`:
```go
// Incorrect:
func (s *stub) OnCreate(name string, opts Options) error { return nil }

// Correct:
func (s *stub) OnCreate(_ string, _ Options) error { return nil }
```
Exception: If the parameter is used inside the function body, keep the name even if the function is a no-op stub.

**No Stuttering Type Names (`revive`):** If a type `Foo` in package `foo` would be referred to as `foo.Foo` externally, rename it to something more descriptive. For type aliases that re-export from another package, keep the name short:
```go
// Incorrect (container.ContainerSpec):
package container
type ContainerSpec = runtime.ContainerSpec

// Correct (container.Spec):
package container
type Spec = runtime.ContainerSpec
```

**Unchecked Errors (`errcheck`):** Every function that returns an error must have its return value checked. In tests, use helper functions:
```go
func mustWriteFile(t *testing.T, path string, data []byte) {
    t.Helper()
    if err := os.WriteFile(path, data, 0o644); err != nil {
        t.Fatal(err)
    }
}
```

---

## 3. Idiomatic Syntax & Coding Standards

*   **Instantiating Pointers to Literals:** Never write or call custom pointer helpers (e.g., `StringPtr("...")` or `IntPtr(10)`). Use the `new(expr)` built-in introduced in Go 1.26 to initialize pointers directly from literals or expressions.
    *   *Incorrect:* `u.Name = &nameVal` (where `nameVal` is a temp variable)
    *   *Correct:* `u.Name = new("John")` or `u.Age = new(30)`
*   **Type-Safe Error Unwrapping:** Never use `errors.As` with a manual pointer-to-pointer setup unless working with legacy Go (< 1.26) versions. Use the type-safe, generic `errors.AsType` helper.
    *   *Incorrect:*
        ```go
        var pathErr *fs.PathError
        if errors.As(err, &pathErr) { ... }
        ```
    *   *Correct:*
        ```go
        if pathErr, ok := errors.AsType[*fs.PathError](err); ok { ... }
        ```
*   **JSON Zero-Value Control:** Do not use pointers solely to avoid emitting zero values when encoding structs. Use the `omitzero` tag option introduced in Go 1.24 to omit empty non-pointer fields.
    *   *Correct:*
        ```go
        type Record struct {
            Created time.Time `json:"created,omitzero"`
        }
        ```
*   **Exported Constructors Return Interfaces:** When a constructor creates an unexported type, return the interface — not the concrete type. This prevents leaking implementation details through the API.
    *   *Incorrect:* `func NewDocker() (*dockerRuntime, error)`
    *   *Correct:* `func NewDocker() (ContainerRuntime, error)`

---

## 4. Concurrency Directives

*   **Poka-Yoke WaitGroups:** Do not write manual `wg.Add(1)` and deferred `wg.Done()` boilerplate for standard concurrent jobs. It is error-prone. Use the `WaitGroup.Go` method introduced in Go 1.25 to automatically manage goroutine lifetime counters.
    *   *Incorrect:*
        ```go
        wg.Add(1)
        go func() {
            defer wg.Done()
            doWork()
        }()
        ```
    *   *Correct:*
        ```go
        wg.Go(func() {
            doWork()
        })
        ```
*   **Structured Error Propagation:** When executing concurrent tasks that return errors or require cooperative cancellation, use `golang.org/x/sync/errgroup` with a context. Do not manage error collection via raw channels manually.
*   **No Raw `GOMAXPROCS` Tuning:** Do not import `uber-go/automaxprocs` or manually adjust CPU thread bounds for containerized deployments. The Go runtime is container-aware as of Go 1.25 and automatically scales thread pools based on cgroup quotas.

---

## 5. Test Location, Isolation, & Testing Architecture

### Test Colocation Rule
*   All test files must reside in the exact same directory as the package they are testing. 
*   **Do not** create global `/tests` or `/test` folders to aggregate unit tests. The matching test file for `file.go` must be `file_test.go` in the same folder.

### Package Naming for Tests: Black-Box by Default
To enforce loose coupling and prevent tests from breaks during internal refactoring, write **black-box tests** by default.
*   **Black-Box Testing:** Declare your test file under a `_test` suffix package name (e.g., in package `user`, define tests in `user_test.go` with `package user_test`). This forces you to test only the public-facing API of your package.
*   **White-Box Testing:** If, and only if, you must test unexported properties, algorithms, or private structures, create a dedicated `xxx_internal_test.go` file inside the same package block (e.g., `package user`). Use this sparingly.

### Test Fixtures with `testdata/`
*   Keep raw payloads, JSON schemas, mock responses, and database seed SQL queries in a subdirectory named `testdata/` inside the local package folder.
*   The Go toolchain ignores the `testdata/` directory during standard compilation.
*   Access files within `testdata/` using standard file reads or `//go:embed`:
    ```go
    //go:embed testdata/user_payload.json
    var rawPayload []byte
    ```

### Mocking Guidelines
*   **Do Not Export Mocks:** Never place mock structures or mock generator configurations in production code files. Mocks must live strictly inside `_test.go` files or a unified `mocks_test.go` file within the test package.
*   **Prefer Real Implementations Over Mocks:** 
    *   For external HTTP dependencies, use `net/http/httptest`.
    *   For relational database access, use lightweight, containerized, or local transient instances rather than mock SQL drivers.
*   **Avoid Mock-Heavy Architectures:** If you find yourself mocking 10 different interfaces to test a single function, the target package contains too many dependencies and must be split.

### Async and Concurrency Testing
*   **No Real-World Sleeps:** Do not use `time.Sleep()` in concurrent tests to coordinate async execution. This introduces flaky and slow CI pipelines.
*   **Virtual Time Isolation:** Wrap time-dependent concurrent code in `testing/synctest`. Inside this block, time is virtualized and instantly fast-forwards through timers without delays.
*   **Automated Context Lifetimes:** Always retrieve the active test context using `t.Context()` to pass downstream to database queries or API clients. This ensures the test teardown cleanly cancels lingering network tasks if the test times out.

---

## 6. Table-Driven Test Structure

All tests with branching conditional scenarios must use the table-driven test structure.

```go
package user_test

import (
    "testing"
)

func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
    }{
        {
            name:    "valid email structure",
            email:   "dev@company.com",
            wantErr: false,
        },
        {
            name:    "invalid email structure - no domain",
            email:   "dev@",
            wantErr: true,
        },
        {
            name:    "empty email field",
            email:   "",
            wantErr: true,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            // Retrieve contextual test execution parameters
            ctx := t.Context()

            err := validator.ValidateEmail(ctx, tc.email)
            if (err != nil) != tc.wantErr {
                t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tc.wantErr)
            }
        })
    }
}
```

---

## 7. Performance Benchmarking Rules

*   **Benchmark Loop Guarding:** When benchmarking algorithms, execute tests using `b.Loop()`. This avoids synthetic compiler loop optimization side effects.
    ```go
    func BenchmarkProcessRecord(b *testing.B) {
        // Run setup logic here...
        
        for b.Loop() {
            processRecord(record)
        }
    }
    ```
*   **Memory Allocations Allocation Target:** Always verify allocations per run. Run benchmarks with the `-benchmem` flag to trace memory overhead:
    ```bash
    go test -bench=. -benchmem ./...
    ```

