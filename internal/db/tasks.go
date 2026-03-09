package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Task status constants
const (
	TaskStatusPending  = "pending"
	TaskStatusStarted  = "started"
	TaskStatusScouted  = "scouted"
	TaskStatusBuilding = "building"
	TaskStatusReview   = "review"
	TaskStatusMerging  = "merging"
	TaskStatusComplete = "complete"
	TaskStatusFailed   = "failed"
)

// ValidTaskStatuses contains all valid task status values
var ValidTaskStatuses = []string{
	TaskStatusPending,
	TaskStatusStarted,
	TaskStatusScouted,
	TaskStatusBuilding,
	TaskStatusReview,
	TaskStatusMerging,
	TaskStatusComplete,
	TaskStatusFailed,
}

// Task priority constants
const (
	TaskPriorityCritical = 1
	TaskPriorityHigh     = 2
	TaskPriorityNormal   = 3
	TaskPriorityLow      = 4
	TaskPriorityTrivial  = 5
)

// Task represents an autonomous work item
type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	TargetFiles string    `json:"target_files"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IsValidTaskStatus checks if a status value is valid
func IsValidTaskStatus(status string) bool {
	for _, s := range ValidTaskStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// AddTask creates a new task in the database with status 'pending' and default priority
func (d *DB) AddTask(title, description, targetFiles string) (int64, error) {
	return d.AddTaskWithPriority(title, description, targetFiles, TaskPriorityNormal)
}

// AddTaskWithPriority creates a new task with a specific priority
func (d *DB) AddTaskWithPriority(title, description, targetFiles string, priority int) (int64, error) {
	query := `
		INSERT INTO tasks (title, description, target_files, status, priority)
		VALUES (?, ?, ?, ?, ?)
	`
	res, err := d.Exec(query, title, description, targetFiles, TaskStatusPending, priority)
	if err != nil {
		return 0, fmt.Errorf("failed to add task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}
	return id, nil
}

// UpdateTaskStatus changes the status of an existing task with validation
func (d *DB) UpdateTaskStatus(taskID int, status string) error {
	if !IsValidTaskStatus(status) {
		return fmt.Errorf("invalid task status: %s (valid: %v)", status, ValidTaskStatuses)
	}

	query := `
		UPDATE tasks
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	res, err := d.Exec(query, status, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %d", taskID)
	}
	return nil
}

// UpdateTaskPriority changes the priority of an existing task
func (d *DB) UpdateTaskPriority(taskID int, priority int) error {
	query := `
		UPDATE tasks
		SET priority = ?
		WHERE id = ?
	`
	res, err := d.Exec(query, priority, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task priority: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %d", taskID)
	}
	return nil
}

// ListTasksByStatus retrieves all tasks matching a specific status.
// If status is empty, it returns all tasks.
func (d *DB) ListTasksByStatus(status string) ([]Task, error) {
	var query string
	var args []interface{}

	if status == "" {
		query = `
			SELECT id, title, description, target_files, status, priority, created_at, updated_at
			FROM tasks
			ORDER BY priority ASC, id ASC
		`
	} else {
		query = `
			SELECT id, title, description, target_files, status, priority, created_at, updated_at
			FROM tasks
			WHERE status = ?
			ORDER BY priority ASC, id ASC
		`
		args = append(args, status)
	}

	return queryList(d, query, func(rows *sql.Rows) (Task, error) {
		var t Task
		var targetFiles sql.NullString
		err := rows.Scan(&t.ID, &t.Title, &t.Description, &targetFiles, &t.Status, &t.Priority, &t.CreatedAt, &t.UpdatedAt)
		if targetFiles.Valid {
			t.TargetFiles = targetFiles.String
		}
		return t, err
	}, args...)
}

// ListTasksByPriority retrieves all tasks matching a specific priority level
func (d *DB) ListTasksByPriority(priority int) ([]Task, error) {
	query := `
		SELECT id, title, description, target_files, status, priority, created_at, updated_at
		FROM tasks
		WHERE priority = ?
		ORDER BY id ASC
	`
	return queryList(d, query, func(rows *sql.Rows) (Task, error) {
		var t Task
		var targetFiles sql.NullString
		err := rows.Scan(&t.ID, &t.Title, &t.Description, &targetFiles, &t.Status, &t.Priority, &t.CreatedAt, &t.UpdatedAt)
		if targetFiles.Valid {
			t.TargetFiles = targetFiles.String
		}
		return t, err
	}, priority)
}

// RemoveTask deletes a task from the database
func (d *DB) RemoveTask(taskID int) error {
	query := `DELETE FROM tasks WHERE id = ?`
	res, err := d.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to remove task: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %d", taskID)
	}
	return nil
}

// GetTaskByID retrieves a single task by its ID.
func (d *DB) GetTaskByID(taskID int) (*Task, error) {
	query := `
		SELECT id, title, description, target_files, status, priority, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`
	var t Task
	var targetFiles sql.NullString
	err := d.QueryRow(query, taskID).Scan(&t.ID, &t.Title, &t.Description, &targetFiles, &t.Status, &t.Priority, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %d", taskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if targetFiles.Valid {
		t.TargetFiles = targetFiles.String
	}
	return &t, nil
}
