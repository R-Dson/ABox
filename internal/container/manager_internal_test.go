package container

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/r-dson/abox/internal/runtimetest"
)

func TestPrepareRawTerminalRestoresState(t *testing.T) {
	makeRawCalled := false
	restoreCalled := false

	state, err := prepareRawTerminal(true, func() (func() error, error) {
		makeRawCalled = true
		return func() error {
			restoreCalled = true
			return nil
		}, nil
	})
	if err != nil {
		t.Fatalf("prepareRawTerminal() error = %v", err)
	}
	if !makeRawCalled {
		t.Fatal("expected raw mode setup")
	}

	state.restore()
	if !restoreCalled {
		t.Fatal("expected terminal restore")
	}
}

func TestPrepareRawTerminalSkipsNonTTY(t *testing.T) {
	state, err := prepareRawTerminal(false, func() (func() error, error) {
		t.Fatal("makeRaw should not be called for non-TTY")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("prepareRawTerminal() error = %v", err)
	}
	state.restore()
}

func TestResizeContainerTTY(t *testing.T) {
	var gotHeight uint
	var gotWidth uint
	stub := &runtimetest.StubRuntime{
		ContainerResizeFn: func(_ context.Context, _ string, height, width uint) error {
			gotHeight = height
			gotWidth = width
			return nil
		},
	}

	if err := resizeContainerTTY(t.Context(), stub, "c-1", func() (uint, uint, bool) {
		return 24, 80, true
	}); err != nil {
		t.Fatalf("resizeContainerTTY() error = %v", err)
	}

	if gotHeight != 24 || gotWidth != 80 {
		t.Fatalf("resize = %dx%d, want 24x80", gotHeight, gotWidth)
	}
}

func TestForwardContainerSignal(t *testing.T) {
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	var gotSignal string
	stub := &runtimetest.StubRuntime{
		ContainerSignalFn: func(_ context.Context, _ string, signal string) error {
			gotSignal = signal
			close(done)
			return nil
		},
	}

	stop := forwardContainerSignals(t.Context(), stub, "c-1", signals)
	defer stop()

	signals <- syscall.SIGINT
	<-done

	if gotSignal != "SIGINT" {
		t.Fatalf("signal = %q, want SIGINT", gotSignal)
	}
}

func TestStopForwardedSignalsRunsBothCleanupCallbacks(t *testing.T) {
	stoppedOSSignals := false
	stoppedForwarder := false

	stopForwardedSignals(func() {
		stoppedOSSignals = true
	}, func() {
		stoppedForwarder = true
	})

	if !stoppedOSSignals {
		t.Fatal("expected OS signal cleanup")
	}
	if !stoppedForwarder {
		t.Fatal("expected forwarding goroutine cleanup")
	}
}

func TestStreamContainerOutput(t *testing.T) {
	attached := &bufferReadWriteCloser{reader: strings.NewReader("container output")}
	var stdout bytes.Buffer

	err := <-streamContainerIO(attached, nil, &stdout, io.Discard, false, true)
	if err != nil {
		t.Fatalf("streamContainerIO() error = %v", err)
	}

	if got := stdout.String(); got != "container output" {
		t.Errorf("stdout = %q, want %q", got, "container output")
	}
}

func TestStreamContainerOutputDemuxesNonTTY(t *testing.T) {
	var muxed bytes.Buffer
	stdoutWriter := stdcopy.NewStdWriter(&muxed, stdcopy.Stdout)
	stderrWriter := stdcopy.NewStdWriter(&muxed, stdcopy.Stderr)
	if _, err := stdoutWriter.Write([]byte("out")); err != nil {
		t.Fatalf("writing stdout frame: %v", err)
	}
	if _, err := stderrWriter.Write([]byte("err")); err != nil {
		t.Fatalf("writing stderr frame: %v", err)
	}

	attached := &bufferReadWriteCloser{reader: bytes.NewReader(muxed.Bytes())}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := <-streamContainerIO(attached, nil, &stdout, &stderr, false, false)
	if err != nil {
		t.Fatalf("streamContainerIO() error = %v", err)
	}
	if stdout.String() != "out" {
		t.Fatalf("stdout = %q, want out", stdout.String())
	}
	if stderr.String() != "err" {
		t.Fatalf("stderr = %q, want err", stderr.String())
	}
}

type bufferReadWriteCloser struct {
	reader io.Reader
	writer bytes.Buffer
}

func (b *bufferReadWriteCloser) Read(p []byte) (int, error) {
	return b.reader.Read(p) //nolint:wrapcheck // io.Reader implementations must preserve io.EOF.
}

func (b *bufferReadWriteCloser) Write(p []byte) (int, error) {
	written, _ := b.writer.Write(p)
	return written, nil
}

func (b *bufferReadWriteCloser) Close() error {
	return nil
}
