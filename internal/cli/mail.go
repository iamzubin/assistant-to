package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"assistant-to/internal/db"

	"github.com/spf13/cobra"
)

var (
	mailTo      string
	mailSubject string
	mailBody    string
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Send or read messages from the inter-agent mailbox",
}

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to another agent",
	Run: func(cmd *cobra.Command, args []string) {
		pwd, _ := os.Getwd()
		dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		query := `INSERT INTO mail (sender, recipient, subject, body) VALUES ('User', ?, ?, ?)`
		_, err = database.Exec(query, mailTo, mailSubject, mailBody)
		if err != nil {
			fmt.Printf("Failed to send mail: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Mail sent to %s\n", mailTo)
	},
}

var mailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in the mailbox",
	Run: func(cmd *cobra.Command, args []string) {
		pwd, _ := os.Getwd()
		dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		var rows *sql.Rows
		var query string
		if mailTo != "" {
			query = `SELECT sender, subject, body, timestamp FROM mail WHERE recipient = ? ORDER BY timestamp DESC`
			rows, err = database.Query(query, mailTo)
		} else {
			query = `SELECT sender, subject, body, timestamp FROM mail ORDER BY timestamp DESC`
			rows, err = database.Query(query)
		}

		if err != nil {
			fmt.Printf("Failed to query mail: %v\n", err)
			os.Exit(1)
		}
		defer rows.Close()

		fmt.Printf("%-15s | %-20s | %s\n", "Sender", "Subject", "Time")
		fmt.Println("------------------------------------------------------------")
		for rows.Next() {
			var sender, subject, body, timestamp string
			rows.Scan(&sender, &subject, &body, &timestamp)
			fmt.Printf("%-15s | %-20s | %s\n", sender, subject, timestamp)
		}
	},
}

func init() {
	mailSendCmd.Flags().StringVar(&mailTo, "to", "", "Recipient agent role")
	mailSendCmd.Flags().StringVar(&mailSubject, "subject", "", "Message subject")
	mailSendCmd.Flags().StringVar(&mailBody, "body", "", "Message body")
	mailSendCmd.MarkFlagRequired("to")
	mailSendCmd.MarkFlagRequired("subject")
	mailSendCmd.MarkFlagRequired("body")

	mailListCmd.Flags().StringVar(&mailTo, "to", "", "Filter by recipient")

	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailListCmd)
	RootCmd.AddCommand(mailCmd)
}
