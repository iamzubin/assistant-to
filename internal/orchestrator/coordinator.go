package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"assistant-to/internal/api"
	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/merge"
	"assistant-to/internal/sandbox"
)

// Coordinator manages the agent swarm by reading tasks from the DB
// and spawning Builder agents in isolated tmux sessions.
type Coordinator struct {
	DB              *db.DB
	Config          *config.Config
	PWD             string // Project root directory
	Prompts         *PromptBook
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	tier2           *Tier2Watchdog
	apiServer       *api.Server
	mcpServer       *api.MCPServer
	RunIndefinitely bool // If true, keep running even when no tasks are found
	mergerSpawned   bool // Track if merger has been spawned this session
}

// NewCoordinator creates a Coordinator, loading config and prompts from the project root.
func NewCoordinator(pwd string) (*Coordinator, error) {
	configPath := filepath.Join(pwd, ".assistant-to", "config.yaml")
	conf, err := config.Load(configPath)
	if err != nil {
		log.Printf("Warning: failed to load config, using defaults: %v", err)
		conf = config.Default()
	}

	// Initialize logging system from config
	config.InitLogging(conf)
	config.Debug("Coordinator initialized with verbose=%v", conf.Logging.Verbose)

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
// 4. Start API and MCP servers for agent communication.
func (c *Coordinator) Run(ctx context.Context) error {
	// Create a cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	defer cancel()

	// Start API server if enabled
	if c.Config.API.Enabled {
		c.apiServer = api.NewServer(c.PWD, c.Config, c.DB)
		if err := c.apiServer.Start(ctx); err != nil {
			log.Printf("Warning: failed to start API server: %v", err)
		} else {
			log.Printf("API server started at http://%s:%d", c.Config.API.Host, c.Config.API.Port)
		}
	}

	// Start MCP server if enabled
	if c.Config.API.MCPEnabled {
		c.mcpServer = api.NewMCPServer(c.Config.API.MCPPort, c.PWD, c.Config, c.DB)
		if err := c.mcpServer.Start(ctx); err != nil {
			log.Printf("Warning: failed to start MCP server: %v", err)
		} else {
			log.Printf("MCP server started at 127.0.0.1:%d", c.Config.API.MCPPort)
		}
	}

	log.Println("Coordinator: fetching pending tasks...")
	tasks, err := c.DB.ListTasksByStatus("pending")
	if err != nil {
		return fmt.Errorf("failed to list pending tasks: %w", err)
	}

	if len(tasks) == 0 {
		if c.RunIndefinitely {
			fmt.Println("No pending tasks found. Waiting for tasks... (Press Ctrl+C to stop)")
			// Keep servers running and wait for context cancellation
			<-ctx.Done()
			return nil
		}
		fmt.Println("No pending tasks found. Add tasks with `dwight task add`, then run `dwight up` again.")
		return nil
	}

	fmt.Printf("Found %d pending task(s). Spawning agents...\n", len(tasks))

	// Enforce max concurrent agents limit
	maxConcurrent := c.Config.Agents.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5 // default
	}
	staggerDelay := time.Duration(c.Config.Agents.StaggerDelayMs) * time.Millisecond
	if staggerDelay <= 0 {
		staggerDelay = 2 * time.Second // default
	}

	config.Debug("Max concurrent agents: %d, stagger delay: %v", maxConcurrent, staggerDelay)

	// Create semaphore to limit concurrent agents
	semaphore := make(chan struct{}, maxConcurrent)
	var spawnedCount int

	for _, task := range tasks {
		taskID := strconv.Itoa(task.ID)

		// Acquire semaphore slot (blocks if at max concurrent)
		select {
		case semaphore <- struct{}{}:
			// Got a slot, continue
		case <-ctx.Done():
			config.Info("Coordinator: Context cancelled, stopping agent spawning")
			return nil
		}

		// Apply stagger delay between spawns (except first)
		if spawnedCount > 0 && staggerDelay > 0 {
			config.Debug("Staggering agent spawn, waiting %v...", staggerDelay)
			select {
			case <-time.After(staggerDelay):
				// Continue
			case <-ctx.Done():
				config.Info("Coordinator: Context cancelled during stagger delay")
				return nil
			}
		}
		spawnedCount++

		// For complex tasks, spawn a Scout first
		if c.shouldSpawnScout(task) {
			if err := c.spawnScout(ctx, task); err != nil {
				config.Error("Coordinator: failed to spawn Scout for task %s: %v", taskID, err)
				<-semaphore // Release slot
				// Continue to spawn Builder anyway
			} else {
				// Start Scout watchdog to wait for completion
				c.wg.Add(1)
				go func(taskID string, task db.Task, sem chan struct{}) {
					defer c.wg.Done()
					defer func() { <-sem }() // Release slot when done
					c.waitForScoutAndSpawnBuilder(ctx, task)
				}(taskID, task, semaphore)
				continue // Skip direct Builder spawn, will be done after Scout completes
			}
		}

		// Spawn Builder directly
		c.spawnBuilderAndWatchdogWithSemaphore(ctx, task, semaphore)
	}

	fmt.Println("All builders spawned. Run `dwight dash` to monitor progress.")

	// Start Tier 2 Monitor Agent (if enabled and there are active tasks)
	if c.Config.Watchdog.Tier2Enabled && len(tasks) > 0 {
		c.tier2 = NewTier2Watchdog(c.DB, c.PWD)
		c.tier2.Start(ctx)
		config.Info("Coordinator: Tier 2 Monitor Agent started")
	} else {
		config.Debug("Coordinator: Tier 2 Monitor Agent disabled or no tasks")
	}

	// Start merger watcher - spawns merger once when all tasks complete
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watchForMerge(ctx)
	}()

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

