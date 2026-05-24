package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/cli"
)

func TestConfigCmd_ListEditors(t *testing.T) {
	root := cli.NewRootCmd("test")
	buf := new(bytes.Buffer)
	root.SetArgs([]string{"config", "list-editors"})
	root.SetOut(buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("list-editors error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("list-editors produced no output")
	}

	// Should contain known editors from embedded registry
	editors := []string{"aider", "claude", "opencode", "codex"}
	for _, name := range editors {
		if !bytes.Contains([]byte(output), []byte(name)) {
			t.Errorf("output missing editor %q: %s", name, output)
		}
	}
}

func TestConfigCmd_SetEditor(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"config", "set-editor", "claude", "--config", cfgPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("set-editor error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !bytes.Contains(data, []byte(`"editor": "claude"`)) {
		t.Errorf("config does not contain editor setting: %s", data)
	}
}

func TestConfigCmd_SetEditor_Invalid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"config", "set-editor", "nonexistent-editor", "--config", cfgPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid editor")
	}
}
