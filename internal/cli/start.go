package cli

import (
	"fmt"
	"os"
	"os/exec"

	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the agent swarm (Coordinator + Dashboard)",
	Long: `Starts the Coordinator infrastructure and opens the Dashboard.

This launches:
1. API Server (REST endpoints for agents)
2. MCP Server (for AI tool integration)
3. Coordinator agent (autonomous task processing)
4. Dashboard (real-time monitoring)

Everything runs in tmux sessions. Use 'dwight stop' to stop everything.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		exePath, err := os.Executable()
		if err != nil {
			fmt.Printf("Failed to get executable path: %v\n", err)
			os.Exit(1)
		}

		prefix := sandbox.ProjectPrefix(pwd)

		// Start Coordinator in its own tmux session
		fmt.Println("Starting Coordinator in tmux...")
		coordSession := prefix + "coordinator"

		// Check if already running
		checkCmd := exec.Command("tmux", "has-session", "-t", coordSession)
		if checkCmd.Run() == nil {
			fmt.Printf("Session %s already exists. Attaching...\n", coordSession)
		} else {
			// Start coordinator infrastructure + agent in background tmux
			coordCmd := exec.Command("tmux", "new-session", "-d", "-s", coordSession,
				fmt.Sprintf("%s start --agent 2>&1", exePath))
			if err := coordCmd.Run(); err != nil {
				fmt.Printf("Failed to start Coordinator: %v\n", err)
			} else {
				fmt.Printf("✓ Coordinator started in tmux session: %s\n", coordSession)
			}
		}

		// Start Dashboard in its own tmux session
		fmt.Println("Starting Dashboard in tmux...")
		dashSession := prefix + "dashboard"

		checkCmd = exec.Command("tmux", "has-session", "-t", dashSession)
		if checkCmd.Run() == nil {
			fmt.Printf("Session %s already exists. Attaching...\n", dashSession)
		} else {
			dashCmd := exec.Command("tmux", "new-session", "-d", "-s", dashSession,
				fmt.Sprintf("%s dash", exePath))
			if err := dashCmd.Run(); err != nil {
				fmt.Printf("Failed to start Dashboard: %v\n", err)
			} else {
				fmt.Printf("✓ Dashboard started in tmux session: %s\n", dashSession)
			}
		}

		fmt.Println()
		fmt.Println("🎯 Run 'dwight attach' to connect to the Dashboard")
		fmt.Println("🛑 Run 'dwight stop' to stop all sessions")
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach to the Dashboard tmux session",
	Long:  `Attaches to the Dashboard tmux session for real-time monitoring.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		prefix := sandbox.ProjectPrefix(pwd)
		dashSession := prefix + "dashboard"

		tmuxCmd := "attach-session"
		if os.Getenv("TMUX") != "" {
			tmuxCmd = "switch-client"
		}

		c := exec.Command("tmux", tmuxCmd, "-t", dashSession)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("Failed to attach to session '%s': %v\n", dashSession, err)
			fmt.Println("Is the dashboard running? Try 'dwight up' first.")
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(upCmd)
	RootCmd.AddCommand(attachCmd)
}
