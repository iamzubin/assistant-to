package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "at",
	Short: "assistant-to: The Managing Director's Autonomous Coding Swarm",
	Long:  `A strictly bound, multi-agent orchestrator shipped as a single compiled Go binary.`,
}

func init() {
	// Add subcommands here as they are developed
}

// Execute is the entrypoint for the CLI.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}
