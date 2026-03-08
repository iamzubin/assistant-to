package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/merge"
	"assistant-to/internal/sandbox"
)

// Coordinator manages the agent swarm by reading tasks from the DB
// and spawning Builder agents in isolated tmux sessions.
type Coordinator struct {
	DB      *db.DB
	Config  *config.Config
	PWD     string // Project root directory
	Prompts *PromptBook
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	tier2   *Tier2Watchdog
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
	// Create a cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	defer cancel()

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

		// Mark task as started
		if err := c.DB.UpdateTaskStatus(task.ID, "started"); err != nil {
			log.Printf("Coordinator: failed to mark task %s as started: %v", taskID, err)
		}

		// Start Watchdog for this builder
		c.wg.Add(1)
		go func(taskID string) {
			defer c.wg.Done()
			watchdog := NewWatchdog(c.DB, c.PWD, c.Config)
			watchdog.MonitorHeartbeats(ctx, "builder-"+taskID)
		}(taskID)
	}

	fmt.Println("All builders spawned. Run `at dash` to monitor progress.")

	// Start Tier 2 Monitor Agent (if there are active tasks)
	if len(tasks) > 0 {
		c.tier2 = NewTier2Watchdog(c.DB, c.PWD)
		c.tier2.Start(ctx)
		log.Println("Coordinator: Tier 2 Monitor Agent started")
	}

	// Wait for context cancellation (shutdown signal)
	<-ctx.Done()
	log.Println("Coordinator: shutting down...")

	// Stop Tier 2 Monitor Agent
	if c.tier2 != nil {
		c.tier2.Stop()
		log.Println("Coordinator: Tier 2 Monitor Agent stopped")
	}

	// Wait for all goroutines to finish
	c.wg.Wait()
	log.Println("Coordinator: all workers stopped")

	return nil
}

// Close gracefully shuts down the Coordinator and cleans up resources.
func (c *Coordinator) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.DB != nil {
		return c.DB.Close()
	}
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

// handleTaskCompletion processes a completed task and triggers merge resolution
func (c *Coordinator) handleTaskCompletion(ctx context.Context, task db.Task) error {
	taskID := strconv.Itoa(task.ID)
	log.Printf("Coordinator: Task %s completed, initiating merge resolution...", taskID)

	// Update task status to merging
	if err := c.DB.UpdateTaskStatus(task.ID, db.TaskStatusMerging); err != nil {
		return fmt.Errorf("failed to update task status to merging: %w", err)
	}

	worktreeDir := filepath.Join(c.PWD, ".assistant-to", "worktrees", taskID)

	// Attempt 4-tier merge resolution
	resolver := merge.NewResolver(worktreeDir, "main")
	result, err := resolver.AttemptResolution()
	if err != nil {
		log.Printf("Coordinator: Merge resolution error for task %s: %v", taskID, err)
		c.DB.RecordEvent("coordinator", "merge_error",
			fmt.Sprintf("Task %s: %v", taskID, err))
		return err
	}

	if result.Success {
		log.Printf("Coordinator: Merge resolution successful for task %s via %s",
			taskID, result.Strategy)

		// Update task status to complete
		if err := c.DB.UpdateTaskStatus(task.ID, db.TaskStatusComplete); err != nil {
			return fmt.Errorf("failed to mark task as complete: %w", err)
		}

		// Send completion mail
		c.DB.SendMail("Coordinator", "User",
			fmt.Sprintf("Task %s Completed", taskID),
			fmt.Sprintf("Task '%s' has been successfully merged using %s strategy.",
				task.Title, result.Strategy),
			db.MailTypeResult, db.PriorityNormal)

		// Clean up worktree after successful merge
		go func() {
			time.Sleep(5 * time.Minute) // Give some time before cleanup
			if err := sandbox.TeardownWorktree(taskID, c.PWD); err != nil {
				log.Printf("Coordinator: Failed to teardown worktree for task %s: %v", taskID, err)
			}
		}()

		return nil
	}

	// Resolution failed, check if we need Tier 4 (AI-assisted)
	if result.Tier == merge.Tier4AIAssisted && !result.Success {
		log.Printf("Coordinator: Tier 4 resolution needed for task %s", taskID)
		return c.triggerAIAssistedMerge(ctx, task, result.Conflicts)
	}

	// All tiers failed
	log.Printf("Coordinator: All merge tiers failed for task %s", taskID)
	c.DB.RecordEvent("coordinator", "merge_failed",
		fmt.Sprintf("Task %s: All resolution tiers failed", taskID))

	// Send escalation mail
	c.DB.SendMail("Coordinator", "User",
		fmt.Sprintf("Merge Failed for Task %s", taskID),
		fmt.Sprintf("Task '%s' could not be automatically merged. Manual intervention required.\n\nFailed tier: %s\nConflicts: %v",
			task.Title, result.Tier, result.Conflicts),
		db.MailTypeEscalation, db.PriorityHigh)

	return fmt.Errorf("merge resolution failed: %s", result.Message)
}

