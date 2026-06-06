package cli_test

import (
	"bytes"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestRootCmd_HasRunFlags(t *testing.T) {
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
				t.Errorf("flag --%s not found on root", tt.flag)
			}
		})
	}
}

func TestRootCmd_HasSubcommands(t *testing.T) {
	root := cli.NewRootCmd("test")

	expected := []string{"audit", "completion", "config", "version"}
	for _, name := range expected {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subcommand %q not registered on root", name)
		}
	}
}

func TestRootCmd_RunE(t *testing.T) {
	root := cli.NewRootCmd("test")
	if root.RunE == nil {
		t.Error("root should have RunE set (run is the default action)")
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

	output := buf.String()
	if output == "" {
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
