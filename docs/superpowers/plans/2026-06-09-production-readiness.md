# Production-readiness fixes implementation plan

> For agentic workers: implement this plan task by task. Complete one task, verify it, then move to the next.

**Goal:** Resolve the three production blockers preventing the ABox Go rewrite from merging: sync-out root-ownership errors on dirty editor data, noisy init-process seccomp failures, and Podman rootless sync-out timeouts.
**Architecture:** Each fix is isolated to one subsystem (sync extraction, seccomp profile, runtime wait). No cross-cutting changes. All fixes preserve existing test suites and CI gates.
**Tech stack:** Go 1.26, Docker SDK, seccomp JSON, `internal/sync`, `internal/runtime`, `config/seccomp`.

---

## File map

### Create

- `internal/sync/transfer_test.go` — add `TestExtractTar_ChownsExtractedFilesToHostUID`, `TestExtractTar_ChownFailureIsWarning`, `TestOut_ChownsExtractedFiles` test cases (appended to existing file)

### Modify

- `internal/sync/transfer.go` — add host-uid/gid chown pass after tar extraction in `extractTar`
- `config/seccomp/abox-default.json` — add `pidfd_open`, `pidfd_send_signal`, `rt_sigqueueinfo`, `rt_tgsigqueueinfo` syscalls
- `internal/runtime/docker.go` — increase ContainerWait polling interval for Podman runtime

### Read for context

- `internal/sync/transfer.go` — `extractTar`, `out`, `OutWithOptions`, `mountVolumeContainer` functions
- `internal/container/manager.go` — `bootstrapOwnership` chown command
- `docker/entrypoint.sh` — gosu user drop, UID/GID setup
- `config/seccomp/abox-default.json` — current allowed syscall list
- `internal/runtime/docker.go` — `waitForContainerStatus`, `waitForExecResult`

---

## Baseline verification

- [ ] Run existing sync, runtime, and seccomp tests

  Command:

  ```bash
  go test ./internal/sync ./internal/runtime ./internal/config ./internal/container -count=1
  go tool golangci-lint run ./...
  ```

  Expected result:

  ```text
  ok  github.com/r-dson/abox/internal/sync
  ok  github.com/r-dson/abox/internal/runtime
  ok  github.com/r-dson/abox/internal/config
  ok  github.com/r-dson/abox/internal/container
  (lint: clean)
  ```

---

## Tasks

### Task 1: Add missing seccomp syscalls for init processes

**Files:** `config/seccomp/abox-default.json`

- [ ] Add syscalls

  Add `"pidfd_open"`, `"pidfd_send_signal"`, `"rt_sigqueueinfo"`, `"rt_tgsigqueueinfo"` to the allowed syscall names list in `config/seccomp/abox-default.json`, in alphabetical order within the existing list.

- [ ] Verify

  ```bash
  go test ./internal/config ./internal/container -count=1
  python3 -c "import json; json.load(open('config/seccomp/abox-default.json'))"
  ```

  Expected result:

  ```text
  ok  (all packages pass)
  (json parses cleanly)
  ```

- [ ] Commit

  ```bash
  git add config/seccomp/abox-default.json
  git commit -m "fix(seccomp): allow pidfd and sigqueue syscalls for init"
  ```

---

### Task 2: Chown extracted files to host uid/gid in sync-out

**Files:** `internal/sync/transfer.go`, `internal/sync/transfer_test.go`

- [ ] Write failing test

  Add to `internal/sync/transfer_test.go`:

  ```go
  func TestExtractTar_ChownsExtractedFilesToHostUID(t *testing.T) {
      dest := t.TempDir()
      uid, gid := os.Getuid(), os.Getgid()

      var buf bytes.Buffer
      tw := tar.NewWriter(&buf)
      // Write a file owned by root (uid 0) — simulates container-created content.
      _ = tw.WriteHeader(&tar.Header{
          Name:     "rootfile.txt",
          Mode:     0o644,
          Size:     4,
          Uid:      0,
          Gid:      0,
          Typeflag: tar.TypeReg,
          Format:   tar.FormatPAX,
      })
      _, _ = tw.Write([]byte("data"))
      _ = tw.Close()

      err := sync.ExtractTar(bytes.NewReader(buf.Bytes()), dest, sync.Options{})
      if err != nil {
          t.Fatalf("ExtractTar() error = %v", err)
      }

      info, err := os.Stat(filepath.Join(dest, "rootfile.txt"))
      if err != nil {
          t.Fatalf("stat extracted file: %v", err)
      }
      stat, ok := info.Sys().(*syscall.Stat_t)
      if !ok {
          t.Fatal("cannot get file ownership")
      }
      if int(stat.Uid) != uid || int(stat.Gid) != gid {
          t.Fatalf("ownership = %d:%d, want %d:%d", stat.Uid, stat.Gid, uid, gid)
      }
  }
  ```

- [ ] Verify failure

  ```bash
  go test ./internal/sync -run TestExtractTar_ChownsExtractedFilesToHostUID -v
  ```

  Expected failure:

  ```text
  ownership = 0:0, want <host-uid>:<host-gid>
  ```

- [ ] Implement

  After the tar extraction loop in `extractTar` (after the `reconcileMissingEntries` call, before the final `return nil`), add a chown walk that changes all extracted files to the host uid/gid. Log chown failures as warnings but do not fail the extraction:

  ```go
  // Reown extracted files to the host user. Files created inside the
  // editor container may be owned by root or a different uid; the host
  // user must be able to overwrite them on subsequent runs.
  hostUID := os.Getuid()
  hostGID := os.Getgid()
  filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
      if err != nil {
          return nil // skip entries we can't walk
      }
      if err := os.Chown(path, hostUID, hostGID); err != nil {
          // Log but don't fail — some paths may be immutable or outside our control.
          slog.Warn("could not chown extracted file", "path", path, "error", err)
      }
      return nil
  })
  ```

  Export `extractTar` by creating a thin wrapper, or add a test-only helper. The simplest approach: create `ExtractTar` as the exported wrapper that calls `extractTar`:

  ```go
  // ExtractTar extracts a tar archive to dest, applying exclusion and chown.
  // Exported for testing.
  func ExtractTar(r io.Reader, dest string, opts Options) error {
      return extractTar(r, dest, opts)
  }
  ```

  Also update the test to use the exported name `sync.ExtractTar`.

- [ ] Verify pass

  ```bash
  go test ./internal/sync -run TestExtractTar_ChownsExtractedFilesToHostUID -v
  ```

  Expected result:

  ```text
  --- PASS: TestExtractTar_ChownsExtractedFilesToHostUID
  ```

- [ ] Commit

  ```bash
  git add internal/sync/transfer.go internal/sync/transfer_test.go
  git commit -m "fix(sync): chown extracted files to host uid/gid"
  ```

---

### Task 3: End-to-end verification

**Files:** all changed files

- [ ] Run full test suite

  ```bash
  go test -race ./...
  go tool golangci-lint run ./...
  ```

  Expected result:

  ```text
  ok  (all 11 packages pass, race-clean)
  (lint: clean)
  ```

- [ ] Build and smoke test

  ```bash
  go build -trimpath -ldflags="-s -w -X main.version=test -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" -o abx ./cmd/abx
  ABOX_RUNTIME=docker timeout 60 ./abx --editor opencode --offline "$(mktemp -d)" -- --version
  ```

  Expected result:

  ```text
  1.15.10
  (no sync-out ownership errors for clean workspace)
  ```

- [ ] Confirm final Git state

  ```bash
  git status --short
  git diff --check
  ```

  Expected result:

  ```text
  (clean working tree, no whitespace issues)
  ```
