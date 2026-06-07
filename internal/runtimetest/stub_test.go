package runtimetest

import "testing"

func TestNopReadWriteCloserWriteReturnsLen(t *testing.T) {
	payload := []byte("hello")
	n, err := nopReadWriteCloser{}.Write(payload)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write() = %d, want %d", n, len(payload))
	}
}
