package config

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
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

// CleanupConfig holds cleanup settings for automatic agent/worktree cleanup
type CleanupConfig struct {
	Enabled        bool `yaml:"enabled"`        // Enable automatic cleanup
	IntervalMs     int  `yaml:"intervalMs"`     // How often to run cleanup check
	CompletedDelay int  `yaml:"completedDelay"` // Minutes to wait after task completion before cleanup
	OrphanTimeout  int  `yaml:"orphanTimeout"`  // Minutes to wait before cleaning up orphan sessions
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Verbose       bool `yaml:"verbose"`
	RedactSecrets bool `yaml:"redactSecrets"`
}

// AgentRuntimeConfig holds runtime and allowed tools for a specific agent role
type AgentRuntimeConfig struct {
	Runtime      string   `yaml:"runtime"`      // "gemini", "opencode", "cli"
	Model        string   `yaml:"model"`        // Model to use for this agent
	AllowedTools []string `yaml:"allowedTools"` // List of allowed tools: "mail", "log", "task", "buffer", "session", "worktree"
	MCPPort      int      `yaml:"mcpPort"`      // MCP server port for this agent
}

// APIConfig holds REST API server settings
type APIConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	MCPEnabled bool   `yaml:"mcpEnabled"`
	MCPPort    int    `yaml:"mcpPort"`
}

// OpenCodeAgent represents a custom OpenCode agent definition
type OpenCodeAgent struct {
	Name         string   `yaml:"name"`         // Agent name (filename without .md)
	Description  string   `yaml:"description"`  // Agent description
	Mode         string   `yaml:"mode"`         // "primary" or "subagent"
	Model        string   `yaml:"model"`        // Model to use
	Temperature  float64  `yaml:"temperature"`  // Temperature setting
	AllowedTools []string `yaml:"allowedTools"` // Tools permissions
	Instructions string   `yaml:"instructions"` // Agent instructions (markdown content)
	FilePath     string   `yaml:"filePath"`     // Source file path
	Scope        string   `yaml:"scope"`        // "project" or "global"
}

// GeminiSkill represents a Gemini skill definition
type GeminiSkill struct {
	Name        string `yaml:"name"`        // Skill name (directory name)
	Description string `yaml:"description"` // Skill description
	Path        string `yaml:"path"`        // Path to skill directory
	Scope       string `yaml:"scope"`       // "project" or "global"
}

// CustomAgentsConfig holds settings for custom OpenCode agents
type CustomAgentsConfig struct {
	Enabled    bool `yaml:"enabled"`    // Enable custom agents
	PerProject bool `yaml:"perProject"` // Enable per-project agents (.opencode/agents/)
	Global     bool `yaml:"global"`     // Enable global agents (~/.config/opencode/agents/)
}

// GeminiSkillsConfig holds settings for Gemini skills
type GeminiSkillsConfig struct {
	Enabled    bool `yaml:"enabled"`    // Enable Gemini skills
	PerProject bool `yaml:"perProject"` // Enable per-project skills (.gemini/skills/)
	Global     bool `yaml:"global"`     // Enable global skills (~/.gemini/skills/)
}

// GetProjectPorts returns project-specific ports based on the project path hash.
// This ensures multiple instances can run on the same machine without port conflicts.
func (c *Config) GetProjectPorts(projectPath string) (apiPort, mcpPort int) {
	basePort := 15000
	maxPort := 65000

	// Generate a deterministic hash from the project path
	hash := sha256.Sum256([]byte(projectPath))
	// Use first 4 bytes as an offset
	offset := int(binary.BigEndian.Uint32(hash[:4]))

	// Calculate ports in valid range
	portRange := maxPort - basePort
	apiPort = basePort + (offset % portRange)
	mcpPort = apiPort + 1

	// Ensure we don't exceed max port
	if mcpPort > maxPort {
		mcpPort = basePort
	}

	return apiPort, mcpPort
}

