package tui

import (
	"testing"
	"time"

	"assistant-to/internal/db"
)

func TestFeedItemDescriptionTruncation(t *testing.T) {
	longMessage := "This is a very long log message that should definitely be truncated because it exceeds the maximum length allowed in the feed display area of the dashboard UI and would otherwise break the layout by taking multiple lines."

	shortMessage := "Short message"

	tests := []struct {
		name        string
		feedItem    feedItem
		wantLen     int
		hasEllipsis bool
	}{
		{
			name: "long message gets truncated",
			feedItem: feedItem{
				AgentID:   "builder-1",
				EventType: "log",
				Details:   longMessage,
				Timestamp: time.Now(),
			},
			wantLen:     maxFeedDescriptionLength,
			hasEllipsis: true,
		},
		{
			name: "short message stays as is",
			feedItem: feedItem{
				AgentID:   "builder-1",
				EventType: "log",
				Details:   shortMessage,
				Timestamp: time.Now(),
			},
			wantLen:     len(shortMessage),
			hasEllipsis: false,
		},
		{
			name: "empty message",
			feedItem: feedItem{
				AgentID:   "builder-1",
				EventType: "log",
				Details:   "",
				Timestamp: time.Now(),
			},
			wantLen:     0,
			hasEllipsis: false,
		},
		{
			name: "exact max length",
			feedItem: feedItem{
				AgentID:   "builder-1",
				EventType: "log",
				Details:   string(make([]byte, maxFeedDescriptionLength)),
				Timestamp: time.Now(),
			},
			wantLen:     maxFeedDescriptionLength,
			hasEllipsis: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.feedItem.Description()
			if len(got) != tt.wantLen {
				t.Errorf("Description() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.hasEllipsis && got[len(got)-3:] != "..." {
				t.Errorf("Description() expected ellipsis, got: %s", got[len(got)-3:])
			}
		})
	}
}

func TestTaskItemTitle(t *testing.T) {
	tests := []struct {
		name    string
		task    db.Task
		wantSub string
		notWant string
	}{
		{
			name: "normal task shows priority indicator",
			task: db.Task{
				ID:     1,
				Title:  "Test Task",
				Status: "pending",
			},
			wantSub: "[1]",
			notWant: "↳",
		},
		{
			name: "subtask shows parent info",
			task: db.Task{
				ID:       2,
				ParentID: 1,
				Title:    "Child Task",
				Status:   "pending",
			},
			wantSub: "↳(P:1)",
		},
		{
			name: "critical priority shows fire emoji",
			task: db.Task{
				ID:       3,
				Priority: 1,
				Title:    "Critical Task",
				Status:   "pending",
			},
			wantSub: "🔥",
		},
		{
			name: "high priority shows bolt emoji",
			task: db.Task{
				ID:       4,
				Priority: 2,
				Title:    "High Task",
				Status:   "pending",
			},
			wantSub: "⚡",
		},
		{
			name: "normal priority shows bullet",
			task: db.Task{
				ID:       5,
				Priority: 3,
				Title:    "Normal Task",
				Status:   "pending",
			},
			wantSub: "●",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := taskItem{tt.task}
			title := item.Title()
			if tt.wantSub != "" && !contains(title, tt.wantSub) {
				t.Errorf("Title() should contain %q, got %q", tt.wantSub, title)
			}
			if tt.notWant != "" && contains(title, tt.notWant) {
				t.Errorf("Title() should not contain %q, got %q", tt.notWant, title)
			}
		})
	}
}

func TestTaskItemDescription(t *testing.T) {
	tests := []struct {
		name    string
		task    db.Task
		wantSub string
		notWant string
	}{
		{
			name: "shows status",
			task: db.Task{
				ID:     1,
				Title:  "Test Task",
				Status: "building",
			},
			wantSub: "Status:",
		},
		{
			name: "shows description when present",
			task: db.Task{
				ID:          1,
				Title:       "Test Task",
				Description: "This is a test description",
				Status:      "pending",
			},
			wantSub: "Desc:",
		},
		{
			name: "shows target files when present",
			task: db.Task{
				ID:          1,
				Title:       "Test Task",
				TargetFiles: "src/main.go, src/utils.go",
				Status:      "pending",
			},
			wantSub: "Files:",
		},
		{
			name: "shows created timestamp",
			task: db.Task{
				ID:        1,
				Title:     "Test Task",
				CreatedAt: time.Now().Add(-2 * time.Hour),
				Status:    "pending",
			},
			wantSub: "Created:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := taskItem{tt.task}
			desc := item.Description()
			if tt.wantSub != "" && !contains(desc, tt.wantSub) {
				t.Errorf("Description() should contain %q, got %q", tt.wantSub, desc)
			}
			if tt.notWant != "" && contains(desc, tt.notWant) {
				t.Errorf("Description() should not contain %q, got %q", tt.notWant, desc)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"less than minute", 30 * time.Second, "<1m"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 2 * 24 * time.Hour, "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.d)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, result, tt.expected)
			}
		})
	}
}

func TestPriorityIndicator(t *testing.T) {
	tests := []struct {
		priority int
		expected string
	}{
		{1, "🔥"},
		{2, "⚡"},
		{3, "●"},
		{4, "○"},
		{5, "◌"},
		{99, "●"}, // default
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := priorityIndicator(tt.priority)
			if result != tt.expected {
				t.Errorf("priorityIndicator(%d) = %q, want %q", tt.priority, result, tt.expected)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
