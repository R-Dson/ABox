package cli

import (
	"github.com/spf13/cobra"
)

// RunOptions holds all CLI flags for the run command.
type RunOptions struct {
	Editor        string
	Shell         bool
	ForceIT       bool
	Offline       bool
	StrictNetwork bool
	NoInternet    bool
	Verbose       bool
	ForceSync     bool
	ExcludeURL    string
	ExtraEnv      []string
	EditorArgs    []string
}

func newRunCmd() *cobra.Command {
	opts := &RunOptions{}
	cmd := &cobra.Command{
		Use:   "run [directory]",
		Short: "Run an editor in a secure sandbox",
		Long:  "Launch an AI coding editor inside an isolated container with workspace sync and exclusion filtering.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Editor, "editor", "", "editor to use (aider|claude|codex|copilot|gemini|goose|opencode|vibe)")
	cmd.Flags().BoolVar(&opts.Shell, "shell", false, "drop into an interactive shell")
	cmd.Flags().BoolVar(&opts.ForceIT, "force-it", false, "force interactive TTY allocation")
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "do not pull images")
	cmd.Flags().BoolVar(&opts.StrictNetwork, "strict-network", false, "block all external network access")
	cmd.Flags().BoolVar(&opts.NoInternet, "no-internet", false, "disable networking entirely")
	cmd.Flags().BoolVar(&opts.ForceSync, "force-sync", false, "overwrite host files even if modified during session")
	cmd.Flags().StringVar(&opts.ExcludeURL, "exclude-url", "", "URL to fetch exclusion patterns from")
	cmd.Flags().StringArrayVar(&opts.ExtraEnv, "env", nil, "pass environment variable to container (repeatable)")

	return cmd
}
