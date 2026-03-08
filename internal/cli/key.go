package cli

import (
	"fmt"
	"os"
	"strconv"

	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key <task-id> <key-type> [count]",
	Short: "Send special keys to an agent's tmux session",
	Long: `Send special keys (escape, ctrl-c) to an agent's tmux session.
This command is strictly reserved for the Coordinator to control child processes.

Key types:
  - escape: Send Escape key (useful for interrupting processes)
  - ctrl-c: Send Ctrl+C (useful for terminating processes)
  - enter:  Send Enter key

The count parameter specifies how many times to send the key (default: 1).`,
	Args: cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		// Enforce Coordinator only
		if os.Getenv("AT_AGENT_ROLE") != "Coordinator" {
			fmt.Println("Error: Only the Coordinator can send special keys.")
			os.Exit(1)
		}

		taskID := args[0]
		keyType := args[1]
		count := 1

		if len(args) == 3 {
			var err error
			count, err = strconv.Atoi(args[2])
			if err != nil || count < 1 {
				fmt.Println("Error: Count must be a positive integer")
				os.Exit(1)
			}
		}

		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		sessionName := sandbox.ProjectPrefix(pwd) + taskID
		session := sandbox.TmuxSession{SessionName: sessionName}

		// Check if session exists
		if !session.HasSession() {
			fmt.Printf("Error: Session %s does not exist\n", sessionName)
			os.Exit(1)
		}

		var sendErr error
		switch keyType {
		case "escape":
			sendErr = session.SendEscape(count)
			if sendErr == nil {
				fmt.Printf("Sent Escape key %d time(s) to session %s\n", count, sessionName)
			}
		case "ctrl-c":
			for i := 0; i < count; i++ {
				sendErr = session.SendCtrlC()
				if sendErr != nil {
					break
				}
			}
			if sendErr == nil {
				fmt.Printf("Sent Ctrl+C %d time(s) to session %s\n", count, sessionName)
			}
		case "enter":
			keys := make([]string, 0, count)
			for i := 0; i < count; i++ {
				keys = append(keys, "C-m")
			}
			sendErr = session.SendKeys(keys...)
			if sendErr == nil {
				fmt.Printf("Sent Enter key %d time(s) to session %s\n", count, sessionName)
			}
		default:
			fmt.Printf("Error: Unknown key type '%s'. Use: escape, ctrl-c, or enter\n", keyType)
			os.Exit(1)
		}

		if sendErr != nil {
			fmt.Printf("Failed to send keys: %v\n", sendErr)
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(keyCmd)
}
