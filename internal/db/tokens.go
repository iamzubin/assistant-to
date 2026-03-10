package db

import (
	"database/sql"
	"fmt"
	"time"
)

type TokenMetrics struct {
	ID               int       `json:"id"`
	AgentID          string    `json:"agent_id"`
	TaskID           *int64    `json:"task_id,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	Model            string    `json:"model"`
	SessionCount     int       `json:"session_count"`
	LastUpdated      time.Time `json:"last_updated"`
}

func (d *DB) RecordTokenMetrics(agentID string, taskID *int64, promptTokens, completionTokens int, costUSD float64, model string) error {
	totalTokens := promptTokens + completionTokens

	query := `
		INSERT INTO token_metrics (agent_id, task_id, prompt_tokens, completion_tokens, total_tokens, cost_usd, model, session_count, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(agent_id) DO UPDATE SET
			prompt_tokens = prompt_tokens + excluded.prompt_tokens,
			completion_tokens = completion_tokens + excluded.completion_tokens,
			total_tokens = total_tokens + excluded.total_tokens,
			cost_usd = cost_usd + excluded.cost_usd,
			model = COALESCE(excluded.model, model),
			session_count = session_count + 1,
			last_updated = CURRENT_TIMESTAMP
	`
	_, err := d.Exec(query, agentID, taskID, promptTokens, completionTokens, totalTokens, costUSD, model)
	if err != nil {
		return fmt.Errorf("failed to record token metrics: %w", err)
	}
	return nil
}

func (d *DB) GetTokenMetricsByAgent(agentID string) (*TokenMetrics, error) {
	query := `
		SELECT id, agent_id, task_id, prompt_tokens, completion_tokens, total_tokens, cost_usd, model, session_count, last_updated
		FROM token_metrics
		WHERE agent_id = ?
		ORDER BY last_updated DESC
		LIMIT 1
	`
	var tm TokenMetrics
	var taskID sql.NullInt64
	err := d.QueryRow(query, agentID).Scan(
		&tm.ID, &tm.AgentID, &taskID, &tm.PromptTokens, &tm.CompletionTokens,
		&tm.TotalTokens, &tm.CostUSD, &tm.Model, &tm.SessionCount, &tm.LastUpdated,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get token metrics: %w", err)
	}
	if taskID.Valid {
		tm.TaskID = &taskID.Int64
	}
	return &tm, nil
}

func (d *DB) GetAllTokenMetrics() ([]TokenMetrics, error) {
	query := `
		SELECT id, agent_id, task_id, prompt_tokens, completion_tokens, total_tokens, cost_usd, model, session_count, last_updated
		FROM token_metrics
		ORDER BY last_updated DESC
	`
	rows, err := d.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all token metrics: %w", err)
	}
	defer rows.Close()

	var metrics []TokenMetrics
	for rows.Next() {
		var tm TokenMetrics
		var taskID sql.NullInt64
		err := rows.Scan(
			&tm.ID, &tm.AgentID, &taskID, &tm.PromptTokens, &tm.CompletionTokens,
			&tm.TotalTokens, &tm.CostUSD, &tm.Model, &tm.SessionCount, &tm.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan token metrics: %w", err)
		}
		if taskID.Valid {
			tm.TaskID = &taskID.Int64
		}
		metrics = append(metrics, tm)
	}
	return metrics, nil
}

func (d *DB) GetTokenMetricsSummary() (map[string]interface{}, error) {
	query := `
		SELECT 
			COUNT(DISTINCT agent_id) as agent_count,
			SUM(total_tokens) as total_tokens,
			SUM(cost_usd) as total_cost,
			SUM(prompt_tokens) as total_prompt,
			SUM(completion_tokens) as total_completion
		FROM token_metrics
	`
	var agentCount int
	var totalTokens, totalPrompt, totalCompletion sql.NullInt64
	var totalCost sql.NullFloat64

	err := d.QueryRow(query).Scan(&agentCount, &totalTokens, &totalCost, &totalPrompt, &totalCompletion)
	if err != nil {
		return nil, fmt.Errorf("failed to get token summary: %w", err)
	}

	summary := map[string]interface{}{
		"agent_count":             agentCount,
		"total_tokens":            0,
		"total_cost_usd":          0.0,
		"total_prompt_tokens":     0,
		"total_completion_tokens": 0,
	}
	if totalTokens.Valid {
		summary["total_tokens"] = totalTokens.Int64
	}
	if totalCost.Valid {
		summary["total_cost_usd"] = totalCost.Float64
	}
	if totalPrompt.Valid {
		summary["total_prompt_tokens"] = totalPrompt.Int64
	}
	if totalCompletion.Valid {
		summary["total_completion_tokens"] = totalCompletion.Int64
	}

	return summary, nil
}

func (d *DB) GetTopTokenConsumers(limit int) ([]TokenMetrics, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
		SELECT id, agent_id, task_id, prompt_tokens, completion_tokens, total_tokens, cost_usd, model, session_count, last_updated
		FROM token_metrics
		ORDER BY total_tokens DESC
		LIMIT ?
	`
	rows, err := d.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top token consumers: %w", err)
	}
	defer rows.Close()

	var metrics []TokenMetrics
	for rows.Next() {
		var tm TokenMetrics
		var taskID sql.NullInt64
		err := rows.Scan(
			&tm.ID, &tm.AgentID, &taskID, &tm.PromptTokens, &tm.CompletionTokens,
			&tm.TotalTokens, &tm.CostUSD, &tm.Model, &tm.SessionCount, &tm.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan token metrics: %w", err)
		}
		if taskID.Valid {
			tm.TaskID = &taskID.Int64
		}
		metrics = append(metrics, tm)
	}
	return metrics, nil
}
