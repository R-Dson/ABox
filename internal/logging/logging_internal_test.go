package logging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestLogging_CloseClosesVerboseFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Setup(true, false); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
	if verboseLogFile == nil {
		t.Fatal("verbose log file was not opened")
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
	if verboseLogFile != nil {
		t.Fatal("verbose log file was not cleared after shutdown")
	}
}

func TestMultiHandlerAttemptsAllAndJoinsErrors(t *testing.T) {
	errOne := errors.New("one")
	errTwo := errors.New("two")
	first := &recordingHandler{err: errOne}
	second := &recordingHandler{err: errTwo}
	h := &multiHandler{handlers: []slog.Handler{first, second}}

	err := h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))
	if !first.called || !second.called {
		t.Fatalf("called first=%v second=%v, want both", first.called, second.called)
	}
	if !errors.Is(err, errOne) || !errors.Is(err, errTwo) {
		t.Fatalf("Handle() error = %v, want joined one and two", err)
	}
}

type recordingHandler struct {
	called bool
	err    error
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(context.Context, slog.Record) error {
	h.called = true
	return h.err
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }
