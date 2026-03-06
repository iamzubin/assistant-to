package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Mail represents a message sent between agents or the system
type Mail struct {
	ID        int
	Sender    string
	Recipient string
	Subject   string
	Body      string
	IsRead    bool
	Timestamp time.Time
}

// SendMail inserts a new message into the mail table
func (db *DB) SendMail(sender, recipient, subject, body string) error {
	query := `
		INSERT INTO mail (sender, recipient, subject, body)
		VALUES (?, ?, ?, ?)
	`
	_, err := db.Exec(query, sender, recipient, subject, body)
	if err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}
	return nil
}

// GetUnreadMail retrieves all unread messages for a specific agent
func (db *DB) GetUnreadMail(agentID string) ([]Mail, error) {
	query := `
		SELECT id, sender, recipient, subject, body, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND is_read = FALSE
		ORDER BY timestamp ASC
	`
	return queryList(db, query, func(rows *sql.Rows) (Mail, error) {
		var m Mail
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &m.Body, &m.IsRead, &m.Timestamp)
		return m, err
	}, agentID)
}

// MarkMailRead updates a message's status to read
func (db *DB) MarkMailRead(mailID int) error {
	query := `
		UPDATE mail
		SET is_read = TRUE
		WHERE id = ?
	`
	_, err := db.Exec(query, mailID)
	if err != nil {
		return fmt.Errorf("failed to mark mail as read: %w", err)
	}
	return nil
}
