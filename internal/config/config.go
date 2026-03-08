package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig holds project-level settings
type ProjectConfig struct {
	Name            string `yaml:"name"`
	Root            string `yaml:"root"`
	CanonicalBranch string `yaml:"canonicalBranch"`
}

// AgentsConfig holds agent orchestration settings
type AgentsConfig struct {
	ManifestPath   string `yaml:"manifestPath"`
	BaseDir        string `yaml:"baseDir"`
	MaxConcurrent  int    `yaml:"maxConcurrent"`
	StaggerDelayMs int    `yaml:"staggerDelayMs"`
	MaxDepth       int    `yaml:"maxDepth"`
	ScoutEnabled   bool   `yaml:"scoutEnabled"`
	ScoutWaitSec   int    `yaml:"scoutWaitSec"`
}

// WorktreesConfig holds worktree settings
type WorktreesConfig struct {
	BaseDir string `yaml:"baseDir"`
}

// TaskTrackerConfig holds task tracking settings
type TaskTrackerConfig struct {
	Enabled bool `yaml:"enabled"`
}

// MulchConfig holds intelligence/mulch settings
type MulchConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Domains     []string `yaml:"domains"`
	PrimeFormat string   `yaml:"primeFormat"`
}

// MergeConfig holds merge resolution settings
type MergeConfig struct {
	AIResolveEnabled bool `yaml:"aiResolveEnabled"`
	ReimagineEnabled bool `yaml:"reimagineEnabled"`
}

