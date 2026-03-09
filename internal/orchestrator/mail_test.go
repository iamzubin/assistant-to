package orchestrator

import (
	"os"
	"testing"
	"time"

	"assistant-to/internal/db"
)

func setupTestDB(t *testing.T) *db.DB {
	tmpFile, err := os.CreateTemp("", "test_mail_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := db.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := database.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
	return database
}

func TestMailIngestor_Ingest(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	err := ingestor.Ingest("sender", "recipient", "Test Subject", "Test Body", db.MailTypeStatus, db.PriorityNormal)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	count, err := ingestor.Count("recipient")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 mail, got %d", count)
	}
}

func TestMailIngestor_IngestSimple(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	err := ingestor.IngestSimple("sender", "recipient", "Test Subject", "Test Body")
	if err != nil {
		t.Fatalf("IngestSimple failed: %v", err)
	}

	mail, err := ingestor.GetUnread("recipient", 10)
	if err != nil {
		t.Fatalf("GetUnread failed: %v", err)
	}
	if len(mail) != 1 {
		t.Errorf("expected 1 mail, got %d", len(mail))
	}
	if mail[0].Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got '%s'", mail[0].Subject)
	}
}

func TestMailIngestor_GetUnread(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	for i := 0; i < 5; i++ {
		err := ingestor.IngestSimple("sender", "test-agent", "Subject", "Body")
		if err != nil {
			t.Fatalf("IngestSimple failed: %v", err)
		}
	}

	mail, err := ingestor.GetUnread("test-agent", 3)
	if err != nil {
		t.Fatalf("GetUnread failed: %v", err)
	}
	if len(mail) != 3 {
		t.Errorf("expected 3 mail, got %d", len(mail))
	}

	mail, err = ingestor.GetUnread("test-agent", 10)
	if err != nil {
		t.Fatalf("GetUnread failed: %v", err)
	}
	if len(mail) != 5 {
		t.Errorf("expected 5 mail (all), got %d", len(mail))
	}
}

func TestMailIngestor_GetByType(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	ingestor.Ingest("s1", "agent", "Subject 1", "Body", db.MailTypeStatus, db.PriorityNormal)
	ingestor.Ingest("s2", "agent", "Subject 2", "Body", db.MailTypeError, db.PriorityHigh)
	ingestor.Ingest("s3", "agent", "Subject 3", "Body", db.MailTypeStatus, db.PriorityNormal)

	mail, err := ingestor.GetByType("agent", db.MailTypeStatus, 10)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(mail) != 2 {
		t.Errorf("expected 2 status mails, got %d", len(mail))
	}

	mail, err = ingestor.GetByType("agent", db.MailTypeError, 10)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(mail) != 1 {
		t.Errorf("expected 1 error mail, got %d", len(mail))
	}
}

func TestMailIngestor_GetSummary(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	ingestor.Ingest("s1", "agent", "Subject 1", "Body", db.MailTypeStatus, db.PriorityNormal)
	ingestor.Ingest("s2", "agent", "Subject 2", "Body", db.MailTypeError, db.PriorityHigh)
	ingestor.Ingest("s3", "agent", "Subject 3", "Body", db.MailTypeStatus, db.PriorityNormal)

	summary, err := ingestor.GetSummary("agent")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}

	if summary[db.MailTypeStatus] != 2 {
		t.Errorf("expected 2 status, got %d", summary[db.MailTypeStatus])
	}
	if summary[db.MailTypeError] != 1 {
		t.Errorf("expected 1 error, got %d", summary[db.MailTypeError])
	}
}

func TestMailIngestor_MarkRead(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	ingestor.IngestSimple("sender", "agent", "Subject", "Body")

	mail, _ := ingestor.GetUnread("agent", 10)
	if len(mail) != 1 {
		t.Fatalf("expected 1 unread mail")
	}

	err := ingestor.MarkRead(mail[0].ID)
	if err != nil {
		t.Fatalf("MarkRead failed: %v", err)
	}

	mail, _ = ingestor.GetUnread("agent", 10)
	if len(mail) != 0 {
		t.Errorf("expected 0 unread after MarkRead, got %d", len(mail))
	}
}

func TestMailIngestor_MarkAllRead(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	for i := 0; i < 3; i++ {
		ingestor.IngestSimple("sender", "agent", "Subject", "Body")
	}

	err := ingestor.MarkAllRead("agent")
	if err != nil {
		t.Fatalf("MarkAllRead failed: %v", err)
	}

	count, _ := ingestor.Count("agent")
	if count != 0 {
		t.Errorf("expected 0 unread after MarkAllRead, got %d", count)
	}
}

func TestMailIngestor_PurgeOlderThan(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	_ = ingestor.IngestSimple("sender", "agent", "Old", "Body")
	_ = ingestor.IngestSimple("sender", "agent", "New", "Body")

	count, err := ingestor.PurgeOlderThan("agent", -time.Hour)
	if err != nil {
		t.Fatalf("PurgeOlderThan failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected to purge 2 mail (all are older than -1h), got %d", count)
	}

	remaining, _ := ingestor.Count("agent")
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

func TestMailIngestor_Count(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ingestor := NewMailIngestor(database)

	count, err := ingestor.Count("nonexistent")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", count)
	}

	for i := 0; i < 3; i++ {
		ingestor.IngestSimple("sender", "agent", "Subject", "Body")
	}

	count, err = ingestor.Count("agent")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}
