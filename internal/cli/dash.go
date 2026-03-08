package cli

import (
	"fmt"
	"path/filepath"

	"assistant-to/internal/db"
	"assistant-to/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Open the director's live dashboard",
	Long:  `Launches a terminal user interface (TUI) to monitor tasks, agents, and system events in real-time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDash()
	},
}

func init() {
	RootCmd.AddCommand(dashCmd)
}

func runDash() error {
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}
	dbPath := filepath.Join(root, ".assistant-to", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open workspace database: %w\nMake sure you have run 'at init' first.", err)
	}
	defer database.Close()

	model := tui.NewDashModel(database, root)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running dashboard: %w", err)
	}

	return nil
}
