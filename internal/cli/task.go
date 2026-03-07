package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskUpdateCmd)
	taskCmd.AddCommand(taskRemoveCmd)
	RootCmd.AddCommand(taskCmd)
}

var taskRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a task from the queue",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("Error: invalid task id: %v\n", err)
			os.Exit(1)
		}
		if err := runTaskRemove(id); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks in the queue",
	Run: func(cmd *cobra.Command, args []string) {
		status, _ := cmd.Flags().GetString("status")
		if err := runTaskList(status); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <id> <status>",
	Short: "Update a task's status",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("Error: invalid task id: %v\n", err)
			os.Exit(1)
		}
		status := args[1]
		if err := runTaskUpdate(id, status); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runTaskAdd() error {
	var (
		title       string
		description string
		targetFiles string
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
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("description cannot be empty")
					}
					return nil
				}).
				Value(&description),

			huh.NewInput().
				Title("Target Files").
				Description("Comma-separated list of files or directories relevant to this task.").
				Value(&targetFiles),

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
	taskID, err := database.AddTask(title, description, targetFiles)
	if err != nil {
		return fmt.Errorf("failed to add task to DB: %w", err)
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

func runTaskList(status string) error {
	dbPath := filepath.Join(".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	tasks, err := database.ListTasksByStatus(status)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	fmt.Printf("%-4s | %-12s | %s\n", "ID", "Status", "Title")
	fmt.Println(strings.Repeat("-", 60))
	for _, t := range tasks {
		displayTitle := t.Title
		if displayTitle == "" {
			displayTitle = "(no title)"
		}
		fmt.Printf("%-4d | %-12s | %s\n", t.ID, t.Status, displayTitle)
	}

	return nil
}

func runTaskUpdate(id int, status string) error {
	dbPath := filepath.Join(".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.UpdateTaskStatus(id, status); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	fmt.Printf("Task %d updated to status: %s\n", id, status)
	return nil
}

func runTaskRemove(id int) error {
	dbPath := filepath.Join(".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.RemoveTask(id); err != nil {
		return fmt.Errorf("failed to remove task: %w", err)
	}

	fmt.Printf("Task %d removed.\n", id)
	return nil
}

func init() {
	taskListCmd.Flags().StringP("status", "s", "", "Filter tasks by status")
}