// spawnScout creates and starts a Scout agent for read-only exploration
func (c *Coordinator) spawnScout(ctx context.Context, task db.Task) error {
	taskID := strconv.Itoa(task.ID)

	// Ensure the worktree exists
	worktreesDir := c.Config.GetWorktreesDir(c.PWD)
	worktreeDir := filepath.Join(worktreesDir, taskID)
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		log.Printf("Coordinator: creating worktree for task %s...", taskID)
		_, err = sandbox.CreateWorktree(c.PWD, taskID, "main", worktreesDir)
		if err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}
	}

	// Build the mission for the Scout
	rolePrompt := c.Prompts.Get("Scout")
	if rolePrompt == "" {
		rolePrompt = "You are a Scout agent. Explore the codebase and report findings."
	}

	// Get MCP configuration for Scout
	mcpPort := c.Config.MCPPortForRole("Scout")
	apiPort := c.Config.API.Port
	mcpContent := c.Prompts.GetMCP("Scout")
	if mcpContent != "" {
		mcpContent = strings.ReplaceAll(mcpContent, "{{.MCPPort}}", fmt.Sprintf("%d", mcpPort))
		mcpContent = strings.ReplaceAll(mcpContent, "{{.APIPort}}", fmt.Sprintf("%d", apiPort))
		mcpContent = strings.ReplaceAll(mcpContent, "{{.TaskID}}", taskID)
	}

	scoutMission := fmt.Sprintf(`%s

%s

---

## Your Mission (Task ID: %d)

**Task Title:** %s

**Description:**
%s

**Target Files:**
%s

**Communication:**
- API Server: http://127.0.0.1:%d
- MCP Server: 127.0.0.1:%d

**Instructions:**
1. Explore the target files and their dependencies
2. Use grep, find, and other tools to understand the codebase structure
3. Identify related files, patterns, and conventions
4. Report your findings via mail to Coordinator
5. Include: file paths, key functions/types, dependencies, and any relevant patterns
6. Mark completion by creating: touch .scout_complete

**Environment:** You are in READ-ONLY mode. Do NOT modify any files.
`, rolePrompt, mcpContent, task.ID, task.Title, task.Description, task.TargetFiles, apiPort, mcpPort)

	// Write mission file
	missionPath := filepath.Join(worktreeDir, ".scout_mission.md")
	if err := os.WriteFile(missionPath, []byte(scoutMission), 0644); err != nil {
		return fmt.Errorf("failed to write scout mission file: %w", err)
	}

	// Generate MCP config files in the worktree
	if err := c.generateWorktreeMCPConfigs(worktreeDir, "Scout", taskID); err != nil {
		log.Printf("Warning: failed to generate MCP configs for scout task %s: %v", taskID, err)
	}

	model := c.Config.ModelForRole("Scout")
	tool := c.Config.RuntimeForRole("Scout")

	var agentCmd string
	switch tool {
	case "gemini":
		agentCmd = fmt.Sprintf(`READ_ONLY_MODE=1 %s --model %s --yolo -p "$(cat .scout_mission.md)"`, tool, model)
	case "opencode":
		agentCmd = fmt.Sprintf(`READ_ONLY_MODE=1 %s --model %s --prompt "$(cat .scout_mission.md)"`, tool, model)
	default:
		agentCmd = fmt.Sprintf(`READ_ONLY_MODE=1 %s --model %s --prompt "$(cat .scout_mission.md)"`, tool, model)
	}

	sessionName := sandbox.ProjectPrefix(c.PWD) + "scout-" + taskID
	session := sandbox.TmuxSession{
		SessionName: sessionName,
		WorktreeDir: worktreeDir,
		Command:     agentCmd,
		ReadOnly:    true,
		EnvVars: map[string]string{
			"AT_AGENT_ROLE": "Scout",
			"AT_TASK_ID":    taskID,
			"AT_READ_ONLY":  "1",
		},
	}

	log.Printf("Coordinator: spawning Scout for task %s (model=%s, session=%s)...", taskID, model, sessionName)
	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scout tmux session: %w", err)
	}

	// Update task status to scouted
	if err := c.DB.UpdateTaskStatus(task.ID, db.TaskStatusScouted); err != nil {
		log.Printf("Coordinator: failed to mark task %s as scouted: %v", taskID, err)
	}

	// Send initial mail to Scout
	c.DB.SendMail("Coordinator", "scout-"+taskID,
		fmt.Sprintf("Mission: Explore Task %s", taskID),
		fmt.Sprintf("Begin reconnaissance for task: %s\n\nTarget: %s", task.Title, task.TargetFiles),
		db.MailTypeDispatch, db.PriorityNormal)

	fmt.Printf("  ✓ Scout %-4s | %-30s | session: %s\n", taskID, truncate(task.Title, 30), sessionName)
	return nil
}

