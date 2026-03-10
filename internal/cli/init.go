package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dwight/internal/config"
	"dwight/internal/db"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	initTool           string
	initModelLarge     string
	initModelMedium    string
	initModelFast      string
	initProjectName    string
	initBranch         string
	initMaxAgents      int
	initScoutEnabled   bool
	initNonInteractive bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the dwight workspace in the current directory",
	Long: `Creates the local .dwight directory structure, configures the environment, and initializes the state database.
This must be run once per project before launching the orchestrator or any agents.

For automated/non-interactive initialization, use flags:
  dwight init --tool=gemini --non-interactive
  dwight init --tool=opencode --project-name=myapp --max-agents=10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	RootCmd.AddCommand(initCmd)

	// Tool and model flags
	initCmd.Flags().StringVar(&initTool, "tool", "", "Orchestrator tool (gemini, opencode)")
	initCmd.Flags().StringVar(&initModelLarge, "model-large", "", "Large model for Coordinator")
	initCmd.Flags().StringVar(&initModelMedium, "model-medium", "", "Medium model for Builder/Merger/Reviewer")
	initCmd.Flags().StringVar(&initModelFast, "model-fast", "", "Fast model for Scout")

	// Project flags
	initCmd.Flags().StringVar(&initProjectName, "project-name", "", "Project name")
	initCmd.Flags().StringVar(&initBranch, "branch", "main", "Canonical branch name")

	// Agent flags
	initCmd.Flags().IntVar(&initMaxAgents, "max-agents", 5, "Maximum concurrent agents")
	initCmd.Flags().BoolVar(&initScoutEnabled, "scout", true, "Enable Scout agent")

	// Automation flag
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "Skip interactive prompts (use with other flags)")
}

func runInit() error {
	// Use flag values or defaults
	tool := initTool
	modelLarge := initModelLarge
	modelMedium := initModelMedium
	modelFast := initModelFast

	// Set defaults if not provided
	if tool == "" {
		tool = "gemini"
	}
	if modelLarge == "" {
		modelLarge = "auto"
	}
	if modelMedium == "" {
		modelMedium = "auto"
	}
	if modelFast == "" {
		modelFast = "auto"
	}

	// Lipgloss UI Styles
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		PaddingTop(1).
		PaddingBottom(1).
		PaddingLeft(4).
		PaddingRight(4).
		MarginBottom(1)

	subHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A8A8A8")).
		MarginBottom(1)

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))

	// Interactive mode (unless --non-interactive flag is set)
	if !initNonInteractive {
		fmt.Println(headerStyle.Render("dwight: Workspace Setup"))
		fmt.Println(subHeaderStyle.Render("Welcome to the Managing Director's Autonomous Coding Swarm.\nLet's configure your agent tiers."))

		// Step 1: Tool Selection (if not provided via flag)
		if initTool == "" {
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Select Orchestrator Tool").
						Description("Choose the underpinning execution environment for the agents.").
						Options(
							huh.NewOption("gemini", "gemini"),
							huh.NewOption("opencode", "opencode"),
						).
						Value(&tool),
				),
			).WithTheme(huh.ThemeCharm()).Run()
			if err != nil {
				return fmt.Errorf("tool selection cancelled: %w", err)
			}
		}

		// Step 2: Model Selection based on Tool (if not provided via flags)
		if initModelLarge == "" || initModelMedium == "" || initModelFast == "" {
			if tool == "gemini" {
				largeOpts := []huh.Option[string]{
					huh.NewOption("auto (use last used or default)", "auto"),
					huh.NewOption("gemini-2.5-pro", "gemini-2.5-pro"),
					huh.NewOption("gemini-2.5-flash", "gemini-2.5-flash"),
					huh.NewOption("gemini-2.5-flash-lite", "gemini-2.5-flash-lite"),
				}
				mediumOpts := []huh.Option[string]{
					huh.NewOption("auto (use last used or default)", "auto"),
					huh.NewOption("gemini-2.5-flash", "gemini-2.5-flash"),
					huh.NewOption("gemini-2.5-pro", "gemini-2.5-pro"),
					huh.NewOption("gemini-2.5-flash-lite", "gemini-2.5-flash-lite"),
				}
				fastOpts := []huh.Option[string]{
					huh.NewOption("auto (use last used or default)", "auto"),
					huh.NewOption("gemini-2.5-flash-lite", "gemini-2.5-flash-lite"),
					huh.NewOption("gemini-2.5-flash", "gemini-2.5-flash"),
				}

				// Only prompt for models that weren't provided via flags
				var groups []*huh.Group
				if initModelLarge == "" {
					groups = append(groups, huh.NewGroup(
						huh.NewSelect[string]().
							Title("Select Large Model").
							Description("Capable of reasoning and orchestrating. Defaulted for Coordinator roles.").
							Options(largeOpts...).
							Value(&modelLarge),
					))
				}
				if initModelMedium == "" {
					groups = append(groups, huh.NewGroup(
						huh.NewSelect[string]().
							Title("Select Medium Model").
							Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
							Options(mediumOpts...).
							Value(&modelMedium),
					))
				}
				if initModelFast == "" {
					groups = append(groups, huh.NewGroup(
						huh.NewSelect[string]().
							Title("Select Fast Model").
							Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
							Options(fastOpts...).
							Value(&modelFast),
					))
				}

				if len(groups) > 0 {
					form := huh.NewForm(groups...).WithTheme(huh.ThemeCharm())
					if err := form.Run(); err != nil {
						return fmt.Errorf("model selection cancelled: %w", err)
					}
				}
			} else if tool == "opencode" {
				// Attempt to fetch models from `opencode models`
				models, fetchErr := fetchOpencodeModels()
				if fetchErr == nil && len(models) > 0 && (initModelLarge == "" || initModelMedium == "" || initModelFast == "") {
					var opts []huh.Option[string]
					opts = append(opts, huh.NewOption("auto (use last used or default)", "auto"))
					for _, m := range models {
						opts = append(opts, huh.NewOption(m, m))
					}

					var groups []*huh.Group
					if initModelLarge == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewSelect[string]().
								Title("Select Large Model").
								Description("Capable of reasoning and orchestrating. Defaulted for Coordinator roles.").
								Options(opts...).
								Value(&modelLarge),
						))
					}
					if initModelMedium == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewSelect[string]().
								Title("Select Medium Model").
								Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
								Options(opts...).
								Value(&modelMedium),
						))
					}
					if initModelFast == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewSelect[string]().
								Title("Select Fast Model").
								Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
								Options(opts...).
								Value(&modelFast),
						))
					}

					if len(groups) > 0 {
						form := huh.NewForm(groups...).WithTheme(huh.ThemeCharm())
						if err := form.Run(); err != nil {
							return fmt.Errorf("model selection cancelled: %w", err)
						}
					}
				} else if initModelLarge == "" || initModelMedium == "" || initModelFast == "" {
					// Fallback to text input for missing models
					var groups []*huh.Group
					if initModelLarge == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewInput().
								Title("Enter Large Model Name").
								Description("Capable of reasoning and orchestrating. Defaulted for Coordinator roles.").
								Value(&modelLarge),
						))
					}
					if initModelMedium == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewInput().
								Title("Enter Medium Model Name").
								Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
								Value(&modelMedium),
						))
					}
					if initModelFast == "" {
						groups = append(groups, huh.NewGroup(
							huh.NewInput().
								Title("Enter Fast Model Name").
								Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
								Value(&modelFast),
						))
					}

					if len(groups) > 0 {
						form := huh.NewForm(groups...).WithTheme(huh.ThemeCharm())
						if err := form.Run(); err != nil {
							return fmt.Errorf("model selection cancelled: %w", err)
						}
					}
				}
			}
		}

		fmt.Println()
	}

	fmt.Println()

	// Step 3: Scaffold Directory Structure
	baseDir := ".dwight"
	dirs := []string{
		filepath.Join(baseDir, "specs"),
		filepath.Join(baseDir, "worktrees"),
		filepath.Join(baseDir, "prompts"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
		fmt.Printf("%s Created directory: %s\n", successStyle.Render("✓"), infoStyle.Render(d))
	}

	// Step 3b: Copy default prompts
	if err := copyDefaultPrompts(filepath.Join(baseDir, "prompts")); err != nil {
		fmt.Printf("%s Warning: failed to copy default prompts: %v\n", infoStyle.Render("!"), err)
	} else {
		fmt.Printf("%s Copied default prompts to: %s\n", successStyle.Render("✓"), infoStyle.Render(filepath.Join(baseDir, "prompts")))
	}

	// Step 4: Generate config.yaml
	// Get project name from flag or directory
	projectName := initProjectName
	if projectName == "" {
		cwd, _ := os.Getwd()
		projectName = filepath.Base(cwd)
	}

	conf := config.Config{
		Tool:        tool,
		ModelLarge:  modelLarge,
		ModelMedium: modelMedium,
		ModelFast:   modelFast,
		Project: config.ProjectConfig{
			Name:            projectName,
			Root:            ".",
			CanonicalBranch: initBranch,
		},
		Agents: config.AgentsConfig{
			ManifestPath:   ".dwight/agent-manifest.json",
			BaseDir:        ".dwight/agent-defs",
			MaxConcurrent:  initMaxAgents,
			StaggerDelayMs: 2000,
			MaxDepth:       2,
			ScoutEnabled:   initScoutEnabled,
			ScoutWaitSec:   600, // 10 minutes
		},
		Worktrees: config.WorktreesConfig{
			BaseDir: ".dwight/worktrees",
		},
		TaskTracker: config.TaskTrackerConfig{
			Enabled: true,
		},
		Mulch: config.MulchConfig{
			Enabled:     true,
			Domains:     []string{},
			PrimeFormat: "markdown",
		},
		Merge: config.MergeConfig{
			AIResolveEnabled: true,
			ReimagineEnabled: false,
		},
		Watchdog: config.WatchdogConfig{
			Tier0Enabled:        true,
			Tier0IntervalMs:     30000,
			Tier1Enabled:        true,
			Tier2Enabled:        true,
			StaleThresholdMs:    300000,
			ZombieThresholdMs:   600000,
			NudgeIntervalMs:     60000,
			RecoveryWaitTime:    5,
			EscapeKeyCount:      2,
			MaxRecoveryAttempts: 3,
		},
		Logging: config.LoggingConfig{
			Verbose:       false,
			RedactSecrets: true,
		},
		API: config.APIConfig{
			Enabled:    true,
			Host:       "127.0.0.1",
			Port:       8765,
			MCPEnabled: true,
			MCPPort:    8766,
		},
		AgentsRT: map[string]config.AgentRuntimeConfig{
			"coordinator": {
				Runtime:      tool,
				Model:        modelLarge,
				AllowedTools: []string{"mail", "log", "task", "spawn", "buffer", "session", "cleanup", "worktree", "dash"},
				MCPPort:      0, // Use API.MCPPort (single MCP server)
			},
			"builder": {
				Runtime:      tool,
				Model:        modelMedium,
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server)
			},
			"scout": {
				Runtime:      tool,
				Model:        modelFast,
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server)
			},
			"reviewer": {
				Runtime:      tool,
				Model:        modelMedium,
				AllowedTools: []string{"mail", "log", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server)
			},
			"merger": {
				Runtime:      tool,
				Model:        modelMedium,
				AllowedTools: []string{"mail", "log", "worktree", "buffer"},
				MCPPort:      0, // Use API.MCPPort (single MCP server)
			},
		},
	}
	configPath := filepath.Join(baseDir, "config.yaml")
	if err := conf.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("%s Generated config file: %s\n", successStyle.Render("✓"), infoStyle.Render(configPath))

	// Step 5: Initialize SQLite DB
	dbPath := filepath.Join(baseDir, "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}
	fmt.Printf("%s Initialized state database: %s\n", successStyle.Render("✓"), infoStyle.Render(dbPath))

	// Step 6: Generate MCP config files for tools
	if err := generateMCPConfigs(baseDir, &conf); err != nil {
		fmt.Printf("%s Warning: failed to generate MCP configs: %v\n", infoStyle.Render("!"), err)
	} else {
		fmt.Printf("%s Generated MCP configuration files\n", successStyle.Render("✓"))
	}

	// Step 7: Create example OpenCode agents and Gemini skills
	if err := createExampleAgentsAndSkills(); err != nil {
		fmt.Printf("%s Warning: failed to create example agents/skills: %v\n", infoStyle.Render("!"), err)
	} else {
		fmt.Printf("%s Created example agents and skills\n", successStyle.Render("✓"))
	}

	// Step 8: Gitignore
	var addToGitignore bool = false
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add .dwight to .gitignore?").
				Description("Prevents your local workspace state from being tracked.").
				Value(&addToGitignore),
		),
	).WithTheme(huh.ThemeCharm()).Run()
	if err != nil {
		return fmt.Errorf("gitignore confirmation cancelled: %w", err)
	}

	gitignoreEntries := []string{
		".dwight/",
		"mcp.json",
		"opencode.json",
		".gemini/",
		".opencode/",
		"mcp-configs/",
	}

	if addToGitignore {
		if err := appendMultipleToGitignore(gitignoreEntries); err != nil {
			fmt.Printf("%s Failed to update .gitignore: %v\n", infoStyle.Render("!"), err)
		} else {
			fmt.Printf("%s Added entries to .gitignore\n", successStyle.Render("✓"))
		}
	}

	fmt.Println(successStyle.Render("\nInitialization complete. Your workspace is ready."))
	return nil
}

func fetchOpencodeModels() ([]string, error) {
	cmd := exec.Command("opencode", "models")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var models []string
	for _, line := range lines {
		// Clean up the output to just get model names if needed
		// Depending on `opencode models` exact output format
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			models = append(models, trimmed)
		}
	}
	return models, nil
}

// copyDefaultPrompts copies prompt files from the dwight binary location to the project
func copyDefaultPrompts(destDir string) error {
	// Try to find prompts in multiple locations
	possiblePaths := []string{
		// Check if we're running from the dwight repo
		filepath.Join("internal", "orchestrator", "prompts"),
		// Check relative to executable (for installed binary)
		filepath.Join("..", "..", "internal", "orchestrator", "prompts"),
		filepath.Join("/usr", "local", "share", "dwight", "prompts"),
	}

	var sourceDir string
	for _, path := range possiblePaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			sourceDir = path
			break
		}
	}

	if sourceDir == "" {
		// Last resort: try to find prompts relative to current working directory
		// assuming we might be in a subdirectory of the dwight repo
		cwd, _ := os.Getwd()
		possiblePaths = []string{
			filepath.Join(cwd, "..", "dwight", "internal", "orchestrator", "prompts"),
			filepath.Join(cwd, "..", "..", "dwight", "internal", "orchestrator", "prompts"),
		}
		for _, path := range possiblePaths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				sourceDir = path
				break
			}
		}
	}

	if sourceDir == "" {
		return fmt.Errorf("could not find source prompts directory")
	}

	// Copy all .md files
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read source prompts: %w", err)
	}

	copied := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		srcPath := filepath.Join(sourceDir, entry.Name())
		dstPath := filepath.Join(destDir, entry.Name())

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", entry.Name(), err)
		}
		copied++
	}

	if copied == 0 {
		return fmt.Errorf("no prompt files found to copy")
	}

	// Copy MCP subdirectory
	mcpSourceDir := filepath.Join(sourceDir, "mcp")
	mcpDestDir := filepath.Join(destDir, "mcp")

	if info, err := os.Stat(mcpSourceDir); err == nil && info.IsDir() {
		if err := os.MkdirAll(mcpDestDir, 0755); err != nil {
			return fmt.Errorf("failed to create mcp prompts directory: %w", err)
		}

		mcpEntries, err := os.ReadDir(mcpSourceDir)
		if err != nil {
			return fmt.Errorf("failed to read mcp prompts: %w", err)
		}

		for _, entry := range mcpEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			srcPath := filepath.Join(mcpSourceDir, entry.Name())
			dstPath := filepath.Join(mcpDestDir, entry.Name())

			content, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("failed to read mcp %s: %w", entry.Name(), err)
			}

			if err := os.WriteFile(dstPath, content, 0644); err != nil {
				return fmt.Errorf("failed to write mcp %s: %w", entry.Name(), err)
			}
			copied++
		}
	}

	return nil
}

// generateMCPConfigs creates MCP configuration files for opencode and gemini
func generateMCPConfigs(baseDir string, cfg *config.Config) error {
	mcpDir := filepath.Join(baseDir, "mcp-configs")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		return fmt.Errorf("failed to create mcp-configs directory: %w", err)
	}

	// Get project root (parent of .dwight)
	projectRoot := filepath.Dir(baseDir)

	// Use absolute path for the command to ensure it works from any directory
	absDwightPath, err := filepath.Abs(filepath.Join(projectRoot, "dwight"))
	if err != nil {
		absDwightPath = "dwight" // Fallback
	}

	// Generate coordinator MCP configs in PROJECT ROOT (not .dwight)
	// Use env vars - the server will set these when starting
	// This allows dynamic port assignment per project instance

	// 1. opencode.json for opencode (correct format per docs)
	opencodeConfig := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]interface{}{
			"dwight": map[string]interface{}{
				"type":    "local",
				"command": []string{absDwightPath, "mcp", "serve"},
				"enabled": true,
				"environment": map[string]string{
					"AT_MCP_PORT":     fmt.Sprintf("%d", cfg.API.MCPPort),
					"AT_AGENT_ROLE":   "coordinator",
					"AT_PROJECT_ROOT": projectRoot,
				},
			},
		},
	}

	opencodePath := filepath.Join(projectRoot, "opencode.json")
	opencodeData, err := json.MarshalIndent(opencodeConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal coordinator opencode config: %w", err)
	}
	if err := os.WriteFile(opencodePath, opencodeData, 0644); err != nil {
		return fmt.Errorf("failed to write coordinator opencode config: %w", err)
	}

	// 2. .gemini/settings.json for Gemini CLI
	geminiConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"dwight": map[string]interface{}{
				"command": absDwightPath,
				"args":    []string{"mcp", "serve"},
				"env": map[string]string{
					"AT_MCP_PORT":     fmt.Sprintf("%d", cfg.API.MCPPort),
					"AT_AGENT_ROLE":   "coordinator",
					"AT_PROJECT_ROOT": projectRoot,
				},
				"trust": true,
			},
		},
	}

	geminiDir := filepath.Join(projectRoot, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return fmt.Errorf("failed to create .gemini directory: %w", err)
	}
	geminiPath := filepath.Join(geminiDir, "settings.json")
	geminiData, err := json.MarshalIndent(geminiConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal coordinator gemini config: %w", err)
	}
	if err := os.WriteFile(geminiPath, geminiData, 0644); err != nil {
		return fmt.Errorf("failed to write coordinator gemini config: %w", err)
	}

	// 3. mcp.json generic config
	mcpConfig := map[string]interface{}{
		"name":        "dwight",
		"description": "Assistant-to Coordinator MCP server",
		"transport":   "stdio",
		"command":     absDwightPath,
		"args":        []string{"mcp", "serve"},
		"env": map[string]string{
			"AT_MCP_PORT":     fmt.Sprintf("%d", cfg.API.MCPPort),
			"AT_AGENT_ROLE":   "coordinator",
			"AT_PROJECT_ROOT": projectRoot,
		},
	}

	mcpPath := filepath.Join(projectRoot, "mcp.json")
	mcpData, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal coordinator mcp config: %w", err)
	}
	if err := os.WriteFile(mcpPath, mcpData, 0644); err != nil {
		return fmt.Errorf("failed to write coordinator mcp config: %w", err)
	}

	// 4. Template configs in mcp-configs directory (for reference/copying)
	templateConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"dwight": map[string]interface{}{
				"command": "dwight",
				"args":    []string{"mcp", "serve"},
				"env": map[string]string{
					"AT_MCP_PORT": "{{PORT}}",
				},
			},
		},
	}

	templatePath := filepath.Join(mcpDir, "template-mcp.json")
	templateData, _ := json.MarshalIndent(templateConfig, "", "  ")
	if err := os.WriteFile(templatePath, templateData, 0644); err != nil {
		return fmt.Errorf("failed to write template mcp config: %w", err)
	}

	// 5. Create README
	readmeContent := "# MCP Configuration Files\n\n" +
		"This directory contains MCP (Model Context Protocol) configuration templates.\n\n" +
		"## Generated Files (in project root)\n\n" +
		"The following files have been created in your PROJECT ROOT:\n" +
		"- **opencode.json**: Opencode MCP config for the Coordinator\n" +
		"- **.gemini/settings.json**: Gemini CLI MCP config\n" +
		"- **mcp.json**: Generic MCP config\n\n" +
		"## Usage\n\n" +
		"### Opencode\n\n" +
		"The opencode.json in your project root configures opencode to connect to the dwight MCP server.\n" +
		"Opencode automatically reads this file when run from the project directory.\n\n" +
		"### Gemini CLI\n\n" +
		"The .gemini/settings.json in your project root configures Gemini CLI to connect to the dwight MCP server.\n" +
		"Gemini CLI automatically reads this file when run from the project directory.\n\n" +
		"## Agent Roles\n\n" +
		"- **Coordinator** (project root): Full access - runs `dwight up`\n" +
		"- **Builder/Scout/Reviewer/Merger** (worktrees): Limited access - spawned by coordinator\n\n" +
		"Each worktree gets its own MCP config when an agent is spawned.\n"

	readmePath := filepath.Join(mcpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to write mcp readme: %w", err)
	}

	return nil
}

func appendMultipleToGitignore(entries []string) error {
	content, err := os.ReadFile(".gitignore")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	existing := make(map[string]bool)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		existing[strings.TrimSpace(line)] = true
	}

	f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	contentStr := string(content)
	for _, entry := range entries {
		if existing[entry] {
			continue
		}
		if len(contentStr) > 0 && contentStr[len(contentStr)-1] != '\n' {
			f.WriteString("\n")
		}
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
		contentStr += "\n" + entry
		existing[entry] = true
	}
	return nil
}

func createExampleAgentsAndSkills() error {
	if err := createExampleOpenCodeAgents(); err != nil {
		return fmt.Errorf("failed to create example agents: %w", err)
	}
	if err := createExampleGeminiSkills(); err != nil {
		return fmt.Errorf("failed to create example skills: %w", err)
	}
	return nil
}

func createExampleOpenCodeAgents() error {
	agentsDir := ".opencode/agents"
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	agents := []struct {
		filename string
		content  string
	}{
		{
			filename: "builder.md",
			content: `---
