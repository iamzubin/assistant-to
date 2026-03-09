package orchestrator

import (
	"fmt"
	"time"

	"assistant-to/internal/db"
)

type MailIngestor struct {
	db *db.DB
}

func NewMailIngestor(database *db.DB) *MailIngestor {
	return &MailIngestor{db: database}
}

func (m *MailIngestor) Ingest(sender, recipient, subject, body, mailType string, priority int) error {
	return m.db.SendMail(sender, recipient, subject, body, mailType, priority)
}

func (m *MailIngestor) IngestSimple(sender, recipient, subject, body string) error {
	return m.db.SendMailSimple(sender, recipient, subject, body)
}

func (m *MailIngestor) GetUnread(agentID string, limit int) ([]db.Mail, error) {
	if limit <= 0 {
		limit = 50
	}
	mail, err := m.db.GetUnreadMail(agentID)
	if err != nil {
		return nil, err
	}
	if len(mail) > limit {
		mail = mail[:limit]
	}
	return mail, nil
}

func (m *MailIngestor) GetByType(agentID, mailType string, limit int) ([]db.Mail, error) {
	if limit <= 0 {
		limit = 50
	}
	mail, err := m.db.GetUnreadMailByType(agentID, mailType)
	if err != nil {
		return nil, err
	}
	if len(mail) > limit {
		mail = mail[:limit]
	}
	return mail, nil
}

func (m *MailIngestor) GetSummary(agentID string) (map[string]int, error) {
	summary := make(map[string]int)
	types := []string{
		db.MailTypeDispatch,
		db.MailTypeWorkerDone,
		db.MailTypeMergeReady,
		db.MailTypeEscalation,
		db.MailTypeStatus,
		db.MailTypeQuestion,
		db.MailTypeResult,
		db.MailTypeError,
	}
	for _, mailType := range types {
		mail, err := m.db.GetUnreadMailByType(agentID, mailType)
		if err != nil {
			return nil, fmt.Errorf("failed to get summary for type %s: %w", mailType, err)
		}
		summary[mailType] = len(mail)
	}
	return summary, nil
}

func (m *MailIngestor) MarkRead(mailID int) error {
	return m.db.MarkMailRead(mailID)
}

func (m *MailIngestor) MarkAllRead(agentID string) error {
	return m.db.MarkAllMailRead(agentID)
}

func (m *MailIngestor) Count(agentID string) (int, error) {
	return m.db.GetMailCount(agentID)
}

func (m *MailIngestor) PurgeOlderThan(agentID string, age time.Duration) (int, error) {
	cutoff := time.Now().Add(-age)
	query := `DELETE FROM mail WHERE recipient = ? AND timestamp < ?`
	result, err := m.db.Exec(query, agentID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to purge old mail: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return int(rowsAffected), nil
}

func (m *MailIngestor) PurgeOlderThanGlobal(age time.Duration) (int, error) {
	cutoff := time.Now().Add(-age)
	query := `DELETE FROM mail WHERE timestamp < ?`
	result, err := m.db.Exec(query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to purge old mail: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return int(rowsAffected), nil
}
