package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"dwight/internal/config"
	"dwight/internal/constants"
	"dwight/internal/db"
	"dwight/internal/sandbox"
)

// RecoveryState tracks the recovery status of an agent
type RecoveryState struct {
	mu                 sync.Mutex
	attempts           int
	lastRecoveryTime   time.Time
	recoveryInProgress bool
	escalationSent     bool
	escalationLevel    int
	stalledSince       *time.Time
}

// Persistent capabilities that are excluded from stale/zombie detection
var PersistentCapabilities = map[string]bool{
	"coordinator": true,
	"monitor":     true,
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

// Escalation levels for progressive nudging (matching Overstory's Tier 0)
const (
	EscalationLevelWarn      = 0 // Log warning, no direct action
	EscalationLevelNudge     = 1 // Send tmux nudge to agent
	EscalationLevelEscalate  = 2 // Invoke Tier 1 AI triage
	EscalationLevelTerminate = 3 // Kill tmux session
	MaxEscalationLevel       = 3
)

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
	nudgeInterval := w.Config.GetWatchdogRecoveryWaitTime()

	// Use a context with timeout for the heartbeat check
	timeoutCtx, cancel := context.WithTimeout(ctx, stallTimeout+1*time.Minute)
	defer cancel()

	// Get session info
	prefix := sandbox.ProjectPrefix(w.PWD)
	taskID := strings.TrimPrefix(agentID, "builder-")
	sessionName := prefix + taskID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	// Check if agent has persistent capability (coordinator/monitor)
	isPersistent := w.isPersistentCapability(agentID)

	// ZFC PRINCIPLE: Check observable state first (tmux, pid), then recorded state
	// Observable state always wins over recorded state

	// 1. Check if tmux session exists (primary liveness signal for TUI agents)
	tmuxAlive := session.HasSession()
	if !tmuxAlive && !isPersistent {
		config.Info("Watchdog: Session %s no longer exists for agent %s (ZFC: observable state)", sessionName, agentID)
		w.DB.RecordEvent(sessionName, constants.EventTypeSessionDead, "Tmux session no longer exists - ZFC: observable state wins")
		return w.terminateAgent(timeoutCtx, agentID, session, "tmux_dead")
	}

	// 2. Check tmux connectivity (ping)
	if tmuxAlive && !session.Ping() {
		config.Error("🚨 Watchdog: Tmux session %s is not responsive (ping failed)", sessionName)
		w.DB.RecordEvent(sessionName, constants.EventTypeTmuxUnresponsive, "Tmux session ping failed - session may be frozen")
	}

	// 3. Check PID liveness (secondary liveness signal)
	pid, err := session.GetPID()
	pidAlive := false
	if err != nil {
		config.Error("Watchdog: Could not get PID for session %s: %v", sessionName, err)
		w.DB.RecordEvent(sessionName, constants.EventTypePIDCheckFailed, fmt.Sprintf("Failed to get PID: %v", err))
	} else if pid > 0 {
		pidAlive = sandbox.IsProcessAlive(pid)
		if !pidAlive && !isPersistent {
			config.Error("🚨 Watchdog: Process %d in session %s is dead (ZFC: pid dead but tmux alive)", pid, sessionName)
			w.DB.RecordEvent(sessionName, constants.EventTypeProcessDead, fmt.Sprintf("Process %d is no longer alive - agent process exited, shell survived", pid))
			return w.terminateAgent(timeoutCtx, agentID, session, "pid_dead")
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

	// PERSISTENT CAPABILITY EXEMPTION:
	// Coordinator and monitor are expected to have long idle periods waiting for mail/events
	// Only tmux/pid liveness matters for them - skip stale/zombie detection
	if isPersistent {
		config.Debug("Watchdog: %s is persistent capability, skipping time-based stale detection", agentID)
		return nil
	}

	// ZOMBIE DETECTION: Agent completely unresponsive for extended period
	zombieThreshold := w.Config.GetZombieThreshold()
	if idleTime > zombieThreshold {
		state := w.getRecoveryState(agentID)
		state.mu.Lock()
		if !state.escalationSent {
			state.mu.Unlock()
			config.Error("🚨 Watchdog: Agent %s is ZOMBIE - no activity for %v (threshold: %v)", agentID, idleTime, zombieThreshold)
			w.DB.RecordEvent(sessionName, constants.EventTypeZombieDetected, fmt.Sprintf("Agent unresponsive for %v", idleTime))
			return w.terminateAgent(timeoutCtx, agentID, session, "zombie_timeout")
		}
		state.mu.Unlock()
		return nil
	}

	// STALL DETECTION: Agent idle but potentially recoverable
	// Use progressive escalation levels
	if idleTime > stallTimeout {
		state := w.getRecoveryState(agentID)
		state.mu.Lock()

		// Initialize stalledSince on first stall detection
		if state.stalledSince == nil {
			now := time.Now()
			state.stalledSince = &now
			state.escalationLevel = EscalationLevelWarn
		}

		// Calculate expected escalation level based on time stalled
		stalledMs := time.Since(*state.stalledSince).Milliseconds()
		nudgeIntervalMs := nudgeInterval.Milliseconds()
		expectedLevel := int(stalledMs / nudgeIntervalMs)
		if expectedLevel > MaxEscalationLevel {
			expectedLevel = MaxEscalationLevel
		}

		// Advance escalation level if enough time has passed
		if expectedLevel > state.escalationLevel {
			state.escalationLevel = expectedLevel
		}

		// Check max recovery attempts
		maxAttempts := w.Config.GetWatchdogMaxRecoveryAttempts()
		if state.attempts >= maxAttempts && state.escalationLevel >= MaxEscalationLevel {
			state.mu.Unlock()
			config.Error("🚨 Watchdog: Agent %s has exceeded max recovery attempts (%d), terminating", agentID, maxAttempts)
			w.DB.RecordEvent(sessionName, constants.EventTypeMaxRecoveryExceeded, fmt.Sprintf("Agent exceeded maximum recovery attempts (%d)", maxAttempts))
			return w.terminateAgent(timeoutCtx, agentID, session, "max_attempts_exceeded")
		}

		state.mu.Unlock()

		// Execute escalation action based on level
		return w.executeEscalationAction(ctx, agentID, session, state, idleTime)
	}

	// Agent recovered - reset escalation tracking
	state := w.getRecoveryState(agentID)
	state.mu.Lock()
	if state.stalledSince != nil {
		state.stalledSince = nil
		state.escalationLevel = 0
		state.attempts = 0
		state.escalationSent = false
		config.Info("Watchdog: Agent %s recovered - reset escalation tracking", agentID)
	}
	state.mu.Unlock()

	return nil
}

// isPersistentCapability checks if an agent has a persistent capability
func (w *Watchdog) isPersistentCapability(agentID string) bool {
	// Check if agent is coordinator or monitor based on agent ID pattern
	// Coordinator: "coordinator", Monitor: "monitor"
	lowerID := strings.ToLower(agentID)
	return PersistentCapabilities[lowerID] ||
		strings.HasPrefix(lowerID, "coordinator") ||
		strings.HasPrefix(lowerID, "monitor")
}

// executeEscalationAction performs the appropriate action based on escalation level
func (w *Watchdog) executeEscalationAction(ctx context.Context, agentID string, session *sandbox.TmuxSession, state *RecoveryState, idleTime time.Duration) error {
	sessionName := session.SessionName

	switch state.escalationLevel {
	case EscalationLevelWarn:
		// Level 0: Log warning only
		config.Warn("🚨 Watchdog alert: Agent %s has been idle for %v (level 0: warn)", agentID, idleTime)
		w.DB.RecordEvent(sessionName, constants.EventTypeRecovery, fmt.Sprintf("Agent stalled - level 0 warning (idle: %v)", idleTime))
		return nil

	case EscalationLevelNudge:
		// Level 1: Send tmux nudge to agent
		config.Warn("🚨 Watchdog: Agent %s stalled for %v - sending nudge (level 1)", agentID, idleTime)
		state.mu.Lock()
		state.attempts++
		state.lastRecoveryTime = time.Now()
		state.mu.Unlock()

		w.DB.RecordEvent(sessionName, constants.EventTypeRecovery, fmt.Sprintf("Sending nudge to stalled agent (attempt %d)", state.attempts))

		// Send interrupt sequence (esc+esc+enter) to try to interrupt
		err := session.SendInterruptSequence()
		if err != nil {
			config.Error("Watchdog: Failed to send interrupt sequence to %s: %v", agentID, err)
		}

		// Send mail to agent
		msgQuery := `INSERT INTO mail (sender, recipient, subject, body, type, priority) VALUES ('Coordinator', ?, 'System Alert', 'System alert: No activity detected for a while. Are you stuck? Please respond with your status.', ?, ?)`
		_, err = w.DB.ExecContext(ctx, msgQuery, agentID, db.MailTypeStatus, db.PriorityHigh)
		if err != nil {
			config.Error("Watchdog: Failed to send recovery mail to %s: %v", agentID, err)
		}

		return nil

	case EscalationLevelEscalate:
		// Level 2: Invoke Tier 1 AI triage
		config.Warn("🚨 Watchdog: Agent %s still stalled - invoking Tier 1 AI triage (level 2)", agentID)
		state.mu.Lock()
		state.attempts++
		state.lastRecoveryTime = time.Now()
		state.mu.Unlock()

		w.DB.RecordEvent(sessionName, constants.EventTypeRecovery, fmt.Sprintf("Invoking Tier 1 AI triage (attempt %d)", state.attempts))

		// Trigger Tier 1 triage in background
		if w.tier1Enabled {
			go func() {
				triageCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()

				analysis, err := w.tier1.PerformTriage(triageCtx, agentID)
				if err != nil {
					config.Error("Watchdog: Tier 1 triage failed for %s: %v", agentID, err)
					w.DB.RecordEvent(sessionName, constants.EventTypeTriageError, fmt.Sprintf("Tier 1 triage error: %v", err))
				} else {
					config.Info("Watchdog: Tier 1 triage completed for %s - Failure mode: %s (confidence: %.2f)",
						agentID, analysis.FailureMode, analysis.Confidence)

					// If triage recommends termination, escalate to level 3
					if analysis.FailureMode == FailureModeTerminal {
						config.Error("Watchdog: Tier 1 classified %s as terminal failure - escalating to terminate", agentID)
						w.terminateAgent(ctx, agentID, session, "triage_terminal")
					}
				}
			}()
		}

		return nil

	case EscalationLevelTerminate:
		// Level 3: Kill the session
		config.Error("🚨 Watchdog: Agent %s reached max escalation - terminating", agentID)
		w.DB.RecordEvent(sessionName, constants.EventTypeMaxRecoveryExceeded, "Agent reached max escalation level - terminating")
		return w.terminateAgent(ctx, agentID, session, "max_escalation")

	default:
		return nil
	}
}

// terminateAgent kills an agent session and records the failure
func (w *Watchdog) terminateAgent(ctx context.Context, agentID string, session *sandbox.TmuxSession, reason string) error {
	state := w.getRecoveryState(agentID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.escalationSent {
		return nil // Already terminated
	}

	state.escalationSent = true
	sessionName := session.SessionName

	config.Error("🚨 Watchdog: TERMINATING agent %s (reason: %s)", agentID, reason)
	w.DB.RecordEvent(sessionName, constants.EventTypeZombieEscalation, fmt.Sprintf("Agent terminated: %s", reason))

	// Record failure to expertise/knowledge system
	w.recordFailure(agentID, reason)

	// Send urgent email to Coordinator
	coordinatorMsg := fmt.Sprintf(
		"AGENT TERMINATED: Agent %s has been terminated by the watchdog.\n\n"+
			"Reason: %s\n"+
			"Recovery attempts: %d\n"+
			"Last activity: %v\n"+
			"Consider checking: tmux ls | grep %s",
		agentID, reason, state.attempts, state.lastRecoveryTime, agentID)

	msgQuery := `INSERT INTO mail (sender, recipient, subject, body, type, priority) VALUES (?, 'Coordinator', 'AGENT TERMINATED: %s', ?, ?, ?)`
	_, err := w.DB.ExecContext(ctx, msgQuery, agentID, coordinatorMsg, db.MailTypeEscalation, db.PriorityCritical)
	if err != nil {
		return fmt.Errorf("failed to send termination mail: %w", err)
	}

	// Note: Actual session killing would be done by the caller or coordinator
	// The watchdog primarily monitors and escalates

	config.Info("Watchdog: Termination alert sent to Coordinator about agent %s", agentID)

	return nil
}

// recordFailure records an agent failure to the expertise/knowledge system
func (w *Watchdog) recordFailure(agentID string, reason string) {
	// This would integrate with the expertise/knowledge system
	// For now, we'll record it as an event that can be picked up by mulch
	config.Info("Watchdog: Recording failure for agent %s (reason: %s)", agentID, reason)

	// Record to expertise system if available
	description := fmt.Sprintf("Agent %s failed: %s", agentID, reason)
	w.DB.RecordEvent(agentID, "agent_failure", description)
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
		state.escalationLevel = 0
		state.stalledSince = nil
		state.mu.Unlock()
		log.Printf("Watchdog: Reset recovery state for agent %s", agentID)
	}
}
