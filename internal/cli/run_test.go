package cli_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestRootCmd_RunEIsWired(t *testing.T) {
	root := cli.NewRootCmd("test")
	if root.RunE == nil {
		t.Fatal("root should have RunE set — run is the default action")
	}
}

func TestRootCmd_DefaultWorkdir(t *testing.T) {
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{}) // no args = default workdir "."
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	err := root.Execute()
	if err != nil {
		msg := err.Error()
		// Full orchestration requires Docker. Accept any runtime error.
		if !strings.Contains(msg, "runtime") &&
			!strings.Contains(msg, "image") &&
			!strings.Contains(msg, "unreachable") &&
			!strings.Contains(msg, "ExitError") &&
			!strings.Contains(msg, "mount") &&
			!strings.Contains(msg, "container") &&
			!strings.Contains(msg, "workspace") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestRunCmd_ValidateWorkdir(t *testing.T) {
	tests := []struct {
		name    string
		workdir string
		wantErr bool
	}{
		{"home rejected", os.Getenv("HOME"), true},
		{"root rejected", "/", true},
		{"tmp allowed", "/tmp", false},
		{"nonexistent rejected", "/nonexistent/path/xyz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cli.ValidateWorkdir(tt.workdir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkdir(%q) error = %v, wantErr %v", tt.workdir, err, tt.wantErr)
			}
		})
	}
}