// Config represents the user's project-level configuration.
type Config struct {
	Tool         string                        `yaml:"tool"`
	ModelLarge   string                        `yaml:"model_large"`
	ModelMedium  string                        `yaml:"model_medium"`
	ModelFast    string                        `yaml:"model_fast"`
	LastModel    string                        `yaml:"last_model"`
	Project      ProjectConfig                 `yaml:"project"`
	Agents       AgentsConfig                  `yaml:"agents"`
	Worktrees    WorktreesConfig               `yaml:"worktrees"`
	TaskTracker  TaskTrackerConfig             `yaml:"taskTracker"`
	Mulch        MulchConfig                   `yaml:"mulch"`
	Merge        MergeConfig                   `yaml:"merge"`
	Watchdog     WatchdogConfig                `yaml:"watchdog"`
	Cleanup      CleanupConfig                 `yaml:"cleanup"`
	Logging      LoggingConfig                 `yaml:"logging"`
	API          APIConfig                     `yaml:"api"`
	AgentsRT     map[string]AgentRuntimeConfig `yaml:"agentsRuntime"` // Per-agent runtime config: coordinator, builder, scout, reviewer, merger
	CustomAgents CustomAgentsConfig            `yaml:"customAgents"`  // Custom OpenCode agents settings
	GeminiSkills GeminiSkillsConfig            `yaml:"geminiSkills"`  // Gemini skills settings
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
			ManifestPath:   ".dwight/agent-manifest.json",
			BaseDir:        ".dwight/agent-defs",
			MaxConcurrent:  5,
			StaggerDelayMs: 2000,
			MaxDepth:       2,
			ScoutEnabled:   true,
			ScoutWaitSec:   600, // 10 minutes
		},
		Worktrees: WorktreesConfig{
			BaseDir: ".dwight/worktrees",
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
		Cleanup: CleanupConfig{
			Enabled:        true,
			IntervalMs:     60000, // 1 minute
			CompletedDelay: 5,     // 5 minutes
			OrphanTimeout:  10,    // 10 minutes
		},
		Logging: LoggingConfig{
			Verbose:       false,
			RedactSecrets: true,
		},
		API: APIConfig{
			Enabled:    true,
			Host:       "127.0.0.1",
			Port:       8765,
			MCPEnabled: true,
			MCPPort:    8766,
		},
		AgentsRT: map[string]AgentRuntimeConfig{
			"coordinator": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-pro",
				AllowedTools: []string{"mail", "log", "task", "spawn", "buffer", "session", "cleanup", "worktree", "dash"},
				MCPPort:      0, // Use API.MCPPort (single MCP server for all roles)
			},
			"builder": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-flash",
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server for all roles)
			},
			"scout": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-flash",
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server for all roles)
			},
			"reviewer": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-flash",
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server for all roles)
			},
			"merger": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-flash",
				AllowedTools: []string{"mail", "log", "worktree", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server for all roles)
			},
			"geminitest": {
				Runtime:      "gemini",
				Model:        "gemini-2.5-flash",
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0,
			},
			"opencodetest": {
				Runtime:      "opencode",
				Model:        "",
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0,
			},
		},
		CustomAgents: CustomAgentsConfig{
			Enabled:    true,
			PerProject: true,
			Global:     true,
		},
		GeminiSkills: GeminiSkillsConfig{
			Enabled:    true,
			PerProject: true,
			Global:     true,
		},
	}
}

// Load reads a config.yaml file from the specified path and unmarshals it.
// After loading, it calculates and applies project-specific ports.
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

	// Calculate and apply project-specific ports for multi-instance support
	apiPort, mcpPort := conf.GetProjectPorts(conf.Project.Root)
	conf.API.Port = apiPort
	conf.API.MCPPort = mcpPort

	return conf, nil
}

