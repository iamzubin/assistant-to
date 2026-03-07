package cli

import (
	"fmt"
	"os"
	"os/exec"

	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Wake up the Coordinator to begin processing the task queue",
	Long: `Starts the Coordinator agent, which reads all pending tasks from the database
and spawns isolated Builder agents in tmux sessions inside git worktrees.
Each Builder receives the appropriate system prompt and model from config.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		// We spawn the Coordinator agent in a tmux session.
		// Unlike Builders, the Coordinator runs in the main project root.
		sessionName := sandbox.ProjectPrefix(pwd) + "Coordinator"

		// Use the 'at spawn' command logic internally or just call the binary.
		// For simplicity and to ensure it uses the same logic, we'll use 'at spawn'.
		// We use a special task-id 'Coordinator' which spawn.go should handle gracefully (it won't find a numeric ID to update status).

		fmt.Printf("Spawning Coordinator agent (session: %s)...\n", sessionName)

		// We'll call 'at spawn Coordinator --role Coordinator'
		// This ensures it gets the Coordinator prompt from agents.md
		spawnCmd := exec.Command("at", "spawn", "Coordinator", "--role", "Coordinator")
		spawnCmd.Stdout = os.Stdout
		spawnCmd.Stderr = os.Stderr

		if err := spawnCmd.Run(); err != nil {
			fmt.Printf("Failed to spawn Coordinator: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Coordinator started. Run `at dash` to monitor progress.")
	},
}

func init() {
	RootCmd.AddCommand(startCmd)
}
