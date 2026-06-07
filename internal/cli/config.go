package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

			if !registry.Has(name) {
				return fmt.Errorf("unknown editor %q (available: %v)", name, registry.Names())
			}

			if cfgPath == "" {
				dir, err := os.UserConfigDir()
				if err != nil {
					return fmt.Errorf("resolving user config directory: %w", err)
				}
				cfgPath = filepath.Join(dir, "abx", "config.json")
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data := map[string]any{}
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

	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
