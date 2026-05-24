package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/r-dson/abox/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage abx configuration",
	}
	cmd.RunE = func(*cobra.Command, []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(newSetEditorCmd())
	cmd.AddCommand(newListEditorsCmd())

	return cmd
}

func newListEditorsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-editors",
		Short: "List available editors from the embedded registry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := config.LoadEditorRegistry()
			if err != nil {
				return fmt.Errorf("loading registry: %w", err)
			}

			for _, name := range registry.Names() {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		},
	}
}

func newSetEditorCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "set-editor <name>",
		Short: "Set the default editor",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			registry, err := config.LoadEditorRegistry()
			if err != nil {
				return fmt.Errorf("loading registry: %w", err)
			}

			if _, err := registry.Get(name); err != nil {
				return fmt.Errorf("unknown editor %q: %w", name, err)
			}

			// Verify the name actually exists (Get falls back to opencode)
			found := false
			for _, n := range registry.Names() {
				if n == name {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown editor %q (available: %v)", name, registry.Names())
			}

			if cfgPath == "" {
				dir, _ := os.UserConfigDir()
				cfgPath = dir + "/abx/config.json"
			}

			return writeConfigField(cfgPath, "editor", name)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "config file path (default: $XDG_CONFIG_HOME/abx/config.json)")

	return cmd
}

// writeConfigField writes a single field to a JSON config file,
// creating the file and parent directories if needed.
func writeConfigField(path, key, value string) error {
	if err := os.MkdirAll(filepathonly(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data := map[string]string{}
	if existing, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(existing, &data); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}
	data[key] = value

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func filepathonly(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
