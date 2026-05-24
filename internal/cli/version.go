package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print abx version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("abx %s (go %s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
