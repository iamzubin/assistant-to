package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Mail type constants for the typed protocol
const (
	MailTypeDispatch   = "dispatch"
	MailTypeWorkerDone = "worker_done"
	MailTypeMergeReady = "merge_ready"
	MailTypeEscalation = "escalation"
	MailTypeStatus     = "status"
	MailTypeQuestion   = "question"
	MailTypeResult     = "result"
	MailTypeError      = "error"
)

// Priority levels (1=critical, 5=low)
const (
	PriorityCritical = 1
	PriorityHigh     = 2
	PriorityNormal   = 3
	PriorityLow      = 4
	PriorityTrivial  = 5
)

// Mail represents a message sent between agents or the system
type Mail struct {
	ID        int
	Sender    string
	Recipient string
	Subject   string
	Body      string
	Type      string
	Priority  int
	IsRead    bool
	Timestamp time.Time
}

// SendMail inserts a new message into the mail table with type and priority
func (d *DB) SendMail(sender, recipient, subject, body, mailType string, priority int) error {
	query := `
		INSERT INTO mail (sender, recipient, subject, body, type, priority)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := d.Exec(query, sender, recipient, subject, body, mailType, priority)
	if err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}
	return nil
}

// SendMailSimple sends mail with default type (status) and priority (normal)
func (d *DB) SendMailSimple(sender, recipient, subject, body string) error {
	return d.SendMail(sender, recipient, subject, body, MailTypeStatus, PriorityNormal)
}

// GetUnreadMail retrieves all unread messages for a specific agent
func (d *DB) GetUnreadMail(agentID string) ([]Mail, error) {
	query := `
		SELECT id, sender, recipient, subject, body, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Mail, error) {
		var m Mail
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &m.Body, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		return m, err
	}, agentID)
}

// GetUnreadMailByType retrieves unread messages filtered by type
func (d *DB) GetUnreadMailByType(agentID, mailType string) ([]Mail, error) {
	query := `
		SELECT id, sender, recipient, subject, body, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND type = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Mail, error) {
		var m Mail
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &m.Body, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		return m, err
	}, agentID, mailType)
}

// MarkMailRead updates a message's status to read
func (d *DB) MarkMailRead(mailID int) error {
	query := `
		UPDATE mail
		SET is_read = TRUE
		WHERE id = ?
	`
	_, err := d.Exec(query, mailID)
	if err != nil {
		return fmt.Errorf("failed to mark mail as read: %w", err)
	}
	return nil
}

// MarkAllMailRead marks all unread messages for an agent as read
func (d *DB) MarkAllMailRead(agentID string) error {
	query := `
		UPDATE mail
		SET is_read = TRUE
		WHERE recipient = ? AND is_read = FALSE
	`
	_, err := d.Exec(query, agentID)
	if err != nil {
		return fmt.Errorf("failed to mark mail as read: %w", err)
	}
	return nil
}

// GetMailCount returns the count of unread messages for an agent
func (d *DB) GetMailCount(agentID string) (int, error) {
	query := `
		SELECT COUNT(*) FROM mail
		WHERE recipient = ? AND is_read = FALSE
	`
	var count int
	err := d.QueryRow(query, agentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get mail count: %w", err)
	}
	return count, nil
}
