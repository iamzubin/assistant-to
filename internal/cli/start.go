package cli

import (
	"context"
	"fmt"
	"os"

	"assistant-to/internal/orchestrator"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Wake up the Coordinator to begin processing the task queue",
	Long: `Starts the Coordinator agent, which reads all pending tasks from the database
and spawns isolated Builder agents in tmux sessions inside git worktrees.
Each Builder receives the appropriate system prompt and model from config.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting working directory: %v\n", err)
			os.Exit(1)
		}

		coord, err := orchestrator.NewCoordinator(pwd)
		if err != nil {
			fmt.Printf("Failed to initialize Coordinator: %v\n", err)
			os.Exit(1)
		}
		defer coord.DB.Close()

		if err := coord.Run(context.Background()); err != nil {
			fmt.Printf("Coordinator error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
