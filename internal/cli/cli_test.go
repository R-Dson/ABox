package cli_test

import (
	"bytes"
	"strings"
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
		{"ssh-agent"},
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

func TestAuditReturnsNonZeroOnFail(t *testing.T) {
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"audit", "/"})
	root.SetOut(buf)
	root.SetErr(buf)

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "audit failed") {
		t.Fatalf("audit error = %v, want audit failed", err)
	}
	if !strings.Contains(buf.String(), "✗ workdir_safety") {
		t.Fatalf("audit output = %q, want failed workdir_safety detail", buf.String())
	}
}

func TestCompletionValidArgs(t *testing.T) {
	root := cli.NewRootCmd("test")
	completion, _, err := root.Find([]string{"completion"})
	if err != nil {
		t.Fatalf("finding completion command: %v", err)
	}
	want := []string{"bash", "zsh", "fish", "powershell"}
	if len(completion.ValidArgs) != len(want) {
		t.Fatalf("ValidArgs = %v, want %v", completion.ValidArgs, want)
	}
	for i := range want {
		if completion.ValidArgs[i] != want[i] {
			t.Fatalf("ValidArgs = %v, want %v", completion.ValidArgs, want)
		}
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
	root := cli.NewRootCmdWithVersion(cli.VersionInfo{Version: "1.0.0-test", Commit: "abc123", Date: "2026-06-08T00:00:00Z"})
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
	for _, want := range []string{"1.0.0-test", "abc123", "2026-06-08T00:00:00Z"} {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Errorf("version output doesn't contain %q: %q", want, output)
		}
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
