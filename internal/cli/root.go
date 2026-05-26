package cli

import (
	"fmt"
	"path/filepath"

	"github.com/r-dson/abox/internal/logging"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command for abx.
// The root command IS the run command — `abx [flags] [dir]` runs an editor.
// Subcommands (audit, config, version, completion) are registered separately.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "abx [flags] [directory]",
		Short:         "Secure sandbox for AI coding editors",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			jsonLogs, _ := cmd.Flags().GetBool("json-logs")
			logging.Setup(verbose, jsonLogs)
			return nil
		},
	}

	root.PersistentFlags().Bool("verbose", false, "enable debug logging to ~/.local/state/abx/abx.log")
	root.PersistentFlags().Bool("json-logs", false, "emit JSON structured logs to stderr")

	// Register run flags directly on root
	cfg := &SessionConfig{}
	root.Flags().StringVar(&cfg.Editor, "editor", "", "editor to use (aider|claude|codex|copilot|gemini|goose|opencode|pi|vibe)")
	root.Flags().BoolVar(&cfg.Shell, "shell", false, "drop into an interactive shell")
	root.Flags().BoolVar(&cfg.ForceIT, "force-it", false, "force interactive TTY allocation")
	root.Flags().BoolVar(&cfg.Offline, "offline", false, "do not pull images")
	root.Flags().BoolVar(&cfg.StrictNetwork, "strict-network", false, "block all external network access")
	root.Flags().BoolVar(&cfg.NoInternet, "no-internet", false, "disable networking entirely")
	root.Flags().BoolVar(&cfg.ForceSync, "force-sync", false, "overwrite host files even if modified during session")
	root.Flags().StringArrayVar(&cfg.ExtraEnv, "env", nil, "pass environment variable to container (repeatable)")

	// Run logic on root
	root.RunE = func(cmd *cobra.Command, args []string) error {
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
		cfg.ExtraEnv = append(cfg.ExtraEnv, LoadDotEnv(absWorkdir)...)
		rt, err := runtime.Detect(cmd.Context())
		if err != nil {
			return err
		}
		return RunSession(cmd.Context(), rt, absWorkdir, cfg)
	}

	root.AddCommand(
		newAuditCmd(),
		newConfigCmd(),
		newVersionCmd(version),
		newCompletionCmd(root),
	)

	return root
}
