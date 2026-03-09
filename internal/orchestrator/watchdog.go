package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"assistant-to/internal/config"
	"assistant-to/internal/constants"
	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

// RecoveryState tracks the recovery status of an agent
type RecoveryState struct {
	mu                 sync.Mutex
	attempts           int
	lastRecoveryTime   time.Time
	recoveryInProgress bool
	escalationSent     bool
}

// Watchdog monitors agent heartbeats and recovers stuck agents.
type Watchdog struct {
	DB           *db.DB
	PWD          string
	Config       *config.Config
	states       map[string]*RecoveryState
	statesMu     sync.RWMutex
	tier1        *Tier1Watchdog
	tier1Enabled bool
}

// NewWatchdog creates a new Watchdog with the given configuration
func NewWatchdog(database *db.DB, pwd string, cfg *config.Config) *Watchdog {
	w := &Watchdog{
		DB:           database,
		PWD:          pwd,
		Config:       cfg,
		states:       make(map[string]*RecoveryState),
		tier1Enabled: cfg.Watchdog.Tier1Enabled,
	}

	// Initialize Tier 1 watchdog only if enabled
	if cfg.Watchdog.Tier1Enabled {
		w.tier1 = NewTier1Watchdog(database, pwd, nil)
	}

	return w
}

// getRecoveryState gets or creates a recovery state for an agent
func (w *Watchdog) getRecoveryState(agentID string) *RecoveryState {
	w.statesMu.Lock()
	defer w.statesMu.Unlock()

	if state, exists := w.states[agentID]; exists {
		return state
	}

	state := &RecoveryState{}
	w.states[agentID] = state
	return state
}

// MonitorHeartbeats runs a continuous loop checking agent activity.
func (w *Watchdog) MonitorHeartbeats(ctx context.Context, agentID string) {
	// Skip if Tier 0 watchdog is disabled
	if !w.Config.Watchdog.Tier0Enabled {
		config.Debug("Watchdog: Tier 0 disabled, skipping monitoring for %s", agentID)
		return
	}

	checkInterval := w.Config.GetWatchdogCheckInterval()
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	config.Info("Watchdog: Starting monitoring for agent %s (check interval: %v)", agentID, checkInterval)

	for {
		select {
		case <-ctx.Done():
			config.Info("Watchdog for agent %s stopping due to context cancellation", agentID)
			return
		case <-ticker.C:
			err := w.checkHeartbeat(ctx, agentID)
			if err != nil {
				config.Error("Heartbeat check failed for %s: %v", agentID, err)
			}
		}
	}
}

