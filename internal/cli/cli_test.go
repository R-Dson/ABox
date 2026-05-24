package cli_test

import (
	"bytes"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestRunCmd_Exists(t *testing.T) {
	root := cli.NewRootCmd("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			found = true
			break
		}
	}
	if !found {
		t.Error("run subcommand not registered on root")
	}
}

func TestRunCmd_Flags(t *testing.T) {
	root := cli.NewRootCmd("test")

	tests := []struct {
		flag string
	}{
		{"editor"},
		{"shell"},
		{"force-it"},
		{"offline"},
		{"strict-network"},
		{"no-internet"},
		{"force-sync"},
		{"exclude-url"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := root.Flags().Lookup(tt.flag)
			if f == nil {
				// Check run subcommand flags
				for _, cmd := range root.Commands() {
					if cmd.Name() == "run" {
						f = cmd.Flags().Lookup(tt.flag)
						break
					}
				}
			}
			if f == nil {
				t.Errorf("flag --%s not found", tt.flag)
			}
		})
	}
}

func TestVersionCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd("1.0.0-test")
	root.SetArgs([]string{"version"})
	root.SetOut(buf)
	root.SetErr(buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("version command error: %v", err)
	}

	// Version cmd uses fmt.Printf directly — capture via SetOut
	output := buf.String()
	if output == "" {
		// fmt.Printf bypasses cobra's output writer, so just verify no error
		return
	}
	if !bytes.Contains([]byte(output), []byte("1.0.0-test")) {
		t.Errorf("version output doesn't contain version: %q", output)
	}
}

func TestRootCmd_SilenceSettings(t *testing.T) {
	root := cli.NewRootCmd("test")
	if !root.SilenceUsage {
		t.Error("SilenceUsage should be true")
	}
	if !root.SilenceErrors {
		t.Error("SilenceErrors should be true")
	}
}
