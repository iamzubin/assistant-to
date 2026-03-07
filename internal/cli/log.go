package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistant-to/internal/db"

	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log <message>",
	Short: "Record an event to the global timeline",
	Args:  cobra.ExactArgs(1),
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

		message := strings.Join(args, " ")
		agentID := os.Getenv("AGENT_ID")
		if agentID == "" {
			agentID = "Unknown"
		}

		query := `INSERT INTO events (agent_id, event_type, details) VALUES (?, 'log', ?)`
		_, err = database.Exec(query, agentID, message)
		if err != nil {
			fmt.Printf("Failed to write log: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Log recorded.")
	},
}

func init() {
	RootCmd.AddCommand(logCmd)
}
