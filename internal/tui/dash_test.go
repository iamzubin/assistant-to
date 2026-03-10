package tui

import (
	"strings"
	"testing"
	"time"

	"dwight/internal/db"

	tea "github.com/charmbracelet/bubbletea"
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
			wantLen:     73,
			hasEllipsis: false,
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
			wantLen:     73,
			hasEllipsis: false,
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

func TestTruncateAtWordBoundary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string returns unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length returns unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "truncates at word boundary with space",
			input:    "hello world foo bar",
			maxLen:   11,
			expected: "hello world...",
		},
		{
			name:     "truncates without word boundary",
			input:    "helloworldfoobar",
			maxLen:   10,
			expected: "helloworld...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "single char longer than max",
			input:    "hello",
			maxLen:   1,
			expected: "h...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateAtWordBoundary(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncateAtWordBoundary(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid session name",
			input:    "builder-1",
			expected: "builder-1",
		},
		{
			name:     "with underscores and dots",
			input:    "project_builder.1",
			expected: "project_builder.1",
		},
		{
			name:     "with dangerous characters",
			input:    "builder; rm -rf /",
			expected: "builderrm-rf",
		},
		{
			name:     "with newlines",
			input:    "builder\n1",
			expected: "builder1",
		},
		{
			name:     "only dangerous chars",
			input:    ";:/\\",
			expected: "invalid",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "invalid",
		},
		{
			name:     "alphanumeric only",
			input:    "abc123XYZ",
			expected: "abc123XYZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSessionName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeSessionName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewDashModelWithNilDB(t *testing.T) {
	model := NewDashModel(nil, "/test/path")
	m, ok := model.(*dashModel)
	if !ok {
		t.Fatal("expected *dashModel")
	}
	if m.projectRoot != "/test/path" {
		t.Errorf("projectRoot = %q, want %q", m.projectRoot, "/test/path")
	}
	if m.width != 80 || m.height != 24 {
		t.Errorf("dimensions = (%d, %d), want (80, 24)", m.width, m.height)
	}
}

func TestViewWithZeroDimensions(t *testing.T) {
	m := &dashModel{
		width:  0,
		height: 0,
		ready:  true,
	}
	view := m.View()
	if view == "" {
		t.Error("View() should not return empty string")
	}
	if m.width != 80 || m.height != 24 {
		t.Logf("View() normalized dimensions to (%d, %d)", m.width, m.height)
	}
}

func TestViewWithNegativeDimensions(t *testing.T) {
	m := &dashModel{
		width:  -10,
		height: -5,
		ready:  true,
	}
	view := m.View()
	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestViewNotReady(t *testing.T) {
	m := &dashModel{
		width:  80,
		height: 24,
		ready:  false,
	}
	view := m.View()
	expected := "Initializing Dashboard"
	if !strings.Contains(view, expected) {
		t.Errorf("View() = %q, want to contain %q", view, expected)
	}
}

func TestViewWithQuitConfirm(t *testing.T) {
	m := &dashModel{
		width:           80,
		height:          24,
		ready:           true,
		showQuitConfirm: true,
	}
	view := m.View()
	if !strings.Contains(view, "Quit") {
		t.Error("View() should show quit confirmation")
	}
}

func TestDashModelImplementsTeaModel(t *testing.T) {
	var _ tea.Model = &dashModel{}
}

func TestGetProjectPorts(t *testing.T) {
	apiPort, mcpPort := getProjectPorts("/test/path")
	if apiPort <= 0 || mcpPort <= 0 {
		t.Errorf("Ports should be positive: api=%d, mcp=%d", apiPort, mcpPort)
	}
	if mcpPort != apiPort+1 {
		t.Errorf("mcpPort should be apiPort+1: api=%d, mcp=%d", apiPort, mcpPort)
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"complete", true},
		{"failed", true},
		{"building", true},
		{"merging", true},
		{"review", true},
		{"scouted", true},
		{"started", true},
		{"unknown", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			color := statusColor(tt.status)
			if color == "" {
				t.Error("statusColor should not return empty color")
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
		{0, "●"},
		{6, "●"},
		{-1, "●"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := priorityIndicator(tt.priority)
			if got != tt.expected {
				t.Errorf("priorityIndicator(%d) = %q, want %q", tt.priority, got, tt.expected)
			}
		})
	}
}
