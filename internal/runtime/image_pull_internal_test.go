package runtime

import (
	"errors"
	"io"
	"testing"
)

func TestDrainImagePullStreamReturnsCopyError(t *testing.T) {
	reader := failingReadCloser{readErr: errors.New("read failed")}

	err := drainImagePullStream(reader, io.Discard)
	if err == nil {
		t.Fatal("expected copy error")
	}
}

func TestDrainImagePullStreamReturnsCloseError(t *testing.T) {
	reader := failingReadCloser{closeErr: errors.New("close failed")}

	err := drainImagePullStream(reader, io.Discard)
	if err == nil {
		t.Fatal("expected close error")
	}
}

type failingReadCloser struct {
	readErr  error
	closeErr error
}

func (r failingReadCloser) Read(_ []byte) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	return 0, io.EOF
}

func (r failingReadCloser) Close() error {
	return r.closeErr
}
