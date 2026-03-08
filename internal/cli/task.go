package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"assistant-to/internal/db"
	"assistant-to/internal/intelligence"
	"assistant-to/internal/orchestrator"

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

	taskListCmd.Flags().StringP("status", "s", "", "Filter tasks by status")
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

	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Open the database
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Insert the task
	taskID, err := database.AddTask(title, description, targetFiles)
	if err != nil {
		return fmt.Errorf("failed to add task to DB: %w", err)
	}

	// Ensure specs directory exists
	specsDir := filepath.Join(root, ".assistant-to", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		// Rollback: remove the task from DB
		if rollbackErr := database.RemoveTask(int(taskID)); rollbackErr != nil {
			return fmt.Errorf("failed to create specs directory: %w (rollback also failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("failed to create specs directory: %w", err)
	}

	// Generate AT_INSTRUCTIONS.md using PromptComposer
	specPath := filepath.Join(specsDir, fmt.Sprintf("%d.md", taskID))
	if err := generateATInstructions(root, int(taskID), title, description, targetFiles, difficulty, specPath); err != nil {
		// Rollback: remove the task from DB
		if rollbackErr := database.RemoveTask(int(taskID)); rollbackErr != nil {
			return fmt.Errorf("failed to generate AT_INSTRUCTIONS.md: %w (rollback also failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("failed to generate AT_INSTRUCTIONS.md: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s Task #%d created successfully\n", successStyle.Render("✓"), taskID)
	fmt.Printf("  %s %s\n", infoStyle.Render("Title:"), title)
	fmt.Printf("  %s %s\n", infoStyle.Render("Spec:"), specPath)

	return nil
}

func runTaskList(status string) error {
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

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
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	if err := database.UpdateTaskStatus(id, status); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	fmt.Printf("Task %d updated to status: %s\n", id, status)
	return nil
}

func runTaskRemove(id int) error {
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	if err := database.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	if err := database.RemoveTask(id); err != nil {
		return fmt.Errorf("failed to remove task: %w", err)
	}

	fmt.Printf("Task %d removed.\n", id)
	return nil
}

// generateATInstructions creates a proper AT_INSTRUCTIONS.md spec file for a task
func generateATInstructions(root string, taskID int, title, description, targetFiles, difficulty, outputPath string) error {
	// Parse target files
	var targetFilesList []string
	if targetFiles != "" {
		targetFilesList = strings.Split(targetFiles, ",")
		for i := range targetFilesList {
			targetFilesList[i] = strings.TrimSpace(targetFilesList[i])
		}
	}

	// Build composition context
	ctx := &orchestrator.CompositionContext{
		Role:        "Builder",
		TaskID:      strconv.Itoa(taskID),
		TaskTitle:   title,
		TaskDesc:    description,
		TargetFiles: targetFilesList,
		Timestamp:   time.Now(),
		BasePath:    root,
	}

	// Try to load code intelligence context if target files exist
	codeIndexPath := filepath.Join(root, ".assistant-to", "code-intelligence.db")
	if _, err := os.Stat(codeIndexPath); err == nil && len(targetFilesList) > 0 {
		idx, err := intelligence.NewCodeIndex(codeIndexPath)
		if err == nil {
			defer idx.Close()

			// Gather file info for target files
			var fileContext strings.Builder
			for _, file := range targetFilesList {
				fileInfo, err := idx.GetFileInfo(file)
				if err == nil {
					fileContext.WriteString(fmt.Sprintf("\n### %s\n", file))
					fileContext.WriteString(fmt.Sprintf("- Package: %s\n", fileInfo.Package))
					if len(fileInfo.Types) > 0 {
						fileContext.WriteString(fmt.Sprintf("- Types: %s\n", strings.Join(fileInfo.Types, ", ")))
					}
					if len(fileInfo.Functions) > 0 {
						funcNames := strings.Join(fileInfo.Functions, ", ")
						if len(funcNames) > 100 {
							funcNames = funcNames[:100] + "..."
						}
						fileContext.WriteString(fmt.Sprintf("- Functions: %s\n", funcNames))
					}
				}
			}
			if fileContext.Len() > 0 {
				ctx.TaskDesc += "\n\n## Target File Context\n" + fileContext.String()
			}
		}
	}

	// Load expertise relevant to the task
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err == nil {
		defer database.Close()
		if err := database.InitSchema(); err == nil {
			// Search for relevant expertise based on target files
			var relevantExpertise []string
			for _, file := range targetFilesList {
				domain := filepath.Dir(file)
				entries, err := database.SearchExpertise(domain)
				if err == nil {
					for _, entry := range entries {
						relevantExpertise = append(relevantExpertise,
							fmt.Sprintf("[%s] %s: %s", entry.Type, entry.Domain, entry.Description))
					}
				}
			}
			ctx.Expertise = relevantExpertise
		}
	}

	// Try to use PromptComposer for rich spec generation
	promptsPath := filepath.Join(root, ".assistant-to", "prompts")
	if _, err := os.Stat(promptsPath); err == nil {
		composer, err := orchestrator.NewPromptComposer(promptsPath)
		if err == nil {
			content, err := composer.Compose("Builder", ctx)
			if err == nil {
				header := fmt.Sprintf(`# AT_INSTRUCTIONS.md
# Generated: %s
# Task ID: %s
# Difficulty: %s
# Target Files: %s
#
# This is an auto-generated task specification for assistant-to.
# Do not edit this file manually - update via 'at task update' instead.

`, time.Now().Format(time.RFC3339), ctx.TaskID, difficulty, targetFiles)

				fullContent := header + content
				return os.WriteFile(outputPath, []byte(fullContent), 0644)
			}
		}
	}

	// Fallback: Generate basic AT_INSTRUCTIONS.md
	spec := fmt.Sprintf(`# AT_INSTRUCTIONS.md
# Generated: %s
# Task ID: %d
# Difficulty: %s
# Target Files: %s
#
# This is an auto-generated task specification for assistant-to.
# Do not edit this file manually - update via 'at task update' instead.

## Task Overview

**Title:** %s
**ID:** %d
**Difficulty:** %s
**Status:** pending

## Description

%s

## Target Files

%s

## Constraints & Guidelines

- Work strictly within the specified target files
- Do not modify files outside the scope unless explicitly required
- Follow existing code patterns and conventions
- Write tests for any new functionality
- Ensure the code compiles and tests pass before completing
`, time.Now().Format(time.RFC3339), taskID, difficulty, targetFiles, title, taskID, difficulty, description, targetFiles)

	if len(ctx.Expertise) > 0 {
		spec += "\n## Relevant Expertise\n\n"
		for _, exp := range ctx.Expertise {
			spec += fmt.Sprintf("- %s\n", exp)
		}
	}

	spec += `
## Completion Criteria

1. All requirements in the Description are met
2. Code follows project conventions
3. Tests pass (if applicable)
4. No breaking changes introduced
5. Changes are committed to the worktree

## Next Steps

1. Review this specification carefully
2. Examine the target files to understand current state
3. Implement the required changes
4. Test your changes thoroughly
5. Signal completion by creating a .builder_complete file in the worktree
`

	return os.WriteFile(outputPath, []byte(spec), 0644)
}
