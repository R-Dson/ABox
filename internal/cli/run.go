package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
		RunE:  runSession(opts),
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

// runSession returns a RunE function that orchestrates the full session lifecycle:
// config → registry → runtime → exclusion matcher → session → snapshot →
// SyncIn → Run → conflict check → SyncOut → exit code.
func runSession(_ *RunOptions) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		// Resolve workdir: explicit arg or current directory
		workdir := "."
		if len(args) > 0 {
			workdir = args[0]
		}
		absWorkdir, err := filepath.Abs(workdir)
		if err != nil {
			return fmt.Errorf("resolving workdir: %w", err)
		}

		if err := ValidateWorkdir(absWorkdir); err != nil {
			return err
		}

		// TODO: Wire full orchestration (config, registry, runtime, sync, etc.)
		// For now, validate inputs and return.
		return fmt.Errorf("session orchestration not yet fully wired: workdir=%s", absWorkdir)
	}
}

// ValidateWorkdir rejects unsafe workspace paths.
func ValidateWorkdir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	home, _ := os.UserHomeDir()
	if home != "" && abs == home {
		return fmt.Errorf("cannot use $HOME (%s) as workspace", abs)
	}
	if abs == "/" {
		return fmt.Errorf("cannot use / as workspace")
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("workspace %s does not exist", abs)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace %s is not a directory", abs)
	}

	return nil
}
