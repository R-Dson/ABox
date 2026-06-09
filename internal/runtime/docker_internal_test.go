package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
)

func TestNormalizeSecurityOptInlinesSeccompProfilePath(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "seccomp.json")
	if err := os.WriteFile(profilePath, []byte(`{"defaultAction":"SCMP_ACT_ALLOW"}`), 0o600); err != nil {
		t.Fatalf("writing seccomp fixture: %v", err)
	}

	got, err := normalizeSecurityOpt([]string{"no-new-privileges", "seccomp=" + profilePath}, true)
	if err != nil {
		t.Fatalf("normalizeSecurityOpt() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("normalizeSecurityOpt() = %v", got)
	}
	if !strings.HasPrefix(got[1], "seccomp={") {
		t.Fatalf("seccomp profile was not inlined: %q", got[1])
	}
}

func TestNormalizeSecurityOptLeavesInlineSeccompAndUnconfined(t *testing.T) {
	opts := []string{"seccomp=unconfined", `seccomp={"defaultAction":"SCMP_ACT_ALLOW"}`}
	got, err := normalizeSecurityOpt(opts, true)
	if err != nil {
		t.Fatalf("normalizeSecurityOpt() error = %v", err)
	}
	for i := range opts {
		if got[i] != opts[i] {
			t.Fatalf("normalizeSecurityOpt()[%d] = %q, want %q", i, got[i], opts[i])
		}
	}
}

func TestContainerAttach_StdinFollowsSpec(t *testing.T) {
	d := &dockerRuntime{}
	d.recordOpenStdin("stdin", true)
	d.recordOpenStdin("no-stdin", false)

	if !d.attachOptions("stdin").Stdin {
		t.Fatal("AttachOptions.Stdin = false, want true for OpenStdin container")
	}
	if d.attachOptions("no-stdin").Stdin {
		t.Fatal("AttachOptions.Stdin = true, want false without OpenStdin")
	}
}

func TestContainerWait_WrapsErrorWithContainerID(t *testing.T) {
	wantErr := errors.New("daemon failed")
	statusCh := make(<-chan dockercontainer.WaitResponse)
	errCh := make(chan error, 1)
	errCh <- wantErr

	_, err := waitForContainerStatus("container-123", statusCh, errCh)
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitForContainerStatus() error = %v, want wrapped daemon error", err)
	}
	if !strings.Contains(err.Error(), "container-123") {
		t.Fatalf("wait error = %q, want container ID", err)
	}
}

func TestContainerWait_IgnoresClosedNilErrChannelUntilStatus(t *testing.T) {
	statusCh := make(chan dockercontainer.WaitResponse, 1)
	statusCh <- dockercontainer.WaitResponse{StatusCode: 23}
	errCh := make(chan error)
	close(errCh)

	code, err := waitForContainerStatus("container-123", statusCh, errCh)
	if err != nil {
		t.Fatalf("waitForContainerStatus() error = %v", err)
	}
	if code != 23 {
		t.Fatalf("exit code = %d, want 23", code)
	}
}

func TestWaitForExecResultPollsUntilNotRunning(t *testing.T) {
	calls := 0
	results := []execInspectResult{
		{Running: true, ExitCode: 0},
		{Running: true, ExitCode: 0},
		{Running: false, ExitCode: 17},
	}

	code, err := waitForExecResult(t.Context(), func(context.Context) (execInspectResult, error) {
		result := results[calls]
		calls++
		return result, nil
	}, time.Nanosecond)
	if err != nil {
		t.Fatalf("waitForExecResult() error = %v", err)
	}
	if code != 17 {
		t.Errorf("exit code = %d, want 17", code)
	}
	if calls != 3 {
		t.Errorf("inspect calls = %d, want 3", calls)
	}
}

func TestWaitForExecResultReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := waitForExecResult(ctx, func(context.Context) (execInspectResult, error) {
		return execInspectResult{Running: true}, nil
	}, time.Hour)
	if err == nil {
		t.Fatal("expected context error")
	}
}
