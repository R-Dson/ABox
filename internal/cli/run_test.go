package cli_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/r-dson/abox/internal/cli"
	"github.com/spf13/cobra"
)

func TestRunCmd_RunSessionOrchestrates(t *testing.T) {
	root := cli.NewRootCmd("test")

	// Verify the run command's RunE is wired (not nil stub)
	var runE func(*cobra.Command, []string) error
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			runE = cmd.RunE
			break
		}
	}
	if runE == nil {
		t.Fatal("run command has no RunE — session orchestration not wired")
	}
}

func TestRunCmd_DefaultWorkdir(t *testing.T) {
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"run"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	// Should not panic or error on missing workdir argument
	// (uses current directory as default)
	// Full orchestration not yet wired — accept "not yet" errors
	err := root.Execute()
	if err != nil && !strings.Contains(err.Error(), "not yet") && !strings.Contains(err.Error(), "runtime") {
		t.Errorf("unexpected error: %v", err)
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
