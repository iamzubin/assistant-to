package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/orchestrator"
	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var (
	spawnModel  string
	spawnRole   string
	spawnPrompt string
)

var runCmd = &cobra.Command{
	Use:   "run <task-id>",
	Short: "Run an agent for a specific task",
	Long:  `Creates an isolated worktree and spawns a new tmux session for an agent targeting the specified task.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		// Optionally create the worktree if it doesn't already exist
		worktreeDir := filepath.Join(pwd, ".assistant-to", "worktrees", taskID)
		if taskID == "Coordinator" {
			worktreeDir = pwd
		} else if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
			fmt.Printf("Worktree not found, attempting to create it on 'main'...\n")
			_, err = sandbox.CreateWorktree(pwd, taskID, "main", "")
			if err != nil {
				fmt.Printf("Failed to create worktree: %v\n", err)
				os.Exit(1)
			}
		}

		sessionName := sandbox.ProjectPrefix(pwd) + taskID

		// Load config to determine the tool
		configPath := filepath.Join(pwd, ".assistant-to", "config.yaml")
		conf, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load workspace config, using defaults.\n")
			conf = config.Default()
		}

		tool := conf.Tool
		if tool == "" {
			tool = "gemini"
		}

		model := spawnModel
		if model == "" && conf != nil {
			model = conf.ModelForRole(spawnRole)
		}

		// Calculate project-specific MCP port
		_, mcpPort := conf.GetProjectPorts(pwd)

		// Normalize role (capitalize first letter) to match LoadPrompts convention
		role := spawnRole
		if len(role) > 0 {
			role = strings.ToUpper(role[:1]) + strings.ToLower(role[1:])
		}

		// Load prompt from agents.md if not provided
		finalPrompt := spawnPrompt
		if finalPrompt == "" {
			// Look for prompts directory
			promptsPath := filepath.Join(pwd, ".assistant-to", "prompts")
			if _, err := os.Stat(promptsPath); os.IsNotExist(err) {
				promptsPath = filepath.Join(pwd, "internal", "orchestrator", "prompts")
			}
			prompts, err := orchestrator.LoadPrompts(promptsPath)
			if err == nil {
				finalPrompt = prompts.Get(role)
				if finalPrompt == "" {
					fmt.Printf("Warning: no prompt found for role %q\n", role)
				}

				// For Coordinator role, inject MCP documentation
				if role == "Coordinator" {
					mcpContent := prompts.GetMCP("coordinator")
					if mcpContent != "" {
						mcpPort := conf.MCPPortForRole("coordinator")
						apiPort := conf.API.Port
						mcpContent = strings.ReplaceAll(mcpContent, "{{.MCPPort}}", fmt.Sprintf("%d", mcpPort))
						mcpContent = strings.ReplaceAll(mcpContent, "{{.APIPort}}", fmt.Sprintf("%d", apiPort))
						mcpContent = strings.ReplaceAll(mcpContent, "{{.TaskID}}", "Coordinator")
						finalPrompt = finalPrompt + "\n\n" + mcpContent
					}
				}
			} else {
				fmt.Printf("Warning: failed to load prompts: %v\n", err)
			}
		}

		// If it's a numeric task ID, enrich the prompt with task details from DB
		if id, err := strconv.Atoi(taskID); err == nil {
			dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
			database, err := db.Open(dbPath)
			if err != nil {
				fmt.Printf("Warning: failed to open state database for task enrichment: %v\n", err)
			} else {
				defer database.Close()
				task, err := database.GetTaskByID(id)
				if err != nil {
					fmt.Printf("Warning: failed to find task %d in database: %v\n", id, err)
				} else {
					// Enrich the prompt with task details (matching Coordinator logic)
					finalPrompt = fmt.Sprintf("%s\n\n---\n\n## Your Task (ID: %d)\n\n**Title:** %s\n\n**Description:**\n%s\n\n**Target Files:**\n%s",
						finalPrompt, task.ID, task.Title, task.Description, task.TargetFiles)
				}
			}
		}

		// Write prompt to a mission file in the worktree
		missionPath := filepath.Join(worktreeDir, ".mission.md")
		if err := os.WriteFile(missionPath, []byte(finalPrompt), 0644); err != nil {
			fmt.Printf("Warning: failed to write mission file: %v\n", err)
		}

		var agentCmd string
		switch tool {
		case "gemini":
			agentCmd = fmt.Sprintf("AT_MCP_PORT=%d AT_PROJECT_ROOT=%s AT_AGENT_ROLE=%s %s --model %s --yolo -p \"$(cat .mission.md)\"", mcpPort, pwd, role, tool, model)
		case "opencode":
			agentCmd = fmt.Sprintf("AT_MCP_PORT=%d AT_PROJECT_ROOT=%s AT_AGENT_ROLE=%s %s --model %s --prompt \"$(cat .mission.md)\"", mcpPort, pwd, role, tool, model)
		default:
			// Generic fallback
			agentCmd = fmt.Sprintf("AT_MCP_PORT=%d AT_PROJECT_ROOT=%s AT_AGENT_ROLE=%s %s --model %s --prompt \"$(cat .mission.md)\"", mcpPort, pwd, role, tool, model)
		}

		// Update mission status if it's a numeric task ID
		if id, err := strconv.Atoi(taskID); err == nil {
			dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
			database, err := db.Open(dbPath)
			if err == nil {
				database.UpdateTaskStatus(id, "started")
				database.Close()
			}
		}

		session := sandbox.TmuxSession{
			SessionName: sessionName,
			WorktreeDir: worktreeDir,
			Command:     agentCmd,
		}

		fmt.Printf("Spawning tmux session: %s\n", sessionName)
		if err := session.Start(cmd.Context()); err != nil {
			fmt.Printf("Error spawning session: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Success! Run 'dwight connect %s' to attach.\n", taskID)
	},
}

var connectCmd = &cobra.Command{
	Use:   "connect <task-id>",
	Short: "Connect to an active agent's tmux session",
	Long: `Attaches your terminal to the tmux session of an actively running agent.
Useful for observing the agent's live shell output or intervening directly.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}
		sessionName := sandbox.ProjectPrefix(pwd) + taskID

		// If we are currently inside of a tmux session, we switch-client to avoid nesting
		// If we are in a normal terminal, we attach-session
		tmuxCmd := "attach-session"
		if os.Getenv("TMUX") != "" {
			tmuxCmd = "switch-client"
		}

		c := exec.Command("tmux", tmuxCmd, "-t", sessionName)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("Failed to connect to session '%s': %v\n", sessionName, err)
			fmt.Println("Are you sure this agent is currently running?")
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "Model for the agent to use")
	runCmd.Flags().StringVarP(&spawnRole, "role", "r", "Builder", "Role of the agent (e.g., Builder, Reviewer)")
	runCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Initial prompt or context for the agent")

	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(connectCmd)
}
