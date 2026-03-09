package tui

import (
	"testing"
	"time"
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
