package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the user's project-level configuration.
type Config struct {
	Tool        string `yaml:"tool"`
	ModelLarge  string `yaml:"model_large"`
	ModelMedium string `yaml:"model_medium"`
	ModelFast   string `yaml:"model_fast"`
}

// Default returns a default configuration object.
func Default() *Config {
	return &Config{
		Tool:        "gemini",
		ModelLarge:  "gemini-3.1-pro-preview",
		ModelMedium: "gemini-3-flash-preview",
		ModelFast:   "gemini-2.5-flash",
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
