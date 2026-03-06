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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the assistant-to workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit() error {
	var (
		tool        string
		modelLarge  string
		modelMedium string
		modelFast   string
	)

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

	fmt.Println(headerStyle.Render("assistant-to: Workspace Setup"))
	fmt.Println(subHeaderStyle.Render("Welcome to the Managing Director's Autonomous Coding Swarm.\nLet's configure your agent tiers."))

	// Step 1: Tool Selection
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

	// Step 2: Model Selection based on Tool
	if tool == "gemini" {
		opts := []huh.Option[string]{
			huh.NewOption("Auto (Gemini 2.5)", "auto-2.5"),
			huh.NewOption("Auto (Gemini 3)", "auto-3"),
		}
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Large Model").
					Description("Capable of reasoning and orchestrating. Defaulted for Coordinator and Lead roles.").
					Options(opts...).
					Value(&modelLarge),
				huh.NewSelect[string]().
					Title("Select Medium Model").
					Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
					Options(opts...).
					Value(&modelMedium),
				huh.NewSelect[string]().
					Title("Select Fast Model").
					Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
					Options(opts...).
					Value(&modelFast),
			),
		).WithTheme(huh.ThemeCharm()).Run()
		if err != nil {
			return fmt.Errorf("model selection cancelled: %w", err)
		}
	} else if tool == "opencode" {
		// Attempt to fetch models from `opencode models`
		models, fetchErr := fetchOpencodeModels()
		if fetchErr == nil && len(models) > 0 {
			var opts []huh.Option[string]
			for _, m := range models {
				opts = append(opts, huh.NewOption(m, m))
			}
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Select Large Model").
						Description("Capable of reasoning and orchestrating. Defaulted for Coordinator and Lead roles.").
						Options(opts...).
						Value(&modelLarge),
					huh.NewSelect[string]().
						Title("Select Medium Model").
						Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
						Options(opts...).
						Value(&modelMedium),
					huh.NewSelect[string]().
						Title("Select Fast Model").
						Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
						Options(opts...).
						Value(&modelFast),
				),
			).WithTheme(huh.ThemeCharm()).Run()
		} else {
			// Fallback to text input
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Enter Large Model Name").
						Description("Capable of reasoning and orchestrating. Defaulted for Coordinator and Lead roles.").
						Value(&modelLarge),
					huh.NewInput().
						Title("Enter Medium Model Name").
						Description("Balanced size for iterative tasks. Defaulted for the Builder, Merger, and Reviewer roles.").
						Value(&modelMedium),
					huh.NewInput().
						Title("Enter Fast Model Name").
						Description("Optimized for speed and log volume. Used by Scout to do repository grepping.").
						Value(&modelFast),
				),
			).WithTheme(huh.ThemeCharm()).Run()
		}
		if err != nil {
			return fmt.Errorf("model selection cancelled: %w", err)
		}
	}

	fmt.Println()

	// Step 3: Scaffold Directory Structure
	baseDir := ".assistant-to"
	dirs := []string{
		filepath.Join(baseDir, "specs"),
		filepath.Join(baseDir, "worktrees"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
		fmt.Printf("%s Created directory: %s\n", successStyle.Render("✓"), infoStyle.Render(d))
	}

	// Step 4: Generate config.yaml
	conf := config.Config{
		Tool:        tool,
		ModelLarge:  modelLarge,
		ModelMedium: modelMedium,
		ModelFast:   modelFast,
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
