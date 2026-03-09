package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Expertise type constants
const (
	ExpertiseTypeConvention = "convention"
	ExpertiseTypePattern    = "pattern"
	ExpertiseTypeFailure    = "failure"
	ExpertiseTypeDecision   = "decision"
)

// Expertise represents a learned piece of project knowledge
type Expertise struct {
	ID          int       `json:"id"`
	Domain      string    `json:"domain"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// RecordExpertise adds a new learning to the expertise table
func (d *DB) RecordExpertise(domain, expertiseType, description string) (int64, error) {
	query := `
		INSERT INTO expertise (domain, type, description)
		VALUES (?, ?, ?)
	`
	res, err := d.Exec(query, domain, expertiseType, description)
	if err != nil {
		return 0, fmt.Errorf("failed to record expertise: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}
	return id, nil
}

// GetExpertiseByDomain retrieves all expertise entries for a specific domain
func (d *DB) GetExpertiseByDomain(domain string) ([]Expertise, error) {
	query := `
		SELECT id, domain, type, description, timestamp
		FROM expertise
		WHERE domain = ?
		ORDER BY timestamp DESC
	`
	return queryList(d, query, func(rows *sql.Rows) (Expertise, error) {
		var e Expertise
		err := rows.Scan(&e.ID, &e.Domain, &e.Type, &e.Description, &e.Timestamp)
		return e, err
	}, domain)
}

// GetExpertiseByType retrieves all expertise entries of a specific type
func (d *DB) GetExpertiseByType(expertiseType string) ([]Expertise, error) {
	query := `
		SELECT id, domain, type, description, timestamp
		FROM expertise
		WHERE type = ?
		ORDER BY timestamp DESC
	`
	return queryList(d, query, func(rows *sql.Rows) (Expertise, error) {
		var e Expertise
		err := rows.Scan(&e.ID, &e.Domain, &e.Type, &e.Description, &e.Timestamp)
		return e, err
	}, expertiseType)
}

// GetAllExpertise retrieves all expertise entries
func (d *DB) GetAllExpertise() ([]Expertise, error) {
	query := `
		SELECT id, domain, type, description, timestamp
		FROM expertise
		ORDER BY domain ASC, timestamp DESC
	`
	return queryList(d, query, func(rows *sql.Rows) (Expertise, error) {
		var e Expertise
		err := rows.Scan(&e.ID, &e.Domain, &e.Type, &e.Description, &e.Timestamp)
		return e, err
	})
}

// GetRecentExpertise retrieves expertise entries from the last N days
func (d *DB) GetRecentExpertise(days int) ([]Expertise, error) {
	query := `
		SELECT id, domain, type, description, timestamp
		FROM expertise
		WHERE timestamp >= datetime('now', ?)
		ORDER BY timestamp DESC
	`
	return queryList(d, query, func(rows *sql.Rows) (Expertise, error) {
		var e Expertise
		err := rows.Scan(&e.ID, &e.Domain, &e.Type, &e.Description, &e.Timestamp)
		return e, err
	}, fmt.Sprintf("-%d days", days))
}

// SearchExpertise searches for expertise entries matching a keyword
func (d *DB) SearchExpertise(keyword string) ([]Expertise, error) {
	query := `
		SELECT id, domain, type, description, timestamp
		FROM expertise
		WHERE description LIKE ? OR domain LIKE ?
		ORDER BY timestamp DESC
	`
	pattern := "%" + keyword + "%"
	return queryList(d, query, func(rows *sql.Rows) (Expertise, error) {
		var e Expertise
		err := rows.Scan(&e.ID, &e.Domain, &e.Type, &e.Description, &e.Timestamp)
		return e, err
	}, pattern, pattern)
}

// DeleteExpertise removes an expertise entry by ID
func (d *DB) DeleteExpertise(id int) error {
	query := `DELETE FROM expertise WHERE id = ?`
	res, err := d.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete expertise: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("expertise not found: %d", id)
	}
	return nil
}

// ValidateExpertiseType checks if the given type is valid
func ValidateExpertiseType(t string) bool {
	switch t {
	case ExpertiseTypeConvention, ExpertiseTypePattern, ExpertiseTypeFailure, ExpertiseTypeDecision:
		return true
	default:
		return false
	}
}

// GetExpertiseTypes returns all valid expertise types
func GetExpertiseTypes() []string {
	return []string{
		ExpertiseTypeConvention,
		ExpertiseTypePattern,
		ExpertiseTypeFailure,
		ExpertiseTypeDecision,
	}
}
