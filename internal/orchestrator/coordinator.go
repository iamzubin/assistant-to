package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

// Coordinator manages the agent swarm by reading tasks from the DB
// and spawning Builder agents in isolated tmux sessions.
type Coordinator struct {
	DB      *db.DB
	Config  *config.Config
	PWD     string // Project root directory
	Prompts *PromptBook
}

// NewCoordinator creates a Coordinator, loading config and prompts from the project root.
func NewCoordinator(pwd string) (*Coordinator, error) {
	configPath := filepath.Join(pwd, ".assistant-to", "config.yaml")
	conf, err := config.Load(configPath)
	if err != nil {
		log.Printf("Warning: failed to load config, using defaults: %v", err)
		conf = config.Default()
	}

	dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	// Look for prompts directory in the project's .assistant-to dir, then next to the binary.
	promptsPath := filepath.Join(pwd, ".assistant-to", "prompts")
	if _, err := os.Stat(promptsPath); os.IsNotExist(err) {
		// Fallback: look relative to this source file's package (dev mode)
		promptsPath = filepath.Join(pwd, "internal", "orchestrator", "prompts")
	}
	prompts, err := LoadPrompts(promptsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent prompts from %s: %w", promptsPath, err)
	}

	return &Coordinator{
		DB:      database,
		Config:  conf,
		PWD:     pwd,
		Prompts: prompts,
	}, nil
}

// Run starts the main orchestrator loop:
// 1. Fetch all pending tasks.
// 2. For each task, create a worktree + tmux session for a Builder agent.
// 3. Start a Watchdog goroutine to monitor each Builder.
func (c *Coordinator) Run(ctx context.Context) error {
	log.Println("Coordinator: fetching pending tasks...")
	tasks, err := c.DB.ListTasksByStatus("pending")
	if err != nil {
		return fmt.Errorf("failed to list pending tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No pending tasks found. Add tasks with `at task add`, then run `at start` again.")
		return nil
	}

	fmt.Printf("Found %d pending task(s). Spawning Builders...\n", len(tasks))

	for _, task := range tasks {
		taskID := strconv.Itoa(task.ID)
		err := c.spawnBuilder(ctx, task)
		if err != nil {
			log.Printf("Coordinator: failed to spawn Builder for task %s: %v", taskID, err)
			continue
		}

		// Mark task as active
		if err := c.DB.UpdateTaskStatus(task.ID, "active"); err != nil {
			log.Printf("Coordinator: failed to mark task %s as active: %v", taskID, err)
		}

		// Start Watchdog for this builder
		watchdog := &Watchdog{DB: c.DB}
		go watchdog.MonitorHeartbeats(ctx, "builder-"+taskID)
	}

	fmt.Println("All builders spawned. Run `at dash` to monitor progress.")
	return nil
}

func (c *Coordinator) spawnBuilder(ctx context.Context, task db.Task) error {
	taskID := strconv.Itoa(task.ID)

	// Ensure the worktree exists
	worktreeDir := filepath.Join(c.PWD, ".assistant-to", "worktrees", taskID)
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		log.Printf("Coordinator: creating worktree for task %s...", taskID)
		_, err = sandbox.CreateWorktree(c.PWD, taskID, "main")
		if err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}
	}

	// Build the initial prompt for the Builder
	rolePrompt := c.Prompts.Get("Builder")
	taskPrompt := fmt.Sprintf("%s\n\n---\n\n## Your Task (ID: %d)\n\n**Title:** %s\n\n**Description:**\n%s\n\n**Target Files:**\n%s",
		rolePrompt, task.ID, task.Title, task.Description, task.TargetFiles)

	// Write prompt to a mission file in the worktree to avoid shell escaping issues
	missionPath := filepath.Join(worktreeDir, ".mission.md")
	if err := os.WriteFile(missionPath, []byte(taskPrompt), 0644); err != nil {
		return fmt.Errorf("failed to write mission file: %w", err)
	}

	model := c.Config.ModelForRole("Builder")
	tool := c.Config.Tool
	if tool == "" {
		tool = "gemini"
	}

	var agentCmd string
	switch tool {
	case "gemini":
		agentCmd = fmt.Sprintf("%s --model %s --yolo -p \"$(cat .mission.md)\"", tool, model)
	case "opencode":
		agentCmd = fmt.Sprintf("%s --model %s --prompt \"$(cat .mission.md)\"", tool, model)
	default:
		agentCmd = fmt.Sprintf("%s --model %s --prompt \"$(cat .mission.md)\"", tool, model)
	}

	sessionName := sandbox.ProjectPrefix(c.PWD) + taskID
	session := sandbox.TmuxSession{
		SessionName: sessionName,
		WorktreeDir: worktreeDir,
		Command:     agentCmd,
	}

	log.Printf("Coordinator: spawning Builder for task %s (model=%s, session=%s)...", taskID, model, sessionName)
	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start tmux session: %w", err)
	}

	fmt.Printf("  ✓ Task %-4s | %-30s | session: %s\n", taskID, truncate(task.Title, 30), sessionName)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