description: Implements features and fixes based on task specifications
mode: subagent
tools: mail,log,buffer
---

You are a Builder agent. Your role is to implement code changes to complete tasks.

## Guidelines

- Understand the task requirements from the specification
- Implement clean, maintainable code
- Follow existing code patterns and conventions
- Write tests for new functionality
- Run build and test commands to verify changes
- Commit your work when complete

## Workflow

1. Read the task specification
2. Implement the required changes
3. Write tests
4. Verify with build/test commands
5. Report completion`,
		},
		{
			filename: "merger.md",
			content: `---
description: Merges completed work into the main branch and resolves conflicts
mode: subagent
tools: mail,log,worktree,buffer
---

You are a Merger agent. Your role is to integrate completed work into the main branch.

## Guidelines

- Review completed work in the task worktree
- Ensure all tests pass
- Resolve any merge conflicts
- Run final verification before merging
- Clean up worktree after successful merge
- Use clear commit messages

## Workflow

1. Review the completed work
2. Fetch latest main branch
3. Merge or rebase as needed
4. Resolve conflicts if any
5. Run tests to verify
6. Push changes
7. Clean up worktree`,
		},
		{
			filename: "coordinator.md",
			content: `---
description: Orchestrates multiple agents to complete complex tasks
mode: subagent
tools: mail,log,task,spawn,buffer,session,cleanup,worktree,dash
---

