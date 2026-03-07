package cli

import (
	"assistant-to/internal/sandbox"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var haltCmd = &cobra.Command{
	Use:   "halt",
	Short: "Immediately kill all active assistant-to agent tmux sessions",
	Long:  `Searches for and forcefully terminates all running agent tmux sessions associated with the current project's workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHalt()
	},
}

func init() {
	rootCmd.AddCommand(haltCmd)
}

func runHalt() error {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#E8183C")).
		PaddingTop(1).
		PaddingBottom(1).
		PaddingLeft(4).
		PaddingRight(4).
		MarginBottom(1)

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB020"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A8A8A8"))

	fmt.Println(headerStyle.Render("assistant-to: Emergency Halt"))
	fmt.Println(infoStyle.Render("Searching for active agent sessions..."))

	// List all tmux sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(warningStyle.Render("No active tmux server found. No agents to kill."))
		return nil
	}

	sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
	killedCount := 0

	pwd, _ := os.Getwd()
	prefix := sandbox.ProjectPrefix(pwd)

	for _, session := range sessions {
		session = strings.TrimSpace(session)
		// We only want to kill sessions managed by the orchestrator prefix for this SPECIFIC project
		if strings.HasPrefix(session, prefix) {
			killCmd := exec.Command("tmux", "kill-session", "-t", session)
			if err := killCmd.Run(); err != nil {
				fmt.Printf("%s Failed to kill session %s: %v\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#E8183C")).Render("✕"), session, err)
			} else {
				fmt.Printf("%s Killed agent session: %s\n", successStyle.Render("✓"), session)
				killedCount++
			}
		}
	}

	fmt.Println()
	if killedCount == 0 {
		fmt.Println(warningStyle.Render("No active agent sessions found to halt."))
	} else {
		fmt.Println(successStyle.Render(fmt.Sprintf("Successfully halted %d active agent sessions.", killedCount)))
	}

	return nil
}
