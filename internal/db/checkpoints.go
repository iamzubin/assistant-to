package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	CheckpointsExpiryHours = 24
)

type Checkpoint struct {
	ID              int       `json:"id"`
	TaskID          int       `json:"task_id"`
	AgentRole       string    `json:"agent_role"`
	AgentIdentity   string    `json:"agent_identity"`
	ContextSnapshot string    `json:"context_snapshot"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (d *DB) CheckpointSave(taskID int, role, identity string, context interface{}) error {
	contextJSON, err := json.Marshal(context)
	if err != nil {
		return fmt.Errorf("failed to serialize context: %w", err)
	}

	expiresAt := time.Now().Add(CheckpointsExpiryHours * time.Hour)

	query := `
		INSERT INTO checkpoints (task_id, agent_role, agent_identity, context_snapshot, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err = d.Exec(query, taskID, role, identity, string(contextJSON), expiresAt)
	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

func (d *DB) CheckpointLoad(taskID int, role string) (*Checkpoint, error) {
	query := `
		SELECT id, task_id, agent_role, agent_identity, context_snapshot, created_at, expires_at
		FROM checkpoints
		WHERE task_id = ? AND agent_role = ? AND expires_at > datetime('now')
		ORDER BY created_at DESC
		LIMIT 1
	`

	var cp Checkpoint
	err := d.QueryRow(query, taskID, role).Scan(
		&cp.ID,
		&cp.TaskID,
		&cp.AgentRole,
		&cp.AgentIdentity,
		&cp.ContextSnapshot,
		&cp.CreatedAt,
		&cp.ExpiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	return &cp, nil
}

func (d *DB) CheckpointDelete(taskID int, role string) error {
	query := `DELETE FROM checkpoints WHERE task_id = ? AND agent_role = ?`
	_, err := d.Exec(query, taskID, role)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}
	return nil
}

func (d *DB) CheckpointDeleteByTaskID(taskID int) error {
	query := `DELETE FROM checkpoints WHERE task_id = ?`
	_, err := d.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoints for task: %w", err)
	}
	return nil
}

func (d *DB) CheckpointListByTaskID(taskID int) ([]Checkpoint, error) {
	query := `
		SELECT id, task_id, agent_role, agent_identity, context_snapshot, created_at, expires_at
		FROM checkpoints
		WHERE task_id = ? AND expires_at > datetime('now')
		ORDER BY created_at DESC
	`

	rows, err := d.Query(query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []Checkpoint
	for rows.Next() {
		var cp Checkpoint
		err := rows.Scan(
			&cp.ID,
			&cp.TaskID,
			&cp.AgentRole,
			&cp.AgentIdentity,
			&cp.ContextSnapshot,
			&cp.CreatedAt,
			&cp.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
		}
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, nil
}

func (d *DB) CleanupExpiredCheckpoints() (int64, error) {
	result, err := d.Exec("DELETE FROM checkpoints WHERE expires_at < datetime('now')")
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired checkpoints: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
