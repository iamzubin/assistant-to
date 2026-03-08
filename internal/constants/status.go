package constants

// Task status constants
const (
	TaskStatusPending  = "pending"
	TaskStatusStarted  = "started"
	TaskStatusScouted  = "scouted"
	TaskStatusBuilding = "building"
	TaskStatusReview   = "review"
	TaskStatusMerging  = "merging"
	TaskStatusComplete = "complete"
	TaskStatusFailed   = "failed"
)

// ValidTaskStatuses contains all valid task status values
var ValidTaskStatuses = []string{
	TaskStatusPending,
	TaskStatusStarted,
	TaskStatusScouted,
	TaskStatusBuilding,
	TaskStatusReview,
	TaskStatusMerging,
	TaskStatusComplete,
	TaskStatusFailed,
}

// Task priority constants
const (
	TaskPriorityCritical = 1
	TaskPriorityHigh     = 2
	TaskPriorityNormal   = 3
	TaskPriorityLow      = 4
	TaskPriorityTrivial  = 5
)
