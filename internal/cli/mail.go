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
	mailTo       string
	mailSubject  string
	mailBody     string
	mailFrom     string
	mailInject   bool
	mailType     string
	mailPriority int
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Send or read messages from the inter-agent mailbox",
}

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to another agent",
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

		sender := os.Getenv("AT_AGENT_ROLE")
		if sender == "" {
			if mailFrom != "" {
				sender = mailFrom
			} else {
				sender = "User"
			}
		}

		// Determine mail type and priority
		msgType := mailType
		if msgType == "" {
			msgType = db.MailTypeStatus
		}
		priority := mailPriority
		if priority == 0 {
			priority = db.PriorityNormal
		}

		query := `INSERT INTO mail (sender, recipient, subject, body, type, priority) VALUES (?, ?, ?, ?, ?, ?)`
		_, err = database.Exec(query, sender, mailTo, mailSubject, mailBody, msgType, priority)
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

		var rows *sql.Rows
		var query string
		if mailTo != "" {
			query = `SELECT sender, subject, body, type, priority, timestamp FROM mail WHERE recipient = ? ORDER BY timestamp DESC`
			rows, err = database.Query(query, mailTo)
		} else {
			query = `SELECT sender, subject, body, type, priority, timestamp FROM mail ORDER BY timestamp DESC`
			rows, err = database.Query(query)
		}

		if err != nil {
			fmt.Printf("Failed to query mail: %v\n", err)
			os.Exit(1)
		}
		defer rows.Close()

		fmt.Println("------------------------------------------------------------")
		for rows.Next() {
			var sender, subject, body, msgType string
			var priority int
			var timestamp string
			rows.Scan(&sender, &subject, &body, &msgType, &priority, &timestamp)
			fmt.Printf("From: %s\nDate: %s\nType: %s | Priority: %d\nSubject: %s\n\n%s\n", sender, timestamp, msgType, priority, subject, body)
			fmt.Println("------------------------------------------------------------")
		}
	},
}

var mailCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for unread messages (with optional --inject for agents)",
	Long: `Check retrieves unread messages for an agent.

When using --inject, messages are formatted for agent tool consumption,
marked as read, and output as structured data that agents can parse.`,
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

		// Determine recipient
		recipient := os.Getenv("AT_AGENT_ROLE")
		if recipient == "" {
			if mailTo != "" {
				recipient = mailTo
			} else {
				recipient = "User"
			}
		}

		// Get unread mail
		mail, err := database.GetUnreadMail(recipient)
		if err != nil {
			fmt.Printf("Failed to get unread mail: %v\n", err)
			os.Exit(1)
		}

		if len(mail) == 0 {
			if mailInject {
				// Output empty JSON array for programmatic consumption
				fmt.Println("[]")
			} else {
				fmt.Println("No unread messages.")
			}
			return
		}

		if mailInject {
			// Output structured format for agent consumption and mark as read
			fmt.Printf("[SYSTEM] You have %d unread message(s):\n\n", len(mail))
			for i, m := range mail {
				fmt.Printf("--- Message %d ---\n", i+1)
				fmt.Printf("From: %s\n", m.Sender)
				fmt.Printf("Subject: %s\n", m.Subject)
				fmt.Printf("Type: %s\n", m.Type)
				fmt.Printf("Priority: %d\n", m.Priority)
				fmt.Printf("Body:\n%s\n\n", m.Body)

				// Mark as read after injection
				if err := database.MarkMailRead(m.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to mark mail %d as read: %v\n", m.ID, err)
				}
			}
		} else {
			// Simple human-readable output, don't mark as read
			fmt.Printf("You have %d unread message(s):\n\n", len(mail))
			for _, m := range mail {
				fmt.Printf("[%s | Priority: %d] From: %s | Subject: %s\n", m.Type, m.Priority, m.Sender, m.Subject)
			}
		}
	},
}

func init() {
	mailSendCmd.Flags().StringVar(&mailTo, "to", "", "Recipient agent role")
	mailSendCmd.Flags().StringVar(&mailFrom, "from", "", "Sender agent role (optional, defaults to AT_AGENT_ROLE or User)")
	mailSendCmd.Flags().StringVar(&mailSubject, "subject", "", "Message subject")
	mailSendCmd.Flags().StringVar(&mailBody, "body", "", "Message body")
	mailSendCmd.Flags().StringVar(&mailType, "type", "status", "Message type (dispatch, worker_done, merge_ready, escalation, status, question, result, error)")
	mailSendCmd.Flags().IntVar(&mailPriority, "priority", 3, "Message priority (1=critical, 5=low)")
	mailSendCmd.MarkFlagRequired("to")
	mailSendCmd.MarkFlagRequired("subject")
	mailSendCmd.MarkFlagRequired("body")

	mailListCmd.Flags().StringVar(&mailTo, "to", "", "Filter by recipient")

	mailCheckCmd.Flags().StringVar(&mailTo, "to", "", "Agent role to check mail for (defaults to AT_AGENT_ROLE env var or 'User')")
	mailCheckCmd.Flags().BoolVar(&mailInject, "inject", false, "Inject messages into agent context and mark as read")

	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailListCmd)
	mailCmd.AddCommand(mailCheckCmd)
	RootCmd.AddCommand(mailCmd)
}
