package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"assistant-to/internal/db"
)

// Watchdog monitors agent heartbeats and recovers stuck agents.
type Watchdog struct {
	DB *db.DB
}

// MonitorHeartbeats runs a continuous loop checking agent activity.
// It heavily utilizes Go's context.WithTimeout for the 5-minute heartbeat monitor.
func (w *Watchdog) MonitorHeartbeats(ctx context.Context, agentID string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Watchdog for agent %s stopping due to context cancellation", agentID)
			return
		case <-ticker.C:
			// Check the heartbeat
			err := w.checkHeartbeat(ctx, agentID)
			if err != nil {
				log.Printf("Heartbeat check failed for %s: %v", agentID, err)
			}
		}
	}
}

func (w *Watchdog) checkHeartbeat(ctx context.Context, agentID string) error {
	// Use a 5-minute timeout for the heartbeat monitor block
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Query the events table for the last heartbeat
	var lastSeen time.Time
	query := `SELECT MAX(timestamp) FROM events WHERE agent_id = ?`
	err := w.DB.QueryRowContext(timeoutCtx, query, agentID).Scan(&lastSeen)
	if err != nil {
		// If no events found, it might be a new agent
		return fmt.Errorf("no events found or db error: %w", err)
	}

	// Verify if 5 minutes have passed
	if time.Since(lastSeen) > 5*time.Minute {
		// The agent is considered stuck
		log.Printf("🚨 Watchdog alert: Agent %s has been idle for > 5 minutes (last seen: %v)", agentID, lastSeen)

		err := w.recoverStuckAgent(timeoutCtx, agentID)
		if err != nil {
			return fmt.Errorf("failed to recover stuck agent %s: %w", agentID, err)
		}
	}

	return nil
}

func (w *Watchdog) recoverStuckAgent(ctx context.Context, agentID string) error {
	// 1. Send an at mail message
	msgQuery := `INSERT INTO mail (sender, recipient, subject, body) VALUES ('Lead', ?, 'System Alert', 'System alert: No activity detected for 5 minutes. Are you stuck? Please output your current blocker.')`
	_, err := w.DB.ExecContext(ctx, msgQuery, agentID)
	if err != nil {
		return fmt.Errorf("failed to send recovery mail: %w", err)
	}

	// Logic to kill the tmux session and respawn would be implemented here,
	// potentially interacting with the internal/sandbox package.

	return nil
}