// ModelForRole returns the appropriate model string for a given agent role.
// Roles: "Coordinator" -> large, "Scout" -> fast, all others -> medium.
func (c *Config) ModelForRole(role string) string {
	// Check for role-specific override first
	if rt, ok := c.AgentsRT[role]; ok && rt.Model != "" {
		return rt.Model
	}
	switch role {
	case "Coordinator":
		return c.ModelLarge
	case "Scout":
		return c.ModelFast
	default: // Builder, Reviewer, Merger
		return c.ModelMedium
	}
}

// RuntimeForRole returns the runtime (gemini/opencode/cli) for a given agent role.
func (c *Config) RuntimeForRole(role string) string {
	// Check for role-specific override first
	if rt, ok := c.AgentsRT[role]; ok && rt.Runtime != "" {
		return rt.Runtime
	}
	// Fall back to global tool setting
	if c.Tool != "" {
		return c.Tool
	}
	return "gemini"
}

// AllowedToolsForRole returns the list of allowed tools for a given agent role.
func (c *Config) AllowedToolsForRole(role string) []string {
	if rt, ok := c.AgentsRT[role]; ok && len(rt.AllowedTools) > 0 {
		return rt.AllowedTools
	}
	// Default allowed tools if not specified
	switch role {
	case "Coordinator":
		return []string{"mail", "log", "task", "spawn", "buffer", "session", "cleanup", "worktree", "dash"}
	case "Builder", "Scout", "Reviewer":
		return []string{"mail", "log", "buffer"}
	case "Merger":
		return []string{"mail", "log", "worktree", "buffer"}
	default:
		return []string{"mail", "log"}
	}
}

// MCPPortForRole returns the MCP port for a given agent role.
func (c *Config) MCPPortForRole(role string) int {
	if rt, ok := c.AgentsRT[role]; ok && rt.MCPPort > 0 {
		return rt.MCPPort
	}
	return c.API.MCPPort
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

// GetWorktreesDir returns the worktrees directory path
func (c *Config) GetWorktreesDir(projectRoot string) string {
	if c.Worktrees.BaseDir != "" {
		return filepath.Join(projectRoot, c.Worktrees.BaseDir)
	}
	return filepath.Join(projectRoot, ".dwight", "worktrees")
}

// GetAgentsBaseDir returns the agents base directory path
func (c *Config) GetAgentsBaseDir(projectRoot string) string {
	if c.Agents.BaseDir != "" {
		return filepath.Join(projectRoot, c.Agents.BaseDir)
	}
	return filepath.Join(projectRoot, ".dwight", "agent-defs")
}

// IsReimagineEnabled returns whether reimagine merge strategy is enabled
func (c *Config) IsReimagineEnabled() bool {
	return c.Merge.ReimagineEnabled
}

// IsAIResolveEnabled returns whether AI-assisted merge resolution is enabled
func (c *Config) IsAIResolveEnabled() bool {
	return c.Merge.AIResolveEnabled
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

// IsTaskTrackerEnabled returns whether task tracking features are enabled
// Note: Currently defined but not actively gating any functionality
func (c *Config) IsTaskTrackerEnabled() bool {
	return c.TaskTracker.Enabled
}

// IsMulchEnabled returns whether mulch/intelligence features are enabled
// Note: Currently defined but intelligence features are always available
func (c *Config) IsMulchEnabled() bool {
	return c.Mulch.Enabled
}

// GetMulchDomains returns the domains for mulch analysis
// Note: Currently defined but not actively used
func (c *Config) GetMulchDomains() []string {
	return c.Mulch.Domains
}

// IsCleanupEnabled returns whether automatic cleanup is enabled
func (c *Config) IsCleanupEnabled() bool {
	return c.Cleanup.Enabled
}

// GetCleanupInterval returns the cleanup check interval
func (c *Config) GetCleanupInterval() time.Duration {
	if c.Cleanup.IntervalMs <= 0 {
		return 60 * time.Second // default 1 minute
	}
	return time.Duration(c.Cleanup.IntervalMs) * time.Millisecond
}

// GetCleanupCompletedDelay returns the delay after task completion before cleanup
func (c *Config) GetCleanupCompletedDelay() time.Duration {
	if c.Cleanup.CompletedDelay <= 0 {
		return 5 * time.Minute // default 5 minutes
	}
	return time.Duration(c.Cleanup.CompletedDelay) * time.Minute
}

// GetCleanupOrphanTimeout returns the timeout for orphan session cleanup
func (c *Config) GetCleanupOrphanTimeout() time.Duration {
	if c.Cleanup.OrphanTimeout <= 0 {
		return 10 * time.Minute // default 10 minutes
	}
	return time.Duration(c.Cleanup.OrphanTimeout) * time.Minute
}

// Save writes the current configuration to the specified path as YAML.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := "# Assistant-to Configuration\n" +
		"# See: https://github.com/dwight/dwight\n\n"

	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func GetHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		return os.Getenv("HOME")
	}
	return usr.HomeDir
}

