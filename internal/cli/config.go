package cli

import "github.com/spf13/cobra"

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage abx configuration",
	}
	cmd.RunE = func(*cobra.Command, []string) error {
		return cmd.Help()
	}
	return cmd
}
