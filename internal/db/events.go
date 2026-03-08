package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Event represents an audit log entry or heartbeat from an agent
type Event struct {
	ID        int
	AgentID   string
	EventType string
	Details   string
	Timestamp time.Time
}

// RecordEvent logs an action, tool call, or heartbeat from an agent
func (d *DB) RecordEvent(agentID, eventType, details string) error {
	query := `
		INSERT INTO events (agent_id, event_type, details)
		VALUES (?, ?, ?)
	`
	_, err := d.Exec(query, agentID, eventType, details)
	if err != nil {
		return fmt.Errorf("failed to record event: %w", err)
	}
	return nil
}

// GetAgentHistory retrieves the chronological event log for a specific agent
func (d *DB) GetAgentHistory(agentID string) ([]Event, error) {
	query := `
		SELECT id, agent_id, event_type, details, timestamp
		FROM events
		WHERE agent_id = ?
		ORDER BY timestamp ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Event, error) {
		var e Event
		err := rows.Scan(&e.ID, &e.AgentID, &e.EventType, &e.Details, &e.Timestamp)
		return e, err
	}, agentID)
}

// GetLastHeartbeat retrieves the timestamp of the last event for an agent
func (d *DB) GetLastHeartbeat(agentID string) (time.Time, error) {
	query := `
		SELECT MAX(timestamp) 
		FROM events 
		WHERE agent_id = ?
	`
	var lastSeen sql.NullString
	err := d.QueryRow(query, agentID).Scan(&lastSeen)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last heartbeat: %w", err)
	}

	if !lastSeen.Valid || lastSeen.String == "" {
		return time.Time{}, nil
	}

	// SQLite CURRENT_TIMESTAMP formatting
	parsedTime, err := time.Parse("2006-01-02 15:04:05", lastSeen.String)
	if err != nil {
		// Fallback for RFC3339 in case of Go native time string
		parsedTime, err = time.Parse(time.RFC3339, lastSeen.String)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
		}
	}

	return parsedTime, nil
}
