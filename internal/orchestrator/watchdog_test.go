package orchestrator

import (
	"strings"
	"testing"
	"time"

	"assistant-to/internal/config"
)

func TestEscalationLevels(t *testing.T) {
	tests := []struct {
		name           string
		level          int
		expectedAction string
	}{
		{"warn level", EscalationLevelWarn, "log warning"},
		{"nudge level", EscalationLevelNudge, "send nudge"},
		{"escalate level", EscalationLevelEscalate, "invoke tier 1"},
		{"terminate level", EscalationLevelTerminate, "terminate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.level < 0 || tt.level > MaxEscalationLevel {
				t.Errorf("Invalid escalation level: %d", tt.level)
			}
		})
	}
}

func TestIsPersistentCapability(t *testing.T) {
	// Test persistent capability detection
	// This mirrors the logic in Watchdog.isPersistentCapability
	tests := []struct {
		agentID      string
		isPersistent bool
	}{
		{"coordinator", true},
		{"Coordinator", true},
		{"monitor", true},
		{"Monitor", true},
		{"builder-1", false},
		{"scout-2", false},
		{"reviewer-3", false},
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			lowerID := strings.ToLower(tt.agentID)
			result := PersistentCapabilities[lowerID] ||
				strings.HasPrefix(lowerID, "coordinator") ||
				strings.HasPrefix(lowerID, "monitor")
			if result != tt.isPersistent {
				t.Errorf("isPersistentCapability(%s) = %v, want %v", tt.agentID, result, tt.isPersistent)
			}
		})
	}
}

func TestRecoveryStateEscalation(t *testing.T) {
	state := &RecoveryState{
		escalationLevel: 0,
		stalledSince:    nil,
	}

	// Test initial escalation level
	if state.escalationLevel != 0 {
		t.Errorf("Initial escalation level should be 0, got %d", state.escalationLevel)
	}

	// Test setting stalledSince
	now := time.Now()
	state.stalledSince = &now

	if state.stalledSince == nil {
		t.Error("stalledSince should not be nil after setting")
	}

	// Test advancing escalation level
	state.escalationLevel = EscalationLevelNudge
	if state.escalationLevel != 1 {
		t.Errorf("Escalation level should be 1 after nudge, got %d", state.escalationLevel)
	}
}

func TestRecoveryStateReset(t *testing.T) {
	state := &RecoveryState{
		attempts:        5,
		escalationLevel: 3,
		escalationSent:  true,
	}

	// Simulate reset
	state.attempts = 0
	state.escalationLevel = 0
	state.escalationSent = false

	if state.attempts != 0 {
		t.Errorf("Expected attempts to be 0 after reset, got %d", state.attempts)
	}
	if state.escalationLevel != 0 {
		t.Errorf("Expected escalationLevel to be 0 after reset, got %d", state.escalationLevel)
	}
	if state.escalationSent != false {
		t.Errorf("Expected escalationSent to be false after reset, got %v", state.escalationSent)
	}
}

func TestCalculateEscalationLevel(t *testing.T) {
	cfg := config.Default()

	// Test calculating escalation level based on time stalled
	nudgeInterval := cfg.GetWatchdogRecoveryWaitTime()

	tests := []struct {
		name          string
		stalledMs     time.Duration
		expectedLevel int
	}{
		{"just stalled", 30 * time.Second, 0},
		{"at nudge interval", nudgeInterval, 1},
		{"double nudge interval", nudgeInterval * 2, 2},
		{"triple nudge interval", nudgeInterval * 3, 3},
		{"beyond max", nudgeInterval * 10, MaxEscalationLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := int(tt.stalledMs / nudgeInterval)
			if level > MaxEscalationLevel {
				level = MaxEscalationLevel
			}
			if level != tt.expectedLevel {
				t.Errorf("Expected level %d for stalledMs=%v, got %d", tt.expectedLevel, tt.stalledMs, level)
			}
		})
	}
}

func TestFailureModeTerminal(t *testing.T) {
	// Test that FailureModeTerminal is defined
	if FailureModeTerminal == FailureModeUnknown {
		t.Error("FailureModeTerminal should be different from FailureModeUnknown")
	}

	// Test string representation
	mode := FailureModeTerminal
	if mode.String() == "unknown" {
		t.Error("FailureModeTerminal.String() should not return 'unknown'")
	}
}

func TestZFCPrincipleObservations(t *testing.T) {
	// Test ZFC principle: observable state wins
	// When tmux is dead but recorded state says "working", we should mark as zombie

	// This test documents the ZFC behavior
	// In practice, the actual tmux check happens in checkHeartbeat
	t.Log("ZFC Principle: Observable state (tmux/pid) takes priority over recorded state")
	t.Log("  - tmux dead + sessions says 'working' → zombie immediately")
	t.Log("  - tmux alive + sessions says 'zombie' → investigate (don't auto-kill)")
	t.Log("  - pid dead + tmux alive → zombie (agent exited, shell survived)")
}
