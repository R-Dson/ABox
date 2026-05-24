package cli

import (
	"github.com/r-dson/abox/internal/logging"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command for abx.
// SilenceUsage and SilenceErrors ensure main.go owns all output.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:               "abx",
		Short:             "Secure sandbox for AI coding editors",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			jsonLogs, _ := cmd.Flags().GetBool("json-logs")
			logging.Setup(verbose, jsonLogs)
			return nil
		},
	}

	root.PersistentFlags().Bool("verbose", false, "enable debug logging to ~/.local/state/abx/abx.log")
	root.PersistentFlags().Bool("json-logs", false, "emit JSON structured logs to stderr")

	// Default run when called with no subcommand
	root.RunE = newRunCmd().RunE

	root.AddCommand(
		newRunCmd(),
		newAuditCmd(),
		newConfigCmd(),
		newVersionCmd(version),
		newCompletionCmd(root),
	)
	return root
}
