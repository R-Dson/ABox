package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/logging"
	"github.com/r-dson/abox/internal/runtime"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
			loadedConfig, err := loadUserConfig()
			if err != nil {
				return err
			}

			verbose := loadedConfig.Verbose
			if cmd.Flags().Changed("verbose") {
				verbose, _ = cmd.Flags().GetBool("verbose")
			}

			jsonLogs := loadedConfig.JSONLogs
			if cmd.Flags().Changed("json-logs") {
				jsonLogs, _ = cmd.Flags().GetBool("json-logs")
			}

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
	root.Flags().BoolVar(&cfg.ForwardSSHAgent, "ssh-agent", false, "forward the host SSH agent into the container")
	root.Flags().StringVar(&cfg.ExcludeURL, "exclude-url", "", "fetch additional exclusion patterns from URL")
	root.Flags().StringArrayVar(&cfg.ExtraEnv, "env", nil, "pass environment variable to container (repeatable)")

	// Run logic on root
	root.RunE = func(cmd *cobra.Command, args []string) error {
		workdir := "."
		if len(args) > 0 {
			workdir = args[0]
		}
		absWorkdir, err := resolveWorkdir(workdir)
		if err != nil {
			return err
		}
		loadedConfig, err := loadUserConfig()
		if err != nil {
			return err
		}
		applyLoadedConfig(cmd, cfg, loadedConfig)

		dotEnv, err := LoadDotEnv(absWorkdir)
		if err != nil {
			return err
		}
		cfg.ExtraEnv = append(cfg.ExtraEnv, dotEnv...)
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

func resolveWorkdir(workdir string) (string, error) {
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolving workdir: %w", err)
	}
	resolvedWorkdir, err := filepath.EvalSymlinks(absWorkdir)
	if err != nil {
		return "", fmt.Errorf("resolving workspace symlinks: %w", err)
	}
	if err := ValidateWorkdir(resolvedWorkdir); err != nil {
		return "", err
	}
	return resolvedWorkdir, nil
}

func loadUserConfig() (*config.Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolving user config directory: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(filepath.Join(configDir, "abx", "config.json"))
	v.SetConfigType("json")
	v.SetEnvPrefix("ABX")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	loadedConfig, err := config.Load(v)
	if err != nil {
		return nil, fmt.Errorf("loading user config: %w", err)
	}
	return loadedConfig, nil
}

func applyLoadedConfig(cmd *cobra.Command, cfg *SessionConfig, loadedConfig *config.Config) {
	if !cmd.Flags().Changed("editor") {
		cfg.Editor = loadedConfig.Editor
	}
	if !cmd.Flags().Changed("strict-network") {
		cfg.StrictNetwork = loadedConfig.StrictNetwork
	}
	if !cmd.Flags().Changed("no-internet") {
		cfg.NoInternet = loadedConfig.NoInternet
	}
	if !cmd.Flags().Changed("exclude-url") {
		cfg.ExcludeURL = loadedConfig.ExcludeURL
	}
	if !cmd.Flags().Changed("ssh-agent") {
		cfg.ForwardSSHAgent = loadedConfig.ForwardSSHAgent
	}
	cfg.PullPolicy = loadedConfig.PullPolicy
	if cfg.Offline || cfg.NoInternet {
		cfg.PullPolicy = "never"
	}
	cfg.MemoryLimit = loadedConfig.MemoryLimit
	cfg.CPULimit = loadedConfig.CPULimit
}