func parseAgentFile(path string) (*OpenCodeAgent, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	agent := &OpenCodeAgent{
		Name:         strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		FilePath:     path,
		Mode:         "subagent",
		Instructions: string(content),
	}

	lines := strings.Split(string(content), "\n")

	var frontmatter []string
	inFrontmatter := false
	bodyStart := 0
	hasFrontmatter := false

	for i, line := range lines {
		lineContent := strings.TrimSpace(line)
		if lineContent == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				hasFrontmatter = true
				continue
			} else {
				frontmatter = lines[1:i]
				bodyStart = i + 1
				break
			}
		}
	}

	if hasFrontmatter && len(frontmatter) > 0 {
		agent.Instructions = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))

		for _, line := range frontmatter {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "description:") {
				agent.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			} else if strings.HasPrefix(line, "mode:") {
				agent.Mode = strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			} else if strings.HasPrefix(line, "model:") {
				agent.Model = strings.TrimSpace(strings.TrimPrefix(line, "model:"))
			} else if strings.HasPrefix(line, "temperature:") {
				tempStr := strings.TrimSpace(strings.TrimPrefix(line, "temperature:"))
				if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
					agent.Temperature = temp
				}
			} else if strings.HasPrefix(line, "tools:") {
				toolsStr := strings.TrimSpace(strings.TrimPrefix(line, "tools:"))
				if toolsStr != "" {
					agent.AllowedTools = strings.Split(toolsStr, ",")
					for j := range agent.AllowedTools {
						agent.AllowedTools[j] = strings.TrimSpace(agent.AllowedTools[j])
					}
				}
			} else if strings.HasPrefix(line, "permission:") {
			} else if strings.HasPrefix(line, "prompt:") {
			} else if strings.HasPrefix(line, "steps:") {
			} else if strings.HasPrefix(line, "disable:") {
			} else if strings.HasPrefix(line, "hidden:") {
			} else if strings.HasPrefix(line, "top_p:") {
			} else if strings.HasPrefix(line, "color:") {
			}
		}
	} else if !hasFrontmatter {
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "description:") {
				agent.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			} else if strings.HasPrefix(line, "mode:") {
				agent.Mode = strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			} else if strings.HasPrefix(line, "model:") {
				agent.Model = strings.TrimSpace(strings.TrimPrefix(line, "model:"))
			} else if strings.HasPrefix(line, "temperature:") {
				tempStr := strings.TrimSpace(strings.TrimPrefix(line, "temperature:"))
				if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
					agent.Temperature = temp
				}
			} else if strings.HasPrefix(line, "tools:") {
				toolsStr := strings.TrimSpace(strings.TrimPrefix(line, "tools:"))
				agent.AllowedTools = strings.Split(toolsStr, ",")
				for j := range agent.AllowedTools {
					agent.AllowedTools[j] = strings.TrimSpace(agent.AllowedTools[j])
				}
			}
			if i > 10 {
				break
			}
		}
	}

	if agent.Instructions == "" {
		agent.Instructions = string(content)
	}

	return agent, nil
}