// WatchdogConfig holds timeout and interval settings for agent monitoring
type WatchdogConfig struct {
	Tier0Enabled        bool `yaml:"tier0Enabled"`
	Tier0IntervalMs     int  `yaml:"tier0IntervalMs"`
	Tier1Enabled        bool `yaml:"tier1Enabled"`
	Tier2Enabled        bool `yaml:"tier2Enabled"`
	StaleThresholdMs    int  `yaml:"staleThresholdMs"`
	ZombieThresholdMs   int  `yaml:"zombieThresholdMs"`
	NudgeIntervalMs     int  `yaml:"nudgeIntervalMs"`
	RecoveryWaitTime    int  `yaml:"recoveryWaitTimeSeconds"` // Deprecated: use nudgeIntervalMs
	EscapeKeyCount      int  `yaml:"escapeKeyCount"`
	MaxRecoveryAttempts int  `yaml:"maxRecoveryAttempts"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Verbose       bool `yaml:"verbose"`
	RedactSecrets bool `yaml:"redactSecrets"`
}

// Config represents the user's project-level configuration.
type Config struct {
	Tool        string            `yaml:"tool"`
	ModelLarge  string            `yaml:"model_large"`
	ModelMedium string            `yaml:"model_medium"`
	ModelFast   string            `yaml:"model_fast"`
	Project     ProjectConfig     `yaml:"project"`
	Agents      AgentsConfig      `yaml:"agents"`
	Worktrees   WorktreesConfig   `yaml:"worktrees"`
	TaskTracker TaskTrackerConfig `yaml:"taskTracker"`
	Mulch       MulchConfig       `yaml:"mulch"`
	Merge       MergeConfig       `yaml:"merge"`
	Watchdog    WatchdogConfig    `yaml:"watchdog"`
	Logging     LoggingConfig     `yaml:"logging"`
}

// Default returns a default configuration object.
func Default() *Config {
	return &Config{
		Tool:        "gemini",
		ModelLarge:  "gemini-2.5-pro",
		ModelMedium: "gemini-2.5-flash",
		ModelFast:   "gemini-2.5-flash-lite",
		Project: ProjectConfig{
			Name:            "",
			Root:            ".",
			CanonicalBranch: "main",
		},
		Agents: AgentsConfig{
			ManifestPath:   ".assistant-to/agent-manifest.json",
			BaseDir:        ".assistant-to/agent-defs",
			MaxConcurrent:  5,
			StaggerDelayMs: 2000,
			MaxDepth:       2,
			ScoutEnabled:   true,
			ScoutWaitSec:   600, // 10 minutes
		},
		Worktrees: WorktreesConfig{
			BaseDir: ".assistant-to/worktrees",
		},
		TaskTracker: TaskTrackerConfig{
			Enabled: true,
		},
		Mulch: MulchConfig{
			Enabled:     true,
			Domains:     []string{},
			PrimeFormat: "markdown",
		},
		Merge: MergeConfig{
			AIResolveEnabled: true,
			ReimagineEnabled: false,
		},
		Watchdog: WatchdogConfig{
			Tier0Enabled:        true,
			Tier0IntervalMs:     30000, // 30 seconds
			Tier1Enabled:        true,
			Tier2Enabled:        true,
			StaleThresholdMs:    300000, // 5 minutes
			ZombieThresholdMs:   600000, // 10 minutes
			NudgeIntervalMs:     60000,  // 1 minute
			RecoveryWaitTime:    5,
			EscapeKeyCount:      2,
			MaxRecoveryAttempts: 3,
		},
		Logging: LoggingConfig{
			Verbose:       false,
			RedactSecrets: true,
		},
	}
}

// Load reads a config.yaml file from the specified path and unmarshals it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Start with defaults, then override with loaded config
	conf := Default()
	if err := yaml.Unmarshal(data, conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set project root if not set
	if conf.Project.Root == "." || conf.Project.Root == "" {
		conf.Project.Root = filepath.Dir(path)
	}

	return conf, nil
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
	if c.Watchdog.Tier0IntervalMs > 0 {
		return time.Duration(c.Watchdog.Tier0IntervalMs) * time.Millisecond
	}
	if c.Watchdog.RecoveryWaitTime > 0 {
		return time.Duration(c.Watchdog.RecoveryWaitTime) * time.Second
	}
	return 30 * time.Second
}

// GetWatchdogStallTimeout returns the stall timeout with default fallback
func (c *Config) GetWatchdogStallTimeout() time.Duration {
	if c.Watchdog.StaleThresholdMs > 0 {
		return time.Duration(c.Watchdog.StaleThresholdMs) * time.Millisecond
	}
	return 5 * time.Minute
}

// GetWatchdogRecoveryWaitTime returns the recovery wait time with default fallback
func (c *Config) GetWatchdogRecoveryWaitTime() time.Duration {
	if c.Watchdog.NudgeIntervalMs > 0 {
		return time.Duration(c.Watchdog.NudgeIntervalMs) * time.Millisecond
	}
	if c.Watchdog.RecoveryWaitTime > 0 {
		return time.Duration(c.Watchdog.RecoveryWaitTime) * time.Second
	}
	return 5 * time.Second
}

// GetWatchdogEscapeKeyCount returns the number of escape keys to send with default fallback
func (c *Config) GetWatchdogEscapeKeyCount() int {
	if c.Watchdog.EscapeKeyCount > 0 {
		return c.Watchdog.EscapeKeyCount
	}
	return 2
}

// GetWatchdogMaxRecoveryAttempts returns the max recovery attempts with default fallback
func (c *Config) GetWatchdogMaxRecoveryAttempts() int {
	if c.Watchdog.MaxRecoveryAttempts > 0 {
		return c.Watchdog.MaxRecoveryAttempts
	}
	return 3
}

// GetZombieThreshold returns the zombie detection threshold with default fallback
func (c *Config) GetZombieThreshold() time.Duration {
	if c.Watchdog.ZombieThresholdMs > 0 {
		return time.Duration(c.Watchdog.ZombieThresholdMs) * time.Millisecond
	}
	return 10 * time.Minute
}

// GetPrimeFormat returns the output format for at prime command
func (c *Config) GetPrimeFormat() string {
	if c.Mulch.PrimeFormat != "" {
		return c.Mulch.PrimeFormat
	}
	return "markdown"
}

// IsReimagineEnabled returns whether reimagine merge strategy is enabled
func (c *Config) IsReimagineEnabled() bool {
	return c.Merge.ReimagineEnabled
}

// IsScoutEnabled returns whether Scout agent is enabled
func (c *Config) IsScoutEnabled() bool {
	return c.Agents.ScoutEnabled
}

// GetScoutWaitDuration returns the Scout wait timeout duration
func (c *Config) GetScoutWaitDuration() time.Duration {
	if c.Agents.ScoutWaitSec > 0 {
		return time.Duration(c.Agents.ScoutWaitSec) * time.Second
	}
	return 10 * time.Minute
}

// Save writes the current configuration to the specified path as YAML.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := "# Assistant-to Configuration\n" +
		"# See: https://github.com/assistant-to/assistant-to\n\n"

	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
