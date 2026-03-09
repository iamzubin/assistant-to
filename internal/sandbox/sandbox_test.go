package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTmuxSession_SendInput(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping tmux test in CI")
	}

	sessionName := "test-tmux-input"
	worktreeDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}

	session := &TmuxSession{
		SessionName: sessionName,
		WorktreeDir: worktreeDir,
	}

	// Start session
	ctx := context.Background()
	if err := session.Start(ctx); err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}
	defer session.Kill()

	// Wait for bash to be ready
	time.Sleep(1 * time.Second)

	// Create a marker file and verify it's gone after we read the input
	outputFile := "test_send_input.txt"
	defer os.Remove(outputFile)

	// command that waits for input and writes it to file
	session.SendInput("read x && echo $x > " + outputFile)
	time.Sleep(500 * time.Millisecond)

	// Send the actual input
	testInput := "success-enter-signal"
	if err := session.SendInput(testInput); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Wait for command to complete
	time.Sleep(2 * time.Second)

	// Check content
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Logf("Capture buffer for debug: \n%s", func() string { b, _ := session.CaptureBuffer(10); return b }())
		t.Fatalf("Failed to read output file: %v", err)
	}

	got := strings.TrimSpace(string(content))
	if got != testInput {
		t.Errorf("Expected %q, got %q", testInput, got)
	}
}