func parseSkillDir(path string) (*GeminiSkill, error) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, err
	}

	skill := &GeminiSkill{
		Name: filepath.Base(path),
		Path: path,
	}

	skillMDPath := filepath.Join(path, "SKILL.md")
	descriptionMDPath := filepath.Join(path, "description.md")

	var content string
	var description string

	if data, err := os.ReadFile(skillMDPath); err == nil {
		content = string(data)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			description = strings.TrimSpace(lines[0])
			description = strings.TrimPrefix(description, "# ")
			description = strings.TrimPrefix(description, "## ")
			description = strings.TrimPrefix(description, "### ")
			if len(description) > 200 {
				description = description[:200]
			}
		}
	} else if data, err := os.ReadFile(descriptionMDPath); err == nil {
		content = string(data)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			description = strings.TrimSpace(lines[0])
			description = strings.TrimPrefix(description, "# ")
			description = strings.TrimPrefix(description, "## ")
			description = strings.TrimPrefix(description, "### ")
			if len(description) > 200 {
				description = description[:200]
			}
		}
	}

	skill.Description = description

	return skill, nil
}

func (c *Config) IsCustomAgentsEnabled() bool {
	return c.CustomAgents.Enabled
}

func (c *Config) IsCustomAgentsPerProject() bool {
	return c.CustomAgents.Enabled && c.CustomAgents.PerProject
}

func (c *Config) IsCustomAgentsGlobal() bool {
	return c.CustomAgents.Enabled && c.CustomAgents.Global
}

func (c *Config) IsGeminiSkillsEnabled() bool {
	return c.GeminiSkills.Enabled
}

func (c *Config) IsGeminiSkillsPerProject() bool {
	return c.GeminiSkills.Enabled && c.GeminiSkills.PerProject
}

func (c *Config) IsGeminiSkillsGlobal() bool {
	return c.GeminiSkills.Enabled && c.GeminiSkills.Global
}

func (c *Config) DiscoverOpenCodeAgents(projectRoot string) []OpenCodeAgent {
	var agents []OpenCodeAgent

	if c.IsCustomAgentsPerProject() {
		projectAgentsPath := filepath.Join(projectRoot, ".opencode", "agents")
		if entries, err := os.ReadDir(projectAgentsPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				agentPath := filepath.Join(projectAgentsPath, entry.Name())
				if agent, err := parseAgentFile(agentPath); err == nil {
					agent.Scope = "project"
					agents = append(agents, *agent)
				}
			}
		}
	}

	if c.IsCustomAgentsGlobal() {
		globalAgentsPath := filepath.Join(GetHomeDir(), ".config", "opencode", "agents")
		if entries, err := os.ReadDir(globalAgentsPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				agentPath := filepath.Join(globalAgentsPath, entry.Name())
				if agent, err := parseAgentFile(agentPath); err == nil {
					agent.Scope = "global"
					agents = append(agents, *agent)
				}
			}
		}
	}

	return agents
}

func (c *Config) DiscoverGeminiSkills(projectRoot string) []GeminiSkill {
	var skills []GeminiSkill

	if c.IsGeminiSkillsPerProject() {
		projectSkillsPath := filepath.Join(projectRoot, ".gemini", "skills")
		if entries, err := os.ReadDir(projectSkillsPath); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				skillPath := filepath.Join(projectSkillsPath, entry.Name())
				if skill, err := parseSkillDir(skillPath); err == nil {
					skill.Scope = "project"
					skills = append(skills, *skill)
				}
			}
		}
	}

	if c.IsGeminiSkillsGlobal() {
		globalSkillsPath := filepath.Join(GetHomeDir(), ".gemini", "skills")
		if entries, err := os.ReadDir(globalSkillsPath); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				skillPath := filepath.Join(globalSkillsPath, entry.Name())
				if skill, err := parseSkillDir(skillPath); err == nil {
					skill.Scope = "global"
					skills = append(skills, *skill)
				}
			}
		}
	}

	return skills
}
