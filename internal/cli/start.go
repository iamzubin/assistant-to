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

		// Start API/MCP servers in background tmux (logs to file)
		fmt.Println("Starting API/MCP servers in background...")
		serverSession := prefix + "servers"

		checkCmd := exec.Command("tmux", "has-session", "-t", serverSession)
		if checkCmd.Run() != nil {
			logFile := fmt.Sprintf("%s/.assistant-to/logs/coordinator.log", pwd)
			os.MkdirAll(fmt.Sprintf("%s/.assistant-to/logs", pwd), 0755)
			serveCmd := exec.Command("tmux", "new-session", "-d", "-s", serverSession,
				fmt.Sprintf("mkdir -p %s/.assistant-to/logs && %s serve > %s 2>&1", pwd, exePath, logFile))
			if err := serveCmd.Run(); err != nil {
				fmt.Printf("Failed to start servers: %v\n", err)
			} else {
				fmt.Printf("✓ API/MCP servers started (logs: %s)\n", logFile)
			}
		} else {
			fmt.Println("Servers already running")
		}

		// Start Coordinator AI agent in its own tmux session
		fmt.Println("Starting Coordinator agent in tmux...")
		coordSession := prefix + "coordinator"

		checkCmd = exec.Command("tmux", "has-session", "-t", coordSession)
		if checkCmd.Run() == nil {
			fmt.Printf("Session %s already exists. Attaching...\n", coordSession)
		} else {
			// Spawn the Coordinator AI agent
			coordCmd := exec.Command("tmux", "new-session", "-d", "-s", coordSession,
				fmt.Sprintf("%s run Coordinator --role Coordinator 2>&1", exePath))
			if err := coordCmd.Run(); err != nil {
				fmt.Printf("Failed to start Coordinator agent: %v\n", err)
			} else {
				fmt.Printf("✓ Coordinator agent started in tmux session: %s\n", coordSession)
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

// serveCmd runs the coordinator infrastructure only (API/MCP servers)
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run coordinator infrastructure only (internal)",
	Long:  `Runs the coordinator infrastructure (API/MCP servers) without spawning agents.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Just run coordinator in foreground (for tmux session)
		// Note: This blocks. Use via 'dwight up' for background execution
		fmt.Println("Starting coordinator infrastructure...")
		fmt.Println("Press Ctrl+C to stop.")

		// This will be run via the orchestrator
		// For now, just print a message - actual implementation in orchestrator.Run
		fmt.Println("(Coordinator infrastructure running - use dwight up to start properly)")
		fmt.Scanln()
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
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(attachCmd)
}
