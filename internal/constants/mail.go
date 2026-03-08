package constants

// Mail type constants
const (
	MailTypeDispatch   = "dispatch"
	MailTypeWorkerDone = "worker_done"
	MailTypeMergeReady = "merge_ready"
	MailTypeEscalation = "escalation"
	MailTypeStatus     = "status"
	MailTypeQuestion   = "question"
	MailTypeResult     = "result"
	MailTypeError      = "error"
)

// Mail priority constants
const (
	PriorityCritical = 1
	PriorityHigh     = 2
	PriorityNormal   = 3
	PriorityLow      = 4
	PriorityTrivial  = 5
)

// Expertise type constants
const (
	ExpertiseTypeConvention = "convention"
	ExpertiseTypePattern    = "pattern"
	ExpertiseTypeFailure    = "failure"
	ExpertiseTypeDecision   = "decision"
)
