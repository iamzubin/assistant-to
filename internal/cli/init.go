package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"assistant-to/internal/config"
	"assistant-to/internal/db"

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
	Short: "Initialize the assistant-to workspace in the current directory",
	Long: `Creates the local .assistant-to directory structure, configures the environment, and initializes the state database.
This must be run once per project before launching the orchestrator or any agents.

For automated/non-interactive initialization, use flags:
  at init --tool=gemini --non-interactive
  at init --tool=opencode --project-name=myapp --max-agents=10`,
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
		modelLarge = "gemini-2.5-pro"
	}
	if modelMedium == "" {
		modelMedium = "gemini-2.5-flash"
	}
	if modelFast == "" {
		modelFast = "gemini-2.5-flash-lite"
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
		fmt.Println(headerStyle.Render("assistant-to: Workspace Setup"))
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
					huh.NewOption("gemini-2.5-pro", "gemini-2.5-pro"),
					huh.NewOption("gemini-2.5-flash", "gemini-2.5-flash"),
					huh.NewOption("gemini-2.5-flash-lite", "gemini-2.5-flash-lite"),
				}
				mediumOpts := []huh.Option[string]{
					huh.NewOption("gemini-2.5-flash", "gemini-2.5-flash"),
					huh.NewOption("gemini-2.5-pro", "gemini-2.5-pro"),
					huh.NewOption("gemini-2.5-flash-lite", "gemini-2.5-flash-lite"),
				}
				fastOpts := []huh.Option[string]{
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
	baseDir := ".assistant-to"
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
			ManifestPath:   ".assistant-to/agent-manifest.json",
			BaseDir:        ".assistant-to/agent-defs",
			MaxConcurrent:  initMaxAgents,
			StaggerDelayMs: 2000,
			MaxDepth:       2,
			ScoutEnabled:   initScoutEnabled,
			ScoutWaitSec:   600, // 10 minutes
		},
		Worktrees: config.WorktreesConfig{
			BaseDir: ".assistant-to/worktrees",
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

	// Step 6: Gitignore
	var addToGitignore bool = false
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add .assistant-to to .gitignore?").
				Description("Prevents your local workspace state from being tracked.").
				Value(&addToGitignore),
		),
	).WithTheme(huh.ThemeCharm()).Run()
	if err != nil {
		return fmt.Errorf("gitignore confirmation cancelled: %w", err)
	}

	if addToGitignore {
		if err := appendToGitignore(".assistant-to/"); err != nil {
			fmt.Printf("%s Failed to update .gitignore: %v\n", infoStyle.Render("!"), err)
		} else {
			fmt.Printf("%s Added .assistant-to/ to .gitignore\n", successStyle.Render("✓"))
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

// copyDefaultPrompts copies prompt files from the assistant-to binary location to the project
func copyDefaultPrompts(destDir string) error {
	// Try to find prompts in multiple locations
	possiblePaths := []string{
		// Check if we're running from the assistant-to repo
		filepath.Join("internal", "orchestrator", "prompts"),
		// Check relative to executable (for installed binary)
		filepath.Join("..", "..", "internal", "orchestrator", "prompts"),
		filepath.Join("/usr", "local", "share", "assistant-to", "prompts"),
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
		// assuming we might be in a subdirectory of the assistant-to repo
		cwd, _ := os.Getwd()
		possiblePaths = []string{
			filepath.Join(cwd, "..", "assistant-to", "internal", "orchestrator", "prompts"),
			filepath.Join(cwd, "..", "..", "assistant-to", "internal", "orchestrator", "prompts"),
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

	return nil
}

func appendToGitignore(entry string) error {
	content, err := os.ReadFile(".gitignore")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil // already exists
		}
	}

	f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	_, err = f.WriteString(entry + "\n")
	return err
}
