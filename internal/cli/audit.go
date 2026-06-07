package cli

import (
	"fmt"

	"github.com/r-dson/abox/internal/audit"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit [directory]",
		Short: "Audit workspace for security issues",
		Long:  "Run pre-flight security checks on a workspace directory before launching an editor.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir := "."
			if len(args) > 0 {
				workdir = args[0]
			}

			result, err := audit.Run(cmd.Context(), workdir)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			hasFailure := false
			for _, check := range result.Checks {
				icon := "✓"
				if check.Status == audit.Fail {
					icon = "✗"
					hasFailure = true
				} else if check.Status == audit.Warn {
					icon = "⚠"
				}
				fmt.Fprintf(w, "%s %s", icon, check.Name)
				if check.Detail != "" {
					fmt.Fprintf(w, ": %s", check.Detail)
				}
				fmt.Fprintln(w)
			}
			if hasFailure {
				return fmt.Errorf("audit failed")
			}
			return nil
		},
	}
	return cmd
}
