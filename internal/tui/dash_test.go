package tui

import (
	"strings"
	"testing"
	"time"

	"dwight/internal/db"
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

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "zero",
			input:    0,
			expected: "0",
		},
		{
			name:     "hundreds",
			input:    500,
			expected: "500",
		},
		{
			name:     "thousands",
			input:    1500,
			expected: "1.5K",
		},
		{
			name:     "ten thousands",
			input:    15000,
			expected: "15.0K",
		},
		{
			name:     "hundred thousands",
			input:    250000,
			expected: "250.0K",
		},
		{
			name:     "millions",
			input:    1500000,
			expected: "1.5M",
		},
		{
			name:     "ten millions",
			input:    15000000,
			expected: "15.0M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTokens(tt.input)
			if got != tt.expected {
				t.Errorf("formatTokens(%d) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTokenSummary(t *testing.T) {
	summary := TokenSummary{
		TotalTokens:           1500000,
		TotalCostUSD:          12.50,
		TotalPromptTokens:     1000000,
		TotalCompletionTokens: 500000,
		AgentCount:            3,
		TopConsumers:          nil,
	}

	if summary.TotalTokens != 1500000 {
		t.Errorf("Expected TotalTokens 1500000, got %d", summary.TotalTokens)
	}
	if summary.TotalCostUSD != 12.50 {
		t.Errorf("Expected TotalCostUSD 12.50, got %f", summary.TotalCostUSD)
	}
	if summary.AgentCount != 3 {
		t.Errorf("Expected AgentCount 3, got %d", summary.AgentCount)
	}
}

func TestTaskItemDescriptionTruncation(t *testing.T) {
	longDescription := "This is a very long task description that exceeds the character limit and should be truncated properly to avoid breaking the TUI layout when displayed in the task list."
	longTargetFiles := "path/to/very/long/file/path/that/exceeds/the/limit/and/needs/to/be/truncated/for/proper/display.go"

	tests := []struct {
		name        string
		task        db.Task
		maxLen      int
		wantLen     int
		hasEllipsis bool
	}{
		{
			name: "short description stays as is",
			task: db.Task{
				Title:       "Test Task",
				Description: "Short desc",
				TargetFiles: "",
				Status:      "pending",
				CreatedAt:   time.Now().Add(-1 * time.Hour),
			},
			maxLen:      maxTaskDescriptionLength,
			wantLen:     56,
			hasEllipsis: false,
		},
		{
			name: "long description gets truncated",
			task: db.Task{
				Title:       "Test Task",
				Description: longDescription,
				TargetFiles: "",
				Status:      "pending",
				CreatedAt:   time.Now().Add(-1 * time.Hour),
			},
			maxLen:      maxTaskDescriptionLength,
			wantLen:     maxTaskDescriptionLength,
			hasEllipsis: true,
		},
		{
			name: "both description and target files truncated",
			task: db.Task{
				Title:       "Test Task",
				Description: longDescription,
				TargetFiles: longTargetFiles,
				Status:      "pending",
				CreatedAt:   time.Now().Add(-1 * time.Hour),
			},
			maxLen:      maxTaskDescriptionLength,
			wantLen:     maxTaskDescriptionLength,
			hasEllipsis: true,
		},
		{
			name: "empty description and files",
			task: db.Task{
				Title:       "Test Task",
				Description: "",
				TargetFiles: "",
				Status:      "pending",
				CreatedAt:   time.Now().Add(-1 * time.Hour),
			},
			maxLen:      maxTaskDescriptionLength,
			wantLen:     35,
			hasEllipsis: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := taskItem{tt.task}
			got := item.Description()
			if len(got) > tt.maxLen {
				t.Errorf("Description() len = %d, want <= %d", len(got), tt.maxLen)
			}
			if len(got) != tt.wantLen {
				t.Errorf("Description() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.hasEllipsis && !strings.HasSuffix(got, "...") {
				t.Errorf("Description() expected ellipsis suffix, got: %s", got[len(got)-5:])
			}
		})
	}
}
