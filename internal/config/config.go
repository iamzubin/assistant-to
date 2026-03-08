package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// WatchdogConfig holds timeout and interval settings for agent monitoring
type WatchdogConfig struct {
	CheckInterval       int `yaml:"check_interval_seconds"`     // How often to check agent health (default: 30)
	StallTimeout        int `yaml:"stall_timeout_seconds"`      // Time before considering agent stalled (default: 300 = 5min)
	RecoveryWaitTime    int `yaml:"recovery_wait_time_seconds"` // Time to wait after recovery attempt before escalation (default: 5)
	EscapeKeyCount      int `yaml:"escape_key_count"`           // Number of escape keys to send when recovering (default: 2)
	MaxRecoveryAttempts int `yaml:"max_recovery_attempts"`      // Max recovery attempts before giving up (default: 3)
}

// Config represents the user's project-level configuration.
type Config struct {
	Tool        string         `yaml:"tool"`
	ModelLarge  string         `yaml:"model_large"`
	ModelMedium string         `yaml:"model_medium"`
	ModelFast   string         `yaml:"model_fast"`
	Watchdog    WatchdogConfig `yaml:"watchdog"`
}

// Default returns a default configuration object.
func Default() *Config {
	return &Config{
		Tool:        "gemini",
		ModelLarge:  "gemini-3.1-pro-preview",
		ModelMedium: "gemini-3-flash-preview",
		ModelFast:   "gemini-2.5-flash",
		Watchdog: WatchdogConfig{
			CheckInterval:       30,
			StallTimeout:        300,
			RecoveryWaitTime:    5,
			EscapeKeyCount:      2,
			MaxRecoveryAttempts: 3,
		},
	}
}

// Load reads a config.yaml file from the specified path and unmarshals it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var conf Config
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &conf, nil
}

// ModelForRole returns the appropriate model string for a given agent role.
// Roles: "Coordinator" -> large, "Scout" -> fast, all others -> medium.
func (c *Config) ModelForRole(role string) string {
	switch role {
	case "Coordinator":
		return c.ModelLarge
	case "Scout":
		return c.ModelFast
	default: // Builder, Reviewer, Merger
		return c.ModelMedium
	}
}

// GetWatchdogCheckInterval returns the health check interval with default fallback
func (c *Config) GetWatchdogCheckInterval() time.Duration {
	if c.Watchdog.CheckInterval <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.Watchdog.CheckInterval) * time.Second
}

// GetWatchdogStallTimeout returns the stall timeout with default fallback
func (c *Config) GetWatchdogStallTimeout() time.Duration {
	if c.Watchdog.StallTimeout <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.Watchdog.StallTimeout) * time.Second
}

// GetWatchdogRecoveryWaitTime returns the recovery wait time with default fallback
func (c *Config) GetWatchdogRecoveryWaitTime() time.Duration {
	if c.Watchdog.RecoveryWaitTime <= 0 {
		return 5 * time.Second
	}
	return time.Duration(c.Watchdog.RecoveryWaitTime) * time.Second
}

// GetWatchdogEscapeKeyCount returns the number of escape keys to send with default fallback
func (c *Config) GetWatchdogEscapeKeyCount() int {
	if c.Watchdog.EscapeKeyCount <= 0 {
		return 2
	}
	return c.Watchdog.EscapeKeyCount
}

// GetWatchdogMaxRecoveryAttempts returns the max recovery attempts with default fallback
func (c *Config) GetWatchdogMaxRecoveryAttempts() int {
	if c.Watchdog.MaxRecoveryAttempts <= 0 {
		return 3
	}
	return c.Watchdog.MaxRecoveryAttempts
}

// Save writes the current configuration to the specified path as YAML.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
