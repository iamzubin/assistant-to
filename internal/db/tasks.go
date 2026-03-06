package db

import (
	"database/sql"
	"fmt"
)

// Task represents an autonomous work item
type Task struct {
	ID          int
	Title       string
	Description string
	TargetFiles string
	Status      string
}

// AddTask creates a new task in the database with status 'pending'
func (db *DB) AddTask(title, description, targetFiles string) (int64, error) {
	query := `
		INSERT INTO tasks (title, description, target_files, status)
		VALUES (?, ?, ?, 'pending')
	`
	res, err := db.Exec(query, title, description, targetFiles)
	if err != nil {
		return 0, fmt.Errorf("failed to add task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}
	return id, nil
}

// UpdateTaskStatus changes the status of an existing task
func (db *DB) UpdateTaskStatus(taskID int, status string) error {
	query := `
		UPDATE tasks
		SET status = ?
		WHERE id = ?
	`
	_, err := db.Exec(query, status, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}
	return nil
}

// ListTasksByStatus retrieves all tasks matching a specific status.
// If status is empty, it returns all tasks.
func (db *DB) ListTasksByStatus(status string) ([]Task, error) {
	var query string
	var args []interface{}

	if status == "" {
		query = `
			SELECT id, title, description, target_files, status
			FROM tasks
			ORDER BY id ASC
		`
	} else {
		query = `
			SELECT id, title, description, target_files, status
			FROM tasks
			WHERE status = ?
			ORDER BY id ASC
		`
		args = append(args, status)
	}

	return queryList(db, query, func(rows *sql.Rows) (Task, error) {
		var t Task
		var targetFiles sql.NullString
		err := rows.Scan(&t.ID, &t.Title, &t.Description, &targetFiles, &t.Status)
		if targetFiles.Valid {
			t.TargetFiles = targetFiles.String
		}
		return t, err
	}, args...)
}
