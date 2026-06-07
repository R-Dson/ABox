package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds all user-configurable settings.
type Config struct {
	Editor            string  `mapstructure:"editor" json:"editor"`
	ExcludeURL        string  `mapstructure:"exclude_url" json:"exclude_url"`
	NoInternet        bool    `mapstructure:"no_internet" json:"no_internet"`
	StrictNetwork     bool    `mapstructure:"strict_network" json:"strict_network"`
	PullPolicy        string  `mapstructure:"pull_policy" json:"pull_policy"`
	MemoryLimit       string  `mapstructure:"memory_limit" json:"memory_limit"`
	CPULimit          float64 `mapstructure:"cpu_limit" json:"cpu_limit"`
	ForwardSSHAgent   bool    `mapstructure:"forward_ssh_agent" json:"forward_ssh_agent"`
	TrustWorkspaceEnv bool    `mapstructure:"trust_workspace_env" json:"trust_workspace_env"`
	Verbose           bool    `mapstructure:"verbose" json:"verbose"`
	JSONLogs          bool    `mapstructure:"json_logs" json:"json_logs"`
}

// Load reads configuration from the provided Viper instance.
// Missing config file is not an error — defaults apply.
func Load(v *viper.Viper) (*Config, error) {
	v.SetDefault("editor", "opencode")
	v.SetDefault("pull_policy", "never")
	v.SetDefault("memory_limit", "4g")
	v.SetDefault("cpu_limit", 2.0)
	v.SetDefault("trust_workspace_env", false)

	if err := v.ReadInConfig(); err != nil {
		// Missing config file is fine — use defaults
		// Viper returns different error types depending on how config path was set,
		// so check for any file-not-found variant.
		if !isConfigNotFound(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}
	return &cfg, nil
}

// isConfigNotFound returns true if the error is due to a missing config file.
func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	if errors.As(err, &notFound) {
		return true
	}
	return errors.Is(err, os.ErrNotExist)
}
