package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"assistant-to/internal/config"
	"assistant-to/internal/orchestrator"
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

		// Step 1: Load config and calculate project-specific ports
		configPath := filepath.Join(pwd, ".assistant-to", "config.yaml")
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load config, using defaults: %v\n", err)
			cfg = config.Default()
		}
		apiPort, mcpPort := cfg.GetProjectPorts(pwd)
		_ = apiPort // Not needed directly

		// Step 2: Start API/MCP servers in background tmux with env vars
		fmt.Println("Starting API/MCP servers in background...")
		serverSession := prefix + "servers"

		checkCmd := exec.Command("tmux", "has-session", "-t", serverSession)
		if checkCmd.Run() != nil {
			logFile := fmt.Sprintf("%s/.assistant-to/logs/coordinator.log", pwd)
			os.MkdirAll(fmt.Sprintf("%s/.assistant-to/logs", pwd), 0755)
			// Pass MCP port via env var to the server
			serveCmd := exec.Command("tmux", "new-session", "-d", "-s", serverSession,
				fmt.Sprintf("mkdir -p %s/.assistant-to/logs && AT_MCP_PORT=%d %s serve > %s 2>&1", pwd, mcpPort, exePath, logFile))
			if err := serveCmd.Run(); err != nil {
				fmt.Printf("Failed to start servers: %v\n", err)
			} else {
				fmt.Printf("✓ API/MCP servers started on port %d (logs: %s)\n", mcpPort, logFile)
			}
		} else {
			fmt.Println("Servers already running")
		}

		// Step 3: Generate opencode.json with actual port (overwrites init-generated file)
		opencodeConfig := map[string]interface{}{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]interface{}{
				"assistant-to": map[string]interface{}{
					"type":    "local",
					"command": []string{"dwight", "mcp", "serve"},
					"enabled": true,
					"environment": map[string]string{
						"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPort),
						"AT_AGENT_ROLE":   "coordinator",
						"AT_PROJECT_ROOT": pwd,
					},
				},
			},
		}
		opencodePath := filepath.Join(pwd, "opencode.json")
		opencodeData, _ := json.MarshalIndent(opencodeConfig, "", "  ")
		if err := os.WriteFile(opencodePath, opencodeData, 0644); err != nil {
			fmt.Printf("Warning: failed to write opencode.json: %v\n", err)
		} else {
			fmt.Printf("✓ Generated opencode.json with port %d\n", mcpPort)
		}

		// Step 4: Start Coordinator AI agent with env vars
		fmt.Println("Starting Coordinator agent in tmux...")
		coordSession := prefix + "coordinator"

		checkCmd = exec.Command("tmux", "has-session", "-t", coordSession)
		if checkCmd.Run() == nil {
			fmt.Printf("Session %s already exists. Attaching...\n", coordSession)
		} else {
			// Spawn the Coordinator AI agent with env vars
			coordCmd := exec.Command("tmux", "new-session", "-d", "-s", coordSession,
				fmt.Sprintf("AT_MCP_PORT=%d AT_PROJECT_ROOT=%s %s run Coordinator --role Coordinator 2>&1", mcpPort, pwd, exePath))
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
		fmt.Println("🎯 Opening Dashboard automatically...")

		// Auto-attach to the dashboard tmux session
		tmuxCmd := "attach-session"
		if os.Getenv("TMUX") != "" {
			tmuxCmd = "switch-client"
		}

		c := exec.Command("tmux", tmuxCmd, "-t", dashSession)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("Failed to attach to dashboard: %v\n", err)
			fmt.Println("You can manually attach with: dwight attach")
		}
	},
}

// serveCmd runs the coordinator infrastructure only (API/MCP servers)
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run coordinator infrastructure only (internal)",
	Long:  `Runs the coordinator infrastructure (API/MCP servers) without spawning agents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pwd, err := findProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to find project root: %w", err)
		}

		// Create and run the coordinator which starts API/MCP servers
		coord, err := orchestrator.NewCoordinator(pwd)
		if err != nil {
			return fmt.Errorf("failed to create coordinator: %w", err)
		}

		// Run in indefinite mode to keep servers running
		coord.RunIndefinitely = true

		fmt.Println("Starting coordinator infrastructure...")
		fmt.Println("Press Ctrl+C to stop.")

		return coord.Run(cmd.Context())
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