func (w *Watchdog) checkHeartbeat(ctx context.Context, agentID string) error {
	stallTimeout := w.Config.GetWatchdogStallTimeout()

	// Use a context with timeout for the heartbeat check
	timeoutCtx, cancel := context.WithTimeout(ctx, stallTimeout+1*time.Minute)
	defer cancel()

	// Get session info
	prefix := sandbox.ProjectPrefix(w.PWD)
	taskID := strings.TrimPrefix(agentID, "builder-")
	sessionName := prefix + taskID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	// TIER 0: Mechanical Checks
	// 1. Check if tmux session exists
	if !session.HasSession() {
		config.Info("Watchdog: Session %s no longer exists for agent %s", sessionName, agentID)
		w.DB.RecordEvent(sessionName, constants.EventTypeSessionDead, "Tmux session no longer exists")
		return nil
	}

	// 2. Check tmux connectivity (ping)
	if !session.Ping() {
		config.Error("🚨 Watchdog: Tmux session %s is not responsive (ping failed)", sessionName)
		w.DB.RecordEvent(sessionName, constants.EventTypeTmuxUnresponsive, "Tmux session ping failed - session may be frozen")
		// Don't return - try to recover
	}

	// 3. Check PID liveness
	pid, err := session.GetPID()
	if err != nil {
		config.Error("Watchdog: Could not get PID for session %s: %v", sessionName, err)
		w.DB.RecordEvent(sessionName, constants.EventTypePIDCheckFailed, fmt.Sprintf("Failed to get PID: %v", err))
	} else {
		if !sandbox.IsProcessAlive(pid) {
			config.Error("🚨 Watchdog: Process %d in session %s is dead", pid, sessionName)
			w.DB.RecordEvent(sessionName, constants.EventTypeProcessDead, fmt.Sprintf("Process %d is no longer alive", pid))
			return nil
		}
	}

	// Query the events table for the last event and its type
	var lastSeen time.Time
	var lastType string
	query := `SELECT timestamp, event_type FROM events WHERE agent_id = ? ORDER BY timestamp DESC LIMIT 1`
	err = w.DB.QueryRowContext(timeoutCtx, query, sessionName).Scan(&lastSeen, &lastType)
	if err != nil {
		if err == sql.ErrNoRows {
			// No events yet, record initial heartbeat
			w.DB.RecordEvent(sessionName, "heartbeat", "Initial heartbeat recorded by watchdog")
			return nil
		}
		return fmt.Errorf("db error: %w", err)
	}

	// If the last event was a question, we don't consider it "stuck" yet
	if lastType == "question" {
		return nil
	}

	// Fallback: Scan tmux buffer for "input required" patterns
	if time.Since(lastSeen) > 1*time.Minute {
		buffer, err := session.CaptureBuffer(20)
		if err == nil {
			lowerBuf := strings.ToLower(buffer)
			if strings.Contains(lowerBuf, "input required") ||
				strings.Contains(lowerBuf, "enter selection") ||
				(strings.Contains(lowerBuf, "?") && time.Since(lastSeen) > 2*time.Minute) {

				config.Info("Watchdog: Detected potential input request in tmux buffer for %s", agentID)
				w.DB.RecordEvent(sessionName, constants.EventTypeQuestion, "Proactive Detection: Agent output suggests it is waiting for input.")
				return nil
			}
		}
	}

	// Check if agent has been idle too long
	idleTime := time.Since(lastSeen)

	// ZOMBIE DETECTION: Agent completely unresponsive for extended period
	zombieThreshold := w.Config.GetZombieThreshold()
	if idleTime > zombieThreshold {
		state := w.getRecoveryState(agentID)
		state.mu.Lock()
		if !state.escalationSent {
			state.mu.Unlock()
			config.Error("🚨 Watchdog: Agent %s is ZOMBIE - no activity for %v (threshold: %v)", agentID, idleTime, zombieThreshold)
			w.DB.RecordEvent(sessionName, constants.EventTypeZombieDetected, fmt.Sprintf("Agent unresponsive for %v", idleTime))
			return w.escalateZombieAgent(timeoutCtx, agentID, session)
		}
		state.mu.Unlock()
		return nil
	}

	// STALL DETECTION: Agent idle but potentially recoverable
	if idleTime > stallTimeout {
		state := w.getRecoveryState(agentID)
		state.mu.Lock()

		// Check if we're already in recovery
		if state.recoveryInProgress {
			// Check if recovery wait time has passed
			recoveryWaitTime := w.Config.GetWatchdogRecoveryWaitTime()
			if time.Since(state.lastRecoveryTime) > recoveryWaitTime && !state.escalationSent {
				// Agent is still stuck after recovery attempt + wait time
				state.mu.Unlock()
				return w.escalateStuckAgent(timeoutCtx, agentID, session)
			}
			state.mu.Unlock()
			return nil
		}

		// Check max recovery attempts
		maxAttempts := w.Config.GetWatchdogMaxRecoveryAttempts()
		if state.attempts >= maxAttempts {
			state.mu.Unlock()
			config.Error("🚨 Watchdog: Agent %s has exceeded max recovery attempts (%d), giving up", agentID, maxAttempts)
			w.DB.RecordEvent(sessionName, constants.EventTypeMaxRecoveryExceeded, fmt.Sprintf("Agent exceeded maximum recovery attempts (%d)", maxAttempts))
			return nil
		}

		state.mu.Unlock()

		// Start recovery
		config.Error("🚨 Watchdog alert: Agent %s has been idle for %v (timeout: %v)", agentID, idleTime, stallTimeout)
		err := w.recoverStuckAgent(timeoutCtx, agentID, session, state)
		if err != nil {
			return fmt.Errorf("failed to recover stuck agent %s: %w", agentID, err)
		}
	}

	return nil
}

