package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// VersionInfo contains build metadata shown by the version command.
type VersionInfo struct {
	Commit string
}

func newVersionCmd(info VersionInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print abx version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s %s/%s)\n", info.Commit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
