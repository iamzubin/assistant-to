package cli

import (
	"fmt"
	"os"
	"strings"

	"dwight/internal/sandbox"

	"github.com/spf13/cobra"
)

var inputCmd = &cobra.Command{
	Use:   "input <task-id> <message...>",
	Short: "Send input directly to an agent's tmux buffer",
	Long: `Injects keystrokes directly into an active agent's tmux session.
This command is strictly reserved for the Coordinator to stimulate child processes.`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// Enforce Coordinator only
		if os.Getenv("AT_AGENT_ROLE") != "Coordinator" {
			fmt.Println("Error: Only the Coordinator can send direct buffer input.")
			os.Exit(1)
		}

		taskID := args[0]
		message := strings.Join(args[1:], " ")

		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		sessionName := sandbox.ProjectPrefix(pwd) + taskID
		session := sandbox.TmuxSession{SessionName: sessionName}

		if err := session.SendInput(message); err != nil {
			fmt.Printf("Failed to send input: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Input sent successfully.")
	},
}

func init() {
	RootCmd.AddCommand(inputCmd)
}
