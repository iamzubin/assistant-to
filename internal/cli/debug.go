package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug and inspect agent state",
	Long:  `Commands for debugging and inspecting agent sessions, buffers, and state.`,
}

var bufferLines int
var bufferAgent string

var bufferCmd = &cobra.Command{
	Use:   "buffer [agent-id]",
	Short: "Capture and display an agent's tmux buffer",
	Long: `Captures the last N lines of an agent's tmux session buffer.
This is useful for debugging when an agent hasn't sent mail in a while,
is waiting for user input, or appears stuck.

Examples:
  dwight debug buffer builder-1           # Last 20 lines
  dwight debug buffer builder-1 --lines 100 # Last 100 lines
  dwight debug buffer Coordinator          # Coordinator buffer`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		agentID := bufferAgent
		if agentID == "" && len(args) > 0 {
			agentID = args[0]
		}
		if agentID == "" {
			fmt.Println("Error: agent-id required")
			os.Exit(1)
		}

		prefix := sandbox.ProjectPrefix(pwd)
		sessionName := prefix + agentID

		session := &sandbox.TmuxSession{SessionName: sessionName}

		if !session.HasSession() {
			fmt.Printf("Error: Session '%s' does not exist\n", sessionName)
			os.Exit(1)
		}

		if bufferLines <= 0 {
			bufferLines = 20
		}

		output, err := session.CaptureBuffer(bufferLines)
		if err != nil {
			fmt.Printf("Error capturing buffer: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("=== Buffer for %s (last %d lines) ===\n\n", agentID, bufferLines)
		fmt.Println(output)
	},
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage agent tmux sessions",
	Long:  `Commands to inspect, kill, and manage agent tmux sessions.`,
}

var sessionKillCmd = &cobra.Command{
	Use:   "kill <agent-id>",
	Short: "Forcefully kill an agent's tmux session",
	Long: `Kills an agent's tmux session immediately. Use this when an agent is stuck,
looping indefinitely, or not responding to other recovery attempts.

After killing, consider using 'dwight worktree teardown' to clean up resources.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		agentID := args[0]
		prefix := sandbox.ProjectPrefix(pwd)
		sessionName := prefix + agentID

		session := &sandbox.TmuxSession{SessionName: sessionName}

		if !session.HasSession() {
			fmt.Printf("Session '%s' does not exist\n", sessionName)
			os.Exit(1)
		}

		fmt.Printf("Killing session '%s'...\n", sessionName)
		if err := session.Kill(); err != nil {
			fmt.Printf("Error killing session: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Session '%s' killed successfully.\n", agentID)
	},
}

var sessionClearCmd = &cobra.Command{
	Use:   "clear <agent-id>",
	Short: "Clear an agent's tmux buffer/terminal",
	Long: `Clears an agent's tmux pane to start fresh. Useful after task completion
or to remove noisy output from previous operations.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		agentID := args[0]
		prefix := sandbox.ProjectPrefix(pwd)
		sessionName := prefix + agentID

		session := &sandbox.TmuxSession{SessionName: sessionName}

		if !session.HasSession() {
			fmt.Printf("Session '%s' does not exist\n", sessionName)
			os.Exit(1)
		}

		if err := session.ClearBuffer(); err != nil {
			fmt.Printf("Error clearing buffer: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Buffer cleared for session '%s'.\n", agentID)
	},
}

var sessionSendCmd = &cobra.Command{
	Use:   "send <agent-id> <text...>",
	Short: "Send input/keystrokes to an agent's tmux session",
	Long: `Sends keystrokes directly to an agent's tmux session buffer.
This is useful for:
- Sending 'y' to confirm prompts
- Sending Ctrl+C to interrupt stuck processes
- Sending arbitrary input when an agent is waiting for user input

Examples:
  dwight debug session send builder-1 y
  dwight debug session send builder-1 "^C"       # Send Ctrl+C
  dwight debug session send builder-1 "n\\r"     # Send 'n' + Enter
  dwight debug session send Coordinator hello\\r   # Send to Coordinator`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		agentID := args[0]
		input := args[1]

		prefix := sandbox.ProjectPrefix(pwd)
		sessionName := prefix + agentID

		session := &sandbox.TmuxSession{SessionName: sessionName}

		if !session.HasSession() {
			fmt.Printf("Session '%s' does not exist\n", sessionName)
			os.Exit(1)
		}

		if err := session.SendInput(input); err != nil {
			fmt.Printf("Error sending input: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Sent input to session '%s': %q\n", agentID, input)
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active agent tmux sessions",
	Long: `Lists all running assistant-to tmux sessions with their status.
Shows agent ID, session name, and basic info.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		prefix := sandbox.ProjectPrefix(pwd)

		sessions, err := sandbox.ListSessions(prefix)
		if err != nil {
			fmt.Printf("Error listing sessions: %v\n", err)
			os.Exit(1)
		}

		if len(sessions) == 0 {
			fmt.Println("No active assistant-to sessions found.")
			return
		}

		fmt.Printf("=== Active Sessions (prefix: %s) ===\n\n", prefix)
		for _, s := range sessions {
			fmt.Printf("- %s\n", s)
		}
	},
}

var cleanupTaskID string
var cleanupAll bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [task-id]",
	Short: "Clean up completed task resources",
	Long: `Cleans up resources after a task is complete:
- Kills the agent tmux session
- Removes the git worktree
- Marks task as complete if not already

Use --all to clean up all completed tasks.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if cleanupAll {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		prefix := sandbox.ProjectPrefix(pwd)

		if cleanupAll {
			fmt.Println("Cleaning up all completed tasks...")
			tasks, err := database.ListTasksByStatus("")
			if err != nil {
				fmt.Printf("Error listing tasks: %v\n", err)
				os.Exit(1)
			}

			cleaned := 0
			for _, task := range tasks {
				if task.Status == "complete" || task.Status == "review" {
					taskID := fmt.Sprintf("%d", task.ID)
					sessionName := prefix + taskID

					session := &sandbox.TmuxSession{SessionName: sessionName}
					if session.HasSession() {
						session.Kill()
						fmt.Printf("Killed session: %s\n", taskID)
					}

					sandbox.TeardownWorktree(taskID, pwd, "")
					fmt.Printf("Cleaned up worktree: %s\n", taskID)
					cleaned++
				}
			}
			fmt.Printf("Cleaned up %d tasks.\n", cleaned)
			return
		}

		taskID := args[0]
		sessionName := prefix + taskID

		fmt.Printf("Cleaning up task %s...\n", taskID)

		session := &sandbox.TmuxSession{SessionName: sessionName}
		if session.HasSession() {
			session.Kill()
			fmt.Printf("Killed session: %s\n", taskID)
		}

		if err := sandbox.TeardownWorktree(taskID, pwd, ""); err != nil {
			fmt.Printf("Warning: Failed to teardown worktree: %v\n", err)
		} else {
			fmt.Printf("Worktree cleaned up: %s\n", taskID)
		}

		fmt.Printf("Task %s cleanup complete.\n", taskID)
	},
}

func init() {
	bufferCmd.Flags().IntVarP(&bufferLines, "lines", "n", 20, "Number of lines to capture")
	bufferCmd.Flags().StringVarP(&bufferAgent, "agent", "a", "", "Agent ID (e.g., builder-1, Coordinator)")

	cleanupCmd.Flags().BoolVarP(&cleanupAll, "all", "a", false, "Clean up all completed tasks")

	debugCmd.AddCommand(bufferCmd)
	debugCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionKillCmd)
	sessionCmd.AddCommand(sessionClearCmd)
	sessionCmd.AddCommand(sessionSendCmd)
	sessionCmd.AddCommand(sessionListCmd)
	debugCmd.AddCommand(cleanupCmd)

	RootCmd.AddCommand(debugCmd)
}