func (c *Coordinator) spawnBuilder(ctx context.Context, task db.Task) error {
	taskID := strconv.Itoa(task.ID)

	// Ensure the worktree exists
	worktreesDir := c.Config.GetWorktreesDir(c.PWD)
	worktreeDir := filepath.Join(worktreesDir, taskID)
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		log.Printf("Coordinator: creating worktree for task %s...", taskID)
		_, err = sandbox.CreateWorktree(c.PWD, taskID, "main", worktreesDir)
		if err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}
	}

	// Build the initial prompt for the Builder
	rolePrompt := c.Prompts.Get("Builder")
	allowedTools := c.Config.AllowedToolsForRole("Builder")
	apiPort := c.Config.API.Port
	mcpPort := c.Config.MCPPortForRole("Builder")
	toolDocs := c.generateToolDocs("Builder", allowedTools, apiPort, mcpPort)

	// Get MCP configuration for this role
	mcpContent := c.Prompts.GetMCP("Builder")
	if mcpContent != "" {
		// Replace template variables in MCP content
		mcpContent = strings.ReplaceAll(mcpContent, "{{.MCPPort}}", fmt.Sprintf("%d", mcpPort))
		mcpContent = strings.ReplaceAll(mcpContent, "{{.APIPort}}", fmt.Sprintf("%d", apiPort))
		mcpContent = strings.ReplaceAll(mcpContent, "{{.TaskID}}", taskID)
	}

	taskPrompt := fmt.Sprintf("%s\n\n---\n\n## Your Task (ID: %d)\n\n**Title:** %s\n\n**Description:**\n%s\n\n**Target Files:**\n%s\n\n**Communication:**\n- API Server: http://127.0.0.1:%d\n- MCP Server: 127.0.0.1:%d\n%s\n\n%s",
		rolePrompt, task.ID, task.Title, task.Description, task.TargetFiles, apiPort, mcpPort, toolDocs, mcpContent)

	// Write prompt to a mission file in the worktree to avoid shell escaping issues
	missionPath := filepath.Join(worktreeDir, ".mission.md")
	if err := os.WriteFile(missionPath, []byte(taskPrompt), 0644); err != nil {
		return fmt.Errorf("failed to write mission file: %w", err)
	}

	// Generate MCP config files in the worktree for the AI tool
	if err := c.generateWorktreeMCPConfigs(worktreeDir, "Builder", taskID); err != nil {
		log.Printf("Warning: failed to generate MCP configs for task %s: %v", taskID, err)
	}

	model := c.Config.ModelForRole("Builder")
	tool := c.Config.RuntimeForRole("Builder")

	// Build command that:
	// 1. Changes to worktree directory (so opencode can find .opencode.json)
	// 2. Runs the AI tool with the mission
	var agentCmd string
	switch tool {
	case "gemini":
		// Gemini CLI doesn't support MCP, use REST API only
		agentCmd = fmt.Sprintf("cd %s && %s --model %s --yolo -p \"$(cat .mission.md)\"",
			worktreeDir, tool, model)
	case "opencode":
		// Opencode automatically reads .opencode.json for MCP config
		agentCmd = fmt.Sprintf("cd %s && %s --model %s --prompt \"$(cat .mission.md)\"",
			worktreeDir, tool, model)
	default:
		agentCmd = fmt.Sprintf("cd %s && %s --model %s --prompt \"$(cat .mission.md)\"",
			worktreeDir, tool, model)
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

	worktreeDir := filepath.Join(c.Config.GetWorktreesDir(c.PWD), taskID)

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
			worktreesDir := c.Config.GetWorktreesDir(c.PWD)
			if err := sandbox.TeardownWorktree(taskID, c.PWD, worktreesDir); err != nil {
				log.Printf("Coordinator: Failed to teardown worktree for task %s: %v", taskID, err)
			}
		}()

		return nil
	}

	// Resolution failed, check if we need Tier 4 (AI-assisted)
	if result.Tier == merge.Tier4AIAssisted && !result.Success {
		if c.Config.Merge.AIResolveEnabled {
			log.Printf("Coordinator: Tier 4 resolution needed for task %s", taskID)
			return c.triggerAIAssistedMerge(ctx, task, result.Conflicts)
		}
		log.Printf("Coordinator: Tier 4 resolution needed but AIResolveEnabled is false for task %s", taskID)
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

	worktreeDir := filepath.Join(c.Config.GetWorktreesDir(c.PWD), taskID)

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
	worktreeDir := filepath.Join(c.Config.GetWorktreesDir(c.PWD), taskIDStr)

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

// watchForMerge periodically checks if all tasks are complete and spawns Merger once
func (c *Coordinator) watchForMerge(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Skip if merger already spawned
			if c.mergerSpawned {
				return
			}

			// Get all non-complete tasks
			tasks, err := c.DB.ListTasksByStatus("")
			if err != nil {
				log.Printf("Coordinator: Failed to list tasks for merge check: %v", err)
				continue
			}

			// Check if there are any pending/building/review tasks
			hasIncomplete := false
			hasCompleted := false
			for _, task := range tasks {
				if task.Status == db.TaskStatusPending ||
					task.Status == db.TaskStatusStarted ||
					task.Status == db.TaskStatusBuilding ||
					task.Status == db.TaskStatusReview {
					hasIncomplete = true
					break
				}
				if task.Status == db.TaskStatusComplete {
					hasCompleted = true
				}
			}

			// If no incomplete tasks but some completed, spawn merger
			if !hasIncomplete && hasCompleted && !c.mergerSpawned {
				log.Printf("Coordinator: All tasks complete, spawning Merger")
				if err := c.spawnMerger(ctx); err != nil {
					log.Printf("Coordinator: Failed to spawn Merger: %v", err)
				} else {
					c.mergerSpawned = true
					return // Stop watching after spawning
				}
			}
		}
	}
}

