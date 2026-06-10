package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPullProgress_AggregatesPercent(t *testing.T) {
	var buf bytes.Buffer
	p := newPullProgress(&buf, "test-image")
	p.isTTY = true // force TTY mode for test
	buf.Reset()

	lines := []string{
		`{"status":"Downloading","progressDetail":{"current":100,"total":200},"id":"aaa"}`,
		`{"status":"Downloading","progressDetail":{"current":50,"total":100},"id":"bbb"}`,
	}
	for _, line := range lines {
		if _, err := p.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("Write() error: %v", err)
		}
	}

	out := buf.String()
	if !strings.Contains(out, " 50%") {
		t.Errorf("expected 50%% aggregate, got %q", out)
	}
}

func TestPullProgress_PrintsInitialPrefix(t *testing.T) {
	var buf bytes.Buffer
	_ = newPullProgress(&buf, "myimage")

	if !strings.HasPrefix(buf.String(), "Pulling myimage...") {
		t.Errorf("expected prefix, got %q", buf.String())
	}
}

func TestPullProgress_IgnoresNonDownload(t *testing.T) {
	var buf bytes.Buffer
	p := newPullProgress(&buf, "test-image")
	buf.Reset()

	if _, err := p.Write([]byte(`{"status":"Pulling from library/alpine","id":""}` + "\n")); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if _, err := p.Write([]byte(`{"status":"Already exists","id":"aaa"}` + "\n")); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("expected no additional output for non-download status, got %q", buf.String())
	}
}

func TestPullProgress_FinishNonTTY(t *testing.T) {
	var buf bytes.Buffer
	p := newPullProgress(&buf, "test-image")
	buf.Reset()

	p.finish()

	want := "Pulling test-image... done.\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPullProgress_FinishTTY(t *testing.T) {
	var buf bytes.Buffer
	p := newPullProgress(&buf, "test-image")
	p.isTTY = true
	buf.Reset()

	p.finish()

	want := "\rPulling test-image... done.\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