// escalateZombieAgent escalates an agent that's completely unresponsive (zombie)
func (w *Watchdog) escalateZombieAgent(ctx context.Context, agentID string, session *sandbox.TmuxSession) error {
	state := w.getRecoveryState(agentID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.escalationSent {
		return nil // Already escalated
	}

	state.escalationSent = true
	state.recoveryInProgress = false

	sessionName := session.SessionName
	config.Error("🚨 Watchdog: ZOMBIE ESCALATION - Agent %s is completely unresponsive", agentID)

	// Record zombie event
	w.DB.RecordEvent(sessionName, constants.EventTypeZombieEscalation, "Agent is zombie - may need session kill and manual intervention")

	// Send urgent email to Coordinator
	coordinatorMsg := fmt.Sprintf(
		"ZOMBIE ALERT: Agent %s has been completely unresponsive for an extended period.\n\n"+
			"The tmux session may need to be killed manually.\n"+
			"Recovery attempts: %d\n"+
			"Consider checking: tmux ls | grep %s",
		agentID, state.attempts, agentID)

	msgQuery := `INSERT INTO mail (sender, recipient, subject, body, type, priority) VALUES (?, 'Coordinator', 'ZOMBIE: Unresponsive Agent', ?, ?, ?)`
	_, err := w.DB.ExecContext(ctx, msgQuery, agentID, coordinatorMsg, db.MailTypeEscalation, db.PriorityCritical)
	if err != nil {
		return fmt.Errorf("failed to send zombie escalation mail: %w", err)
	}

	config.Info("Watchdog: Zombie escalation sent to Coordinator about agent %s", agentID)

	return nil
}

func (w *Watchdog) recoverStuckAgent(ctx context.Context, agentID string, session *sandbox.TmuxSession, state *RecoveryState) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.attempts++
	state.lastRecoveryTime = time.Now()
	state.recoveryInProgress = true

	escapeCount := w.Config.GetWatchdogEscapeKeyCount()
	sessionName := session.SessionName

	log.Printf("Watchdog: Recovering agent %s (attempt %d/%d) - sending %d escape key(s)",
		agentID, state.attempts, w.Config.GetWatchdogMaxRecoveryAttempts(), escapeCount)

	// Record recovery attempt event
	w.DB.RecordEvent(sessionName, constants.EventTypeRecovery,
		fmt.Sprintf("Sending %d escape key(s) to interrupt stuck process (attempt %d)", escapeCount, state.attempts))

	// TIER 1: AI Triage - Perform intelligent analysis on first recovery attempt
	if w.tier1Enabled && state.attempts == 1 {
		log.Printf("Watchdog: Triggering Tier 1 AI Triage for %s", agentID)
		w.DB.RecordEvent(sessionName, constants.EventTypeTriageTriggered, "Tier 1 AI Triage triggered on first recovery attempt")

		// Run triage in background to not block
		go func() {
			triageCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			analysis, err := w.tier1.PerformTriage(triageCtx, agentID)
			if err != nil {
				log.Printf("Watchdog: Tier 1 triage failed for %s: %v", agentID, err)
				w.DB.RecordEvent(sessionName, constants.EventTypeTriageError, fmt.Sprintf("Tier 1 triage error: %v", err))
			} else {
				log.Printf("Watchdog: Tier 1 triage completed for %s - Failure mode: %s (confidence: %.2f)",
					agentID, analysis.FailureMode, analysis.Confidence)
			}
		}()
	}

	// Send escape keys to try to interrupt the current process
	err := session.SendEscape(escapeCount)
	if err != nil {
		log.Printf("Watchdog: Failed to send escape keys to %s: %v", agentID, err)
		// Continue anyway, session might still be recoverable
	}

	// Send a mail message to the agent
	msgQuery := `INSERT INTO mail (sender, recipient, subject, body, type, priority) VALUES ('Coordinator', ?, 'System Alert', 'System alert: No activity detected. Are you stuck? I sent escape keys to interrupt the current process. Please respond with your status.', ?, ?)`
	_, err = w.DB.ExecContext(ctx, msgQuery, agentID, db.MailTypeStatus, db.PriorityHigh)
	if err != nil {
		log.Printf("Watchdog: Failed to send recovery mail to %s: %v", agentID, err)
	}

	// The escalation will happen in checkHeartbeat after RecoveryWaitTime passes
	// if the agent is still not responding

	return nil
}

func (w *Watchdog) escalateStuckAgent(ctx context.Context, agentID string, session *sandbox.TmuxSession) error {
	state := w.getRecoveryState(agentID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.escalationSent {
		return nil // Already escalated
	}

	state.escalationSent = true
	state.recoveryInProgress = false
	sessionName := session.SessionName

	log.Printf("🚨 Watchdog: ESCALATION - Agent %s is non-responsive even after recovery attempt", agentID)

	// Record escalation event
	w.DB.RecordEvent(sessionName, constants.EventTypeEscalation, "Agent non-responsive after recovery attempt - notifying Coordinator")

	// Send email to Coordinator
	coordinatorMsg := fmt.Sprintf(
		"ESCALATION: Agent %s has been non-responsive for an extended period.\n\n"+
			"Recovery attempts: %d\n"+
			"Last recovery attempt: %v\n"+
			"The agent may need manual intervention or the tmux session may need to be killed.",
		agentID, state.attempts, state.lastRecoveryTime)

	msgQuery := `INSERT INTO mail (sender, recipient, subject, body) VALUES (?, 'Coordinator', 'ESCALATION: Non-responsive Agent', ?)`
	_, err := w.DB.ExecContext(ctx, msgQuery, agentID, coordinatorMsg)
	if err != nil {
		return fmt.Errorf("failed to send escalation mail: %w", err)
	}

	log.Printf("Watchdog: Escalation email sent to Coordinator about agent %s", agentID)

	return nil
}

// ResetRecoveryState resets the recovery state for an agent (call this when agent becomes active again)
func (w *Watchdog) ResetRecoveryState(agentID string) {
	w.statesMu.Lock()
	defer w.statesMu.Unlock()

	if state, exists := w.states[agentID]; exists {
		state.mu.Lock()
		state.attempts = 0
		state.recoveryInProgress = false
		state.escalationSent = false
		state.mu.Unlock()
		log.Printf("Watchdog: Reset recovery state for agent %s", agentID)
	}
}
