package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup configures the global slog logger.
// When verbose is true, logs are written to ~/.local/state/abx/abx.log.
// When jsonOutput is true, stderr gets JSON-formatted logs.
func Setup(verbose, jsonOutput bool) {
	var handlers []slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}

	if verbose {
		logDir := filepath.Join(homeDir(), ".local", "state", "abx")
		if err := os.MkdirAll(logDir, 0o700); err == nil {
			f, err := os.OpenFile(
				filepath.Join(logDir, "abx.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				handlers = append(handlers, slog.NewTextHandler(f, opts))
			}
		}
	}

	if jsonOutput || !isTerminal(os.Stderr) {
		handlers = append(handlers, slog.NewJSONHandler(os.Stderr, opts))
	} else {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, opts))
	}

	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = &multiHandler{handlers: handlers}
	}
	slog.SetDefault(slog.New(handler))
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// multiHandler fans out log records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if err := h.Handle(ctx, r); err != nil {
			return fmt.Errorf("multi-handler: %w", err)
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}

// Verify interface compliance
var _ slog.Handler = (*multiHandler)(nil)

// Discard is a convenience writer for suppressing output in tests.
var Discard = io.Discard