// spawnMerger spawns a Merger agent in the project root to merge all worktrees
func (c *Coordinator) spawnMerger(ctx context.Context) error {
	log.Printf("Coordinator: Spawning Merger agent in project root")

	// Load config and prompts
	configPath := filepath.Join(c.PWD, ".assistant-to", "config.yaml")
	conf, err := config.Load(configPath)
	if err != nil {
		conf = config.Default()
	}

	tool := conf.RuntimeForRole("merger")
	model := conf.ModelForRole("merger")

	// Load prompt
	promptsPath := filepath.Join(c.PWD, ".assistant-to", "prompts")
	if _, err := os.Stat(promptsPath); os.IsNotExist(err) {
		promptsPath = filepath.Join(c.PWD, "internal", "orchestrator", "prompts")
	}
	prompts, err := LoadPrompts(promptsPath)
	if err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}

	rolePrompt := prompts.Get("Merger")
	if rolePrompt == "" {
		rolePrompt = "You are the Merger agent. Merge completed task branches into main."
	}

	// Get list of completed tasks
	tasks, _ := c.DB.ListTasksByStatus("complete")
	var taskList string
	for _, task := range tasks {
		taskList += fmt.Sprintf("- Task %d: %s (branch: at-%d)\n", task.ID, task.Title, task.ID)
	}

	mission := fmt.Sprintf(`%s

## Merge Mission

Merge these completed task branches into main:
%s

Commands:
- dwight worktree merge <task-id>  # Merge a task
- dwight mail send --to coordinator --subject "Merged" --body "All tasks merged"
- go build ./... && go test ./...   # Verify after merge
`, rolePrompt, taskList)

	// Write mission file in project root
	missionPath := filepath.Join(c.PWD, ".merger_mission.md")
	if err := os.WriteFile(missionPath, []byte(mission), 0644); err != nil {
		return fmt.Errorf("failed to write merger mission: %w", err)
	}

	// Generate MCP config for merger
	if err := c.generateWorktreeMCPConfigs(c.PWD, "merger", "merger"); err != nil {
		log.Printf("Warning: failed to generate MCP configs for merger: %v", err)
	}

	// Build command - runs in project root
	var agentCmd string
	switch tool {
	case "gemini":
		agentCmd = fmt.Sprintf("AT_AGENT_ROLE=merger %s --model %s --yolo -p \"$(cat .merger_mission.md)\"", tool, model)
	case "opencode":
		agentCmd = fmt.Sprintf("AT_AGENT_ROLE=merger %s --model %s --prompt \"$(cat .merger_mission.md)\"", tool, model)
	default:
		agentCmd = fmt.Sprintf("AT_AGENT_ROLE=merger %s --model %s --prompt \"$(cat .merger_mission.md)\"", tool, model)
	}

	sessionName := sandbox.ProjectPrefix(c.PWD) + "merger"
	session := sandbox.TmuxSession{
		SessionName: sessionName,
		WorktreeDir: c.PWD, // Project root, not worktree
		Command:     agentCmd,
	}

	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start merger session: %w", err)
	}

	log.Printf("Coordinator: Merger agent started in session %s", sessionName)
	return nil
}