You are a Coordinator agent. Your role is to orchestrate task completion by managing sub-agents.

## Guidelines

- Break down complex tasks into smaller sub-tasks
- Assign appropriate agents to each sub-task
- Monitor progress and handle failures
- Coordinate dependencies between tasks
- Ensure overall quality and consistency

## Workflow

1. Analyze the task and create a plan
2. Create sub-tasks for independent work
3. Spawn appropriate agents for each sub-task
4. Monitor progress and intervene if needed
5. Aggregate results
6. Verify complete task meets requirements`,
		},
		{
			filename: "scout.md",
			content: `---
description: Explores codebase to find relevant code, patterns, and context
mode: subagent
tools: mail,log,buffer
---

You are a Scout agent. Your role is to explore the codebase and gather context.

## Guidelines

- Search for relevant code and files
- Identify patterns and conventions
- Find related tests and documentation
- Provide concise, relevant findings
- Use efficient search strategies

## Techniques

- Use grep/find to locate code patterns
- Read relevant files to understand context
- Identify file structure and organization
- Find related tests and examples`,
		},
		{
			filename: "reviewer.md",
			content: `---
description: Reviews code changes for quality, bugs, and best practices
mode: subagent
tools: mail,log,buffer
---

You are a Reviewer agent. Your role is to carefully review code changes and provide feedback.

