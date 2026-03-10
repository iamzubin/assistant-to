package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"dwight/internal/db"
	"dwight/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Open the director's live dashboard",
	Long: `Launches a terminal user interface (TUI) to monitor tasks, agents, and system events in real-time.

If no pending tasks exist, automatically spawns the Coordinator to start processing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDash(true) // Auto-spawn Coordinator by default
	},
}

func init() {
	RootCmd.AddCommand(dashCmd)
}

func runDash(autoSpawn bool) error {
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}
	dbPath := filepath.Join(root, ".dwight", "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open workspace database: %w\nMake sure you have run 'dwight init' first.", err)
	}
	defer database.Close()

	// Check if we need to spawn Coordinator
	if autoSpawn {
		tasks, err := database.ListTasksByStatus("pending")
		if err == nil && len(tasks) == 0 {
			fmt.Println("No pending tasks. Spawning Coordinator...")
			if err := spawnCoordinator(root); err != nil {
				fmt.Printf("Warning: failed to spawn Coordinator: %v\n", err)
			} else {
				fmt.Println("Coordinator spawned. Opening dashboard...")
			}
		}
	}

	model := tui.NewDashModel(database, root)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running dashboard: %w", err)
	}

	return nil
}

func spawnCoordinator(root string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	spawnCmd := exec.Command(exePath, "spawn", "Coordinator", "--role", "Coordinator")
	spawnCmd.Stdout = os.Stdout
	spawnCmd.Stderr = os.Stderr

	return spawnCmd.Start()
}
