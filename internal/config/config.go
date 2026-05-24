package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds all user-configurable settings.
type Config struct {
	Editor        string  `mapstructure:"editor"`
	ExcludeURL    string  `mapstructure:"exclude_url"`
	NoInternet    bool    `mapstructure:"no_internet"`
	StrictNetwork bool    `mapstructure:"strict_network"`
	PullPolicy    string  `mapstructure:"pull_policy"`
	MemoryLimit   string  `mapstructure:"memory_limit"`
	CPULimit      float64 `mapstructure:"cpu_limit"`
	Verbose       bool    `mapstructure:"verbose"`
	JSONLogs      bool    `mapstructure:"json_logs"`
}

// Load reads configuration from the provided Viper instance.
// Missing config file is not an error — defaults apply.
func Load(v *viper.Viper) (*Config, error) {
	v.SetDefault("editor", "opencode")
	v.SetDefault("pull_policy", "missing")
	v.SetDefault("memory_limit", "4g")
	v.SetDefault("cpu_limit", 2.0)

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
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	// Viper wraps some not-found errors; check the error string as last resort.
	return false
}
