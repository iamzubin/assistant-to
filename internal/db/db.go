package db

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB Wrapper for the sqlite instance
type DB struct {
	*sql.DB
}

// Open initializes the SQLite database at the given path
func Open(dbPath string) (*DB, error) {
	// Ensure the path is absolute or resolved properly
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve db path: %w", err)
	}

	// Open the database with PRAGMAs in the DSN to ensure they apply to all connections in the pool
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", absPath)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &DB{DB: database}, nil
}

// InitSchema creates the necessary tables if they don't exist
func (d *DB) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS mail (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL,
		subject TEXT NOT NULL,
		body TEXT NOT NULL,
		type TEXT DEFAULT 'status',
		priority INTEGER DEFAULT 5,
		is_read BOOLEAN DEFAULT FALSE,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		parent_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		target_files TEXT,
		status TEXT DEFAULT 'pending',
		priority INTEGER DEFAULT 3,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		details TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS expertise (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT NOT NULL,
		type TEXT NOT NULL,
		description TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS token_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		task_id INTEGER,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		cost_usd REAL DEFAULT 0,
		model TEXT,
		session_count INTEGER DEFAULT 1,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_mail_recipient ON mail(recipient);
	CREATE INDEX IF NOT EXISTS idx_mail_type ON mail(type);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent_id);
	CREATE INDEX IF NOT EXISTS idx_expertise_domain ON expertise(domain);
	CREATE INDEX IF NOT EXISTS idx_expertise_type ON expertise(type);
	CREATE INDEX IF NOT EXISTS idx_token_metrics_agent ON token_metrics(agent_id);
	CREATE INDEX IF NOT EXISTS idx_token_metrics_task ON token_metrics(task_id);
	`
	_, err := d.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Enable foreign keys
	_, err = d.Exec("PRAGMA foreign_keys = ON;")
	return err
}