// triggerAIAssistedMerge spawns a Merger agent for Tier 4 resolution
func (c *Coordinator) triggerAIAssistedMerge(ctx context.Context, task db.Task, conflicts []string) error {
	taskID := strconv.Itoa(task.ID)
	log.Printf("Coordinator: Spawning Merger agent for task %s", taskID)

	worktreeDir := filepath.Join(c.PWD, ".assistant-to", "worktrees", taskID)

	aiResolver := merge.NewAIAssistedResolution(c.DB, c.PWD, worktreeDir, "main", taskID)
	result, err := aiResolver.AttemptResolution(ctx)
	if err != nil {
		return fmt.Errorf("failed to spawn merger agent: %w", err)
	}

	if result.Success {
		log.Printf("Coordinator: Merger agent spawned successfully for task %s", taskID)

		// Start a goroutine to wait for merger completion
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			// Wait for resolution with timeout
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			defer cancel()

			finalResult, err := aiResolver.WaitForResolution(waitCtx, 30*time.Minute)
			if err != nil {
				log.Printf("Coordinator: Error waiting for merger on task %s: %v", taskID, err)
				return
			}

			if finalResult.Success {
				log.Printf("Coordinator: Merger completed successfully for task %s", taskID)
				c.DB.UpdateTaskStatus(task.ID, db.TaskStatusComplete)
			} else {
				log.Printf("Coordinator: Merger failed for task %s", taskID)
				c.DB.RecordEvent("coordinator", "merger_failed",
					fmt.Sprintf("Task %s merger agent failed", taskID))
			}
		}()
	}

	return nil
}

// checkBuilderCompletion checks if a builder has completed and triggers merge
func (c *Coordinator) checkBuilderCompletion(agentID string) bool {
	// Extract task ID from agent ID (format: builder-<id>)
	if !strings.HasPrefix(agentID, "builder-") {
		return false
	}

	taskIDStr := strings.TrimPrefix(agentID, "builder-")
	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		log.Printf("Coordinator: Failed to parse task ID from agent %s: %v", agentID, err)
		return false
	}

	// Get task
	task, err := c.DB.GetTaskByID(taskID)
	if err != nil {
		log.Printf("Coordinator: Failed to get task %d: %v", taskID, err)
		return false
	}

	// Check if builder session is still active
	worktreeDir := filepath.Join(c.PWD, ".assistant-to", "worktrees", taskIDStr)

	// Check for completion indicator file
	completionFile := filepath.Join(worktreeDir, ".builder_complete")
	if _, err := os.Stat(completionFile); err == nil {
		// Builder has signaled completion
		return true
	}

	// Alternative: Check if session is gone
	sessionName := sandbox.ProjectPrefix(c.PWD) + taskIDStr
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		// Session doesn't exist, might be complete
		return task.Status == db.TaskStatusBuilding
	}

	return false
}
