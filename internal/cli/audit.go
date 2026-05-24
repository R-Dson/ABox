package cli

import "github.com/spf13/cobra"

func newAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit [directory]",
		Short: "Pre-run workspace security check",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
}
