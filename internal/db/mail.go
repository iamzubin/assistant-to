package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

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

const (
	PriorityCritical = 1
	PriorityHigh     = 2
	PriorityNormal   = 3
	PriorityLow      = 4
	PriorityTrivial  = 5
)

const MaxBodyInlineSize = 4096

type Mail struct {
	ID        int       `json:"id"`
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	BodyPath  string    `json:"body_path,omitempty"`
	Type      string    `json:"type"`
	Priority  int       `json:"priority"`
	IsRead    bool      `json:"is_read"`
	Timestamp time.Time `json:"timestamp"`
}

type MailSummary struct {
	ID        int       `json:"id"`
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"`
	Subject   string    `json:"subject"`
	BodyTrunc string    `json:"body_trunc"`
	Type      string    `json:"type"`
	Priority  int       `json:"priority"`
	IsRead    bool      `json:"is_read"`
	Timestamp time.Time `json:"timestamp"`
	HasBody   bool      `json:"has_body"`
}

const bodyTruncLength = 200

func (m *Mail) GetBody() string {
	if m.Body != "" {
		return m.Body
	}
	if m.BodyPath != "" {
		data, err := os.ReadFile(m.BodyPath)
		if err == nil {
			return string(data)
		}
	}
	return ""
}

// SendMail inserts a new message into the mail table with type and priority
func (d *DB) SendMail(sender, recipient, subject, body, mailType string, priority int) error {
	return d.SendMailWithStorage(sender, recipient, subject, body, mailType, priority, "")
}

// SendMailWithStorage inserts a new message with optional file-based body storage
func (d *DB) SendMailWithStorage(sender, recipient, subject, body, mailType string, priority int, storageDir string) error {
	var bodyPath string

	if len(body) > MaxBodyInlineSize && storageDir != "" {
		result, err := d.Exec(`
			INSERT INTO mail (sender, recipient, subject, body, body_path, type, priority)
			VALUES (?, ?, ?, '', ?, ?, ?)
		`, sender, recipient, subject, bodyPath, mailType, priority)
		if err != nil {
			return fmt.Errorf("failed to send mail: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get mail ID: %w", err)
		}

		bodyPath = filepath.Join(storageDir, fmt.Sprintf("mail_%d.body", id))
		if err := os.WriteFile(bodyPath, []byte(body), 0644); err != nil {
			return fmt.Errorf("failed to write mail body to file: %w", err)
		}

		_, err = d.Exec("UPDATE mail SET body_path = ? WHERE id = ?", bodyPath, id)
		if err != nil {
			return fmt.Errorf("failed to update body path: %w", err)
		}

		return nil
	}

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
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Mail, error) {
		var m Mail
		var body sql.NullString
		var bodyPath sql.NullString
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		if err != nil {
			return m, err
		}
		m.Body = body.String
		m.BodyPath = bodyPath.String
		return m, nil
	}, agentID)
}

// GetUnreadMailByType retrieves unread messages filtered by type
func (d *DB) GetUnreadMailByType(agentID, mailType string) ([]Mail, error) {
	query := `
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND type = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Mail, error) {
		var m Mail
		var body sql.NullString
		var bodyPath sql.NullString
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		if err != nil {
			return m, err
		}
		m.Body = body.String
		m.BodyPath = bodyPath.String
		return m, nil
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

func truncateBody(body string) string {
	if len(body) <= bodyTruncLength {
		return body
	}
	return body[:bodyTruncLength] + "..."
}

func (d *DB) GetUnreadMailSummaries(agentID string, limit, offset int) ([]MailSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
		LIMIT ? OFFSET ?
	`

	rows, err := d.Query(query, agentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get mail summaries: %w", err)
	}
	defer rows.Close()

	var summaries []MailSummary
	for rows.Next() {
		var m MailSummary
		var body sql.NullString
		var bodyPath sql.NullString
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail: %w", err)
		}

		bodyStr := body.String
		if bodyPath.Valid && bodyPath.String != "" {
			data, err := os.ReadFile(bodyPath.String)
			if err == nil {
				bodyStr = string(data)
			}
		}

		m.BodyTrunc = truncateBody(bodyStr)
		m.HasBody = bodyStr != ""
		summaries = append(summaries, m)
	}

	return summaries, nil
}

func (d *DB) GetUnreadMailSummariesByType(agentID, mailType string, limit, offset int) ([]MailSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND type = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
		LIMIT ? OFFSET ?
	`

	rows, err := d.Query(query, agentID, mailType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get mail summaries: %w", err)
	}
	defer rows.Close()

	var summaries []MailSummary
	for rows.Next() {
		var m MailSummary
		var body sql.NullString
		var bodyPath sql.NullString
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail: %w", err)
		}

		bodyStr := body.String
		if bodyPath.Valid && bodyPath.String != "" {
			data, err := os.ReadFile(bodyPath.String)
			if err == nil {
				bodyStr = string(data)
			}
		}

		m.BodyTrunc = truncateBody(bodyStr)
		m.HasBody = bodyStr != ""
		summaries = append(summaries, m)
	}

	return summaries, nil
}

func (d *DB) GetMailByID(mailID int) (*Mail, error) {
	query := `
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE id = ?
	`

	var m Mail
	var body sql.NullString
	var bodyPath sql.NullString
	err := d.QueryRow(query, mailID).Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get mail by ID: %w", err)
	}

	m.Body = body.String
	m.BodyPath = bodyPath.String
	return &m, nil
}

func (d *DB) GetUnreadMailPaginated(agentID string, limit, offset int) ([]Mail, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, sender, recipient, subject, body, body_path, type, priority, is_read, timestamp
		FROM mail
		WHERE recipient = ? AND is_read = FALSE
		ORDER BY priority ASC, timestamp ASC
		LIMIT ? OFFSET ?
	`

	rows, err := d.Query(query, agentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get mail: %w", err)
	}
	defer rows.Close()

	var mailList []Mail
	for rows.Next() {
		var m Mail
		var body sql.NullString
		var bodyPath sql.NullString
		err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.Subject, &body, &bodyPath, &m.Type, &m.Priority, &m.IsRead, &m.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail: %w", err)
		}

		bodyStr := body.String
		if bodyPath.Valid && bodyPath.String != "" {
			data, err := os.ReadFile(bodyPath.String)
			if err == nil {
				bodyStr = string(data)
			}
		}

		m.Body = bodyStr
		m.BodyPath = bodyPath.String
		mailList = append(mailList, m)
	}

	return mailList, nil
}