## Guidelines

- Review code for bugs, security issues, and performance
- Check for adherence to coding standards
- Ensure proper error handling
- Verify tests are adequate
- Provide specific, actionable feedback

## Output Format

1. Summary of changes
2. Issues found (severity: critical/major/minor)
3. Suggestions for improvement
4. Approval or request changes`,
		},
	}

	for _, agent := range agents {
		path := filepath.Join(agentsDir, agent.filename)
		if err := os.WriteFile(path, []byte(agent.content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", agent.filename, err)
		}
	}

	return nil
}

func createExampleGeminiSkills() error {
	skillsDir := ".gemini/skills"
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	skills := []struct {
		dirname string
		skillMD string
	}{
		{
			dirname: "builder",
			skillMD: `# Builder Skill

Implements features and fixes based on task specifications.

## Workflow

1. Understand task requirements from specification
2. Implement clean, maintainable code
3. Follow existing code patterns and conventions
4. Write tests for new functionality
5. Run build and test commands to verify changes
6. Report completion

## Guidelines

- Use appropriate frameworks and libraries
- Keep functions focused and small
- Add comments for complex logic
- Ensure error handling`,
		},
		{
			dirname: "merger",
			skillMD: `# Merger Skill

Merges completed work into the main branch.

## Workflow

1. Review completed work in task worktree
2. Fetch latest main branch
3. Merge or rebase as needed
4. Resolve conflicts if any
5. Run tests to verify
6. Push changes
7. Clean up worktree

