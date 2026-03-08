package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

// Watchdog monitors agent heartbeats and recovers stuck agents.
type Watchdog struct {
	DB  *db.DB
	PWD string
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

	// Query the events table for the last event and its type
	var lastSeen time.Time
	var lastType string
	query := `SELECT timestamp, event_type FROM events WHERE agent_id = ? ORDER BY timestamp DESC LIMIT 1`
	err := w.DB.QueryRowContext(timeoutCtx, query, agentID).Scan(&lastSeen, &lastType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // New agent, no events yet
		}
		return fmt.Errorf("db error: %w", err)
	}

	// If the last event was a question, we don't consider it "stuck" yet
	if lastType == "question" {
		return nil
	}

	// Fallback: Scan tmux buffer for "input required" patterns
	// This helps if the agent is blocked on a tool that doesn't explicitly log.
	if time.Since(lastSeen) > 1*time.Minute {
		prefix := sandbox.ProjectPrefix(w.PWD)
		taskID := strings.TrimPrefix(agentID, "builder-")
		sessionName := prefix + taskID

		session := &sandbox.TmuxSession{SessionName: sessionName}
		buffer, err := session.CaptureBuffer(20)
		if err == nil {
			// Look for common "input required" markers or question marks at the end of output
			lowerBuf := strings.ToLower(buffer)
			if strings.Contains(lowerBuf, "input required") ||
				strings.Contains(lowerBuf, "enter selection") ||
				(strings.Contains(lowerBuf, "?") && time.Since(lastSeen) > 2*time.Minute) {

				log.Printf("Watchdog: Detected potential input request in tmux buffer for %s", agentID)
				// Log a synthetic 'question' event to trigger dashboard highlights
				w.DB.RecordEvent(agentID, "question", "Proactive Detection: Agent output suggests it is waiting for input.")
				return nil
			}
		}
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
	msgQuery := `INSERT INTO mail (sender, recipient, subject, body) VALUES ('Coordinator', ?, 'System Alert', 'System alert: No activity detected for 5 minutes. Are you stuck? Please output your current blocker.')`
	_, err := w.DB.ExecContext(ctx, msgQuery, agentID)
	if err != nil {
		return fmt.Errorf("failed to send recovery mail: %w", err)
	}

	// Logic to kill the tmux session and respawn would be implemented here,
	// potentially interacting with the internal/sandbox package.

	return nil
}
