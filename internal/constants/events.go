package constants

// Event type constants
const (
	EventTypeToolCall    = "tool_call"
	EventTypeFileWrite   = "file_write"
	EventTypeError       = "error"
	EventTypeMailSent    = "mail_sent"
	EventTypeMailRead    = "mail_read"
	EventTypeSpawn       = "spawn"
	EventTypeKill        = "kill"
	EventTypeRecovery    = "recovery_attempt"
	EventTypeEscalation  = "escalation"
	EventTypeDrift       = "drift_detected"
	EventTypeTriage      = "triage_started"
	EventTypeSessionDead = "session_dead"
)