## Guidelines

- Verify all tests pass before merging
- Write clear commit messages
- Handle merge conflicts carefully
- Ensure no regressions`,
		},
		{
			dirname: "coordinator",
			skillMD: `# Coordinator Skill

Orchestrates multiple agents to complete complex tasks.

## Workflow

1. Analyze task and create a plan
2. Break into smaller sub-tasks
3. Spawn appropriate agents for each
4. Monitor progress
5. Aggregate results
6. Verify complete task

## Guidelines

- Assign correct agents to sub-tasks
- Handle dependencies between tasks
- Monitor for failures
- Ensure quality across all work`,
		},
		{
			dirname: "scout",
			skillMD: `# Scout Skill

Explores codebase to find relevant code and context.

## Techniques

- Use grep/find to locate patterns
- Read relevant files for context
- Identify file structure
- Find related tests

## Guidelines

- Be thorough in search
- Provide concise findings
- Include file paths and line numbers
- Identify patterns and conventions`,
		},
		{
			dirname: "reviewer",
			skillMD: `# Reviewer Skill

Reviews code changes for quality and correctness.

## Checklist

- Bug detection
- Security issues
- Performance problems
- Code style adherence
- Error handling
- Test coverage

## Output

1. Summary of changes
2. Issues by severity
3. Suggestions
4. Approval status`,
		},
		{
			dirname: "debug",
			skillMD: `# Debug Helper

Helps diagnose and fix bugs.

## Capabilities

- Analyze error messages and stack traces
- Suggest potential causes
- Recommend debugging strategies
- Create minimal reproduction cases`,
		},
		{
			dirname: "refactor",
			skillMD: `# Refactoring Assistant

Improves code structure and design.

## Guidelines

- Identify code smells
- Suggest refactoring opportunities
- Maintain existing behavior
- Prioritize readability`,
		},
	}

	for _, skill := range skills {
		skillPath := filepath.Join(skillsDir, skill.dirname)
		if err := os.MkdirAll(skillPath, 0755); err != nil {
			return fmt.Errorf("failed to create skill directory %s: %w", skill.dirname, err)
		}
		skillFilePath := filepath.Join(skillPath, "SKILL.md")
		if err := os.WriteFile(skillFilePath, []byte(skill.skillMD), 0644); err != nil {
			return fmt.Errorf("failed to write SKILL.md for %s: %w", skill.dirname, err)
		}
	}

	return nil
}
