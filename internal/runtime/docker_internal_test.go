package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSecurityOptInlinesSeccompProfilePath(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "seccomp.json")
	if err := os.WriteFile(profilePath, []byte(`{"defaultAction":"SCMP_ACT_ALLOW"}`), 0o600); err != nil {
		t.Fatalf("writing seccomp fixture: %v", err)
	}

	got, err := normalizeSecurityOpt([]string{"no-new-privileges", "seccomp=" + profilePath})
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
	got, err := normalizeSecurityOpt(opts)
	if err != nil {
		t.Fatalf("normalizeSecurityOpt() error = %v", err)
	}
	for i := range opts {
		if got[i] != opts[i] {
			t.Fatalf("normalizeSecurityOpt()[%d] = %q, want %q", i, got[i], opts[i])
		}
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
