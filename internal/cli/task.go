package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistant-to/internal/db"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks for the autonomous coding swarm",
	Long:  `Parent command for task management. Use subcommands like "add" to interact with the task queue.`,
}

var taskAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Interactively add a new task to the queue",
	Long:  `Presents an interactive form to define a new workload for the agents, writing the resulting specification to the .assistant-to/specs directory and enqueueing it in the state database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTaskAdd()
	},
}

func init() {
	taskCmd.AddCommand(taskAddCmd)
	rootCmd.AddCommand(taskCmd)
}

func runTaskAdd() error {
	var (
		title       string
		description string
		difficulty  string
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

	fmt.Println(headerStyle.Render("assistant-to: New Task"))
	fmt.Println(subHeaderStyle.Render("Define a new work item for the autonomous coding swarm."))

	// Interactive form
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task Title").
				Description("A concise name for this work item.").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("title cannot be empty")
					}
					return nil
				}).
				Value(&title),

			huh.NewText().
				Title("Description").
				Description("Describe what needs to be done. Be specific about requirements and acceptance criteria.").
				Value(&description),

			huh.NewSelect[string]().
				Title("Difficulty").
				Description("Determines agent allocation strategy.").
				Options(
					huh.NewOption("Small Fix", "small_fix"),
					huh.NewOption("Small Feature", "small_feature"),
					huh.NewOption("Complex Refactor", "complex_refactor"),
					huh.NewOption("Full Module", "full_module"),
				).
				Value(&difficulty),
		),
	).WithTheme(huh.ThemeCharm()).Run()
	if err != nil {
		return fmt.Errorf("task creation cancelled: %w", err)
	}

	// Open the database
	dbPath := filepath.Join(".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Insert the task
	taskID, err := database.AddTask(title, description, "")
	if err != nil {
		return fmt.Errorf("failed to add task: %w", err)
	}

	// Generate the spec markdown
	spec := fmt.Sprintf(`# Task %d: %s

**Status:** pending
**Difficulty:** %s

## Description

%s
`, taskID, title, difficulty, description)

	// Ensure specs directory exists
	specsDir := filepath.Join(".assistant-to", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		return fmt.Errorf("failed to create specs directory: %w", err)
	}

	// Write the spec file
	specPath := filepath.Join(specsDir, fmt.Sprintf("%d.md", taskID))
	if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
		return fmt.Errorf("failed to write spec file: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s Task #%d created successfully\n", successStyle.Render("✓"), taskID)
	fmt.Printf("  %s %s\n", infoStyle.Render("Title:"), title)
	fmt.Printf("  %s %s\n", infoStyle.Render("Spec:"), specPath)

	return nil
}
