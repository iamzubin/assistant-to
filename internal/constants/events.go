package constants

// Event type constants
const (
	EventTypeToolCall            = "tool_call"
	EventTypeFileWrite           = "file_write"
	EventTypeError               = "error"
	EventTypeMailSent            = "mail_sent"
	EventTypeMailRead            = "mail_read"
	EventTypeSpawn               = "spawn"
	EventTypeKill                = "kill"
	EventTypeRecovery            = "recovery_attempt"
	EventTypeEscalation          = "escalation"
	EventTypeDrift               = "drift_detected"
	EventTypeTriage              = "triage_started"
	EventTypeSessionDead         = "session_dead"
	EventTypeTmuxUnresponsive    = "tmux_unresponsive"
	EventTypePIDCheckFailed      = "pid_check_failed"
	EventTypeProcessDead         = "process_dead"
	EventTypeQuestion            = "question"
	EventTypeZombieDetected      = "zombie_detected"
	EventTypeZombieEscalation    = "zombie_escalation"
	EventTypeMaxRecoveryExceeded = "max_recovery_exceeded"
	EventTypeTriageTriggered     = "triage_triggered"
	EventTypeTriageError         = "triage_error"
	EventTypeMonitorStarted      = "monitor_started"
	EventTypeMonitorStopped      = "monitor_stopped"
)