// shouldSpawnScout determines if a task needs exploration before building
// Simple tasks: single file, <50 lines, clear instructions
// Complex tasks: multiple files, refactoring, needs exploration
func (c *Coordinator) shouldSpawnScout(task db.Task) bool {
	if !c.Config.IsScoutEnabled() {
		return false
	}

	// Check if task description suggests complexity
	complexKeywords := []string{"explore", "find", "understand", "investigate", "refactor", "multiple", "complex"}
	descLower := strings.ToLower(task.Description)
	for _, keyword := range complexKeywords {
		if strings.Contains(descLower, keyword) {
			return true
		}
	}

	// Check if spanning multiple packages/directories
	if task.TargetFiles != "" {
		files := strings.Split(task.TargetFiles, ",")
		dirs := make(map[string]bool)
		for _, file := range files {
			dir := filepath.Dir(strings.TrimSpace(file))
			if dir != "." && dir != "" {
				dirs[dir] = true
			}
		}
		// 3+ directories = complex
		if len(dirs) >= 3 {
			return true
		}
	}

	return false
}

// waitForScoutAndSpawnBuilder waits for Scout to complete then spawns Builder
func (c *Coordinator) waitForScoutAndSpawnBuilder(ctx context.Context, task db.Task) {
	taskID := strconv.Itoa(task.ID)
	worktreeDir := filepath.Join(c.Config.GetWorktreesDir(c.PWD), taskID)
	scoutCompleteFile := filepath.Join(worktreeDir, ".scout_complete")

	// Wait for Scout completion with configurable timeout
	scoutTimeout := c.Config.GetScoutWaitDuration()
	timeout := time.After(scoutTimeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Printf("Coordinator: Waiting for Scout to complete for task %s (timeout: %v)...", taskID, scoutTimeout)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Coordinator: Context cancelled while waiting for Scout on task %s", taskID)
			return
		case <-timeout:
			log.Printf("Coordinator: Scout timeout for task %s after %v, proceeding with Builder anyway", taskID, scoutTimeout)
			c.spawnBuilderAndWatchdog(ctx, task)
			return
		case <-ticker.C:
			// Check if Scout completed
			if _, err := os.Stat(scoutCompleteFile); err == nil {
				log.Printf("Coordinator: Scout completed for task %s, spawning Builder", taskID)
				c.spawnBuilderAndWatchdog(ctx, task)
				return
			}

			// Check if Scout session still exists
			scoutSession := sandbox.ProjectPrefix(c.PWD) + "scout-" + taskID
			cmd := exec.Command("tmux", "has-session", "-t", scoutSession)
			if err := cmd.Run(); err != nil {
				// Scout session gone, assume completion
				log.Printf("Coordinator: Scout session ended for task %s, spawning Builder", taskID)
				c.spawnBuilderAndWatchdog(ctx, task)
				return
			}
		}
	}
}

