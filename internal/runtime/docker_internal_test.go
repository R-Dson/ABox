package runtime

import (
	"context"
	"testing"
	"time"
)

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
