package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"assistant-to/internal/config"
	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var (
	spawnModel  string
	spawnRole   string
	spawnPrompt string
)

var spawnCmd = &cobra.Command{
	Use:   "spawn <task-id>",
	Short: "Manually spawn a tmux sandbox for an agent",
	Long: `Creates an isolated worktree and spawns a new tmux session for an agent targeting the specified task.
This simulates the orchestrator launching a task manually for testing and debugging.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting working directory: %v\n", err)
			os.Exit(1)
		}

		// Optionally create the worktree if it doesn't already exist
		worktreeDir := filepath.Join(pwd, ".assistant-to", "worktrees", taskID)
		if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
			fmt.Printf("Worktree not found, attempting to create it on 'main'...\n")
			_, err = sandbox.CreateWorktree(pwd, taskID, "main")
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

		var agentCmd string
		switch tool {
		case "gemini":
			agentCmd = fmt.Sprintf("%s --model %s --yolo", tool, model)
			if spawnPrompt != "" {
				agentCmd = fmt.Sprintf("%s --model %s --yolo -p %q", tool, model, spawnPrompt)
			}
		case "opencode":
			agentCmd = fmt.Sprintf("%s --model %s", tool, model)
			if spawnPrompt != "" {
				agentCmd = fmt.Sprintf("%s --model %s --prompt %q", tool, model, spawnPrompt)
			}
		default:
			// Generic fallback
			agentCmd = fmt.Sprintf("%s --model %s", tool, model)
			if spawnPrompt != "" {
				agentCmd = fmt.Sprintf("%s --model %s --prompt %q", tool, model, spawnPrompt)
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
		fmt.Printf("Success! Run 'at connect %s' to attach.\n", taskID)
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
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting working directory: %v\n", err)
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
	spawnCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "Model for the agent to use")
	spawnCmd.Flags().StringVarP(&spawnRole, "role", "r", "Builder", "Role of the agent (e.g., Builder, Reviewer)")
	spawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Initial prompt or context for the agent")

	rootCmd.AddCommand(spawnCmd)
	rootCmd.AddCommand(connectCmd)
}