// spawnBuilderAndWatchdog spawns a Builder agent and starts its watchdog
func (c *Coordinator) spawnBuilderAndWatchdog(ctx context.Context, task db.Task) {
	taskID := strconv.Itoa(task.ID)

	err := c.spawnBuilder(ctx, task)
	if err != nil {
		log.Printf("Coordinator: failed to spawn Builder for task %s: %v", taskID, err)
		return
	}

	// Mark task as started
	if err := c.DB.UpdateTaskStatus(task.ID, db.TaskStatusStarted); err != nil {
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

// spawnBuilderAndWatchdogWithSemaphore spawns a Builder with semaphore-based concurrency control
func (c *Coordinator) spawnBuilderAndWatchdogWithSemaphore(ctx context.Context, task db.Task, semaphore chan struct{}) {
	taskID := strconv.Itoa(task.ID)

	err := c.spawnBuilder(ctx, task)
	if err != nil {
		config.Error("Coordinator: failed to spawn Builder for task %s: %v", taskID, err)
		<-semaphore // Release slot on failure
		return
	}

	// Mark task as started
	if err := c.DB.UpdateTaskStatus(task.ID, db.TaskStatusStarted); err != nil {
		config.Error("Coordinator: failed to mark task %s as started: %v", taskID, err)
	}

	// Start Watchdog for this builder
	c.wg.Add(1)
	go func(taskID string, sem chan struct{}) {
		defer c.wg.Done()
		defer func() { <-sem }() // Release slot when watchdog finishes
		watchdog := NewWatchdog(c.DB, c.PWD, c.Config)
		watchdog.MonitorHeartbeats(ctx, "builder-"+taskID)
	}(taskID, semaphore)
}

// generateToolDocs generates documentation for the allowed tools based on config
func (c *Coordinator) generateToolDocs(role string, allowedTools []string, apiPort, mcpPort int) string {
	var docs string

	has := func(tool string) bool {
		for _, t := range allowedTools {
			if t == tool {
				return true
			}
		}
		return false
	}

	docs += "\n\n## Available Tools (REST API)\n\n"
	docs += "You can call these tools via REST API at `http://127.0.0.1:" + fmt.Sprintf("%d", apiPort) + "`:\n\n"

	if has("mail") {
		docs += "### Mail\n"
		docs += "- `GET /api/mail/list?recipient=<agent>` - List mail messages\n"
		docs += "- `POST /api/mail/send` - Send mail (to, subject, body, type)\n"
		docs += "- `GET /api/mail/check?recipient=<agent>` - Check and retrieve unread mail\n\n"
	}

	if has("log") {
		docs += "### Logging\n"
		docs += "- `POST /api/log` - Log event (type, details)\n\n"
	}

	if has("task") {
		docs += "### Task Management\n"
		docs += "- `GET /api/task/list?status=<status>` - List tasks\n"
		docs += "- `POST /api/task/update` - Update task status (task_id, status)\n\n"
	}

	if has("buffer") {
		docs += "### Buffer/Debug\n"
		docs += "- `GET /api/buffer?agent_id=<id>&lines=<n>` - Capture tmux buffer\n\n"
	}

	if has("session") {
		docs += "### Session Management\n"
		docs += "- `GET /api/session/list` - List active sessions\n"
		docs += "- `POST /api/session/kill` - Kill session (agent_id)\n"
		docs += "- `POST /api/session/send` - Send input (agent_id, input)\n"
		docs += "- `POST /api/session/clear` - Clear buffer (agent_id)\n\n"
	}

	if has("cleanup") {
		docs += "### Cleanup\n"
		docs += "- `POST /api/cleanup` - Cleanup task (task_id)\n\n"
	}

	if has("worktree") {
		docs += "### Worktree\n"
		docs += "- `POST /api/worktree/merge` - Merge worktree (task_id, base_branch)\n"
		docs += "- `POST /api/worktree/teardown` - Teardown worktree (task_id)\n\n"
	}

	if has("spawn") {
		docs += "### Spawn Agents\n"
		docs += "- Spawn new agents via CLI: `dwight run <task-id> --role <role>`\n\n"
	}

	if has("dash") {
		docs += "### Dashboard\n"
		docs += "- `dwight dash` - Open live dashboard\n\n"
	}

	docs += "## MCP Tools\n\n"
	docs += fmt.Sprintf("You can also connect to MCP server at `127.0.0.1:%d` for structured tool calls.\n", mcpPort)
	docs += "Available MCP tools: " + strings.Join(allowedTools, ", ")

	return docs
}

// generateWorktreeMCPConfigs creates MCP configuration files in the worktree for AI tools
func (c *Coordinator) generateWorktreeMCPConfigs(worktreeDir, role, taskID string) error {
	mcpPort := c.Config.MCPPortForRole(role)

	// Generate opencode.json (correct format per https://opencode.ai/docs/mcp-servers)
	opencodeConfig := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]interface{}{
			"assistant-to": map[string]interface{}{
				"type":    "local",
				"command": []string{"dwight", "mcp", "serve"},
				"enabled": true,
				"environment": map[string]string{
					"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPort),
					"AT_AGENT_ROLE":   role,
					"AT_TASK_ID":      taskID,
					"AT_PROJECT_ROOT": c.PWD,
				},
			},
		},
	}

	opencodePath := filepath.Join(worktreeDir, "opencode.json")
	opencodeData, err := json.MarshalIndent(opencodeConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal opencode config: %w", err)
	}
	if err := os.WriteFile(opencodePath, opencodeData, 0644); err != nil {
		return fmt.Errorf("failed to write opencode config: %w", err)
	}

	// Generate Gemini settings.json (correct format per https://geminicli.com/docs/tools/mcp-server/)
	geminiConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"assistant-to": map[string]interface{}{
				"command": "dwight",
				"args":    []string{"mcp", "serve"},
				"env": map[string]string{
					"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPort),
					"AT_AGENT_ROLE":   role,
					"AT_TASK_ID":      taskID,
					"AT_PROJECT_ROOT": c.PWD,
				},
			},
		},
	}

	geminiPath := filepath.Join(worktreeDir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(geminiPath), 0755); err != nil {
		return fmt.Errorf("failed to create .gemini directory: %w", err)
	}
	geminiData, err := json.MarshalIndent(geminiConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal gemini config: %w", err)
	}
	if err := os.WriteFile(geminiPath, geminiData, 0644); err != nil {
		return fmt.Errorf("failed to write gemini config: %w", err)
	}

	// Generate Claude Desktop config (claude_desktop_config.json)
	claudeConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"assistant-to": map[string]interface{}{
				"command": "dwight",
				"args":    []string{"mcp", "serve"},
				"env": map[string]string{
					"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPort),
					"AT_AGENT_ROLE":   role,
					"AT_TASK_ID":      taskID,
					"AT_PROJECT_ROOT": c.PWD,
				},
			},
		},
	}

	claudePath := filepath.Join(worktreeDir, "claude_desktop_config.json")
	claudeData, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal claude config: %w", err)
	}
	if err := os.WriteFile(claudePath, claudeData, 0644); err != nil {
		return fmt.Errorf("failed to write claude config: %w", err)
	}

	// Generate generic mcp.json
	mcpConfig := map[string]interface{}{
		"name":        "assistant-to",
		"description": fmt.Sprintf("Assistant-to %s agent MCP server", role),
		"transport":   "stdio",
		"command":     "dwight",
		"args":        []string{"mcp", "serve"},
		"env": map[string]string{
			"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPort),
			"AT_AGENT_ROLE":   role,
			"AT_TASK_ID":      taskID,
			"AT_PROJECT_ROOT": c.PWD,
		},
	}

	mcpPath := filepath.Join(worktreeDir, "mcp.json")
	mcpData, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mcp config: %w", err)
	}
	if err := os.WriteFile(mcpPath, mcpData, 0644); err != nil {
		return fmt.Errorf("failed to write mcp config: %w", err)
	}

	return nil
}
