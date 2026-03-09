package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

// Tier1Watchdog performs AI triage on stalled agents
// It analyzes the last 1000 lines of transcript to identify failure modes
type Tier1Watchdog struct {
	DB     *db.DB
	PWD    string
	Config *WatchdogConfig
}

// WatchdogConfig holds configuration for tier 1 watchdog
type WatchdogConfig struct {
	TriageTimeout time.Duration
}

// NewTier1Watchdog creates a new Tier 1 watchdog instance
func NewTier1Watchdog(database *db.DB, pwd string, cfg *WatchdogConfig) *Tier1Watchdog {
	if cfg == nil {
		cfg = &WatchdogConfig{
			TriageTimeout: 2 * time.Minute,
		}
	}
	return &Tier1Watchdog{
		DB:     database,
		PWD:    pwd,
		Config: cfg,
	}
}

// TriageAnalysis represents the result of analyzing a stalled agent's transcript
type TriageAnalysis struct {
	AgentID        string
	Transcript     string
	FailureMode    FailureMode
	Confidence     float64
	Recommendation string
}

// FailureMode represents detected failure types
type FailureMode int

const (
	FailureModeUnknown FailureMode = iota
	FailureModeInfiniteLoop
	FailureModeInputRequired
	FailureModeBuildError
	FailureModeTestFailure
	FailureModeResourceExhaustion
	FailureModeNetworkTimeout
)

func (f FailureMode) String() string {
	switch f {
	case FailureModeInfiniteLoop:
		return "infinite_loop"
	case FailureModeInputRequired:
		return "input_required"
	case FailureModeBuildError:
		return "build_error"
	case FailureModeTestFailure:
		return "test_failure"
	case FailureModeResourceExhaustion:
		return "resource_exhaustion"
	case FailureModeNetworkTimeout:
		return "network_timeout"
	default:
		return "unknown"
	}
}

// PerformTriage performs AI triage on a stalled agent
// This should be called when Tier 0 detects a stall
func (t *Tier1Watchdog) PerformTriage(ctx context.Context, agentID string) (*TriageAnalysis, error) {
	log.Printf("Tier1: Starting AI triage for agent %s", agentID)
	t.DB.RecordEvent(agentID, "triage_started", "Tier 1 AI Triage initiated")

	// Get session info
	prefix := sandbox.ProjectPrefix(t.PWD)
	taskID := strings.TrimPrefix(agentID, "builder-")
	sessionName := prefix + taskID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	// 1. Capture last 1000 lines of transcript
	transcript, err := t.captureTranscript(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to capture transcript: %w", err)
	}

	// 2. Analyze transcript for failure patterns
	analysis := t.analyzeTranscript(agentID, transcript)

	// 3. Record analysis result
	t.DB.RecordEvent(agentID, "triage_completed",
		fmt.Sprintf("Failure mode: %s (confidence: %.2f)", analysis.FailureMode, analysis.Confidence))

	// 4. Send appropriate stimulus based on failure mode
	err = t.sendStimulus(ctx, agentID, analysis)
	if err != nil {
		log.Printf("Tier1: Failed to send stimulus for %s: %v", agentID, err)
	}

	return analysis, nil
}

// captureTranscript captures the last 1000 lines from the tmux session
func (t *Tier1Watchdog) captureTranscript(ctx context.Context, session *sandbox.TmuxSession) (string, error) {
	log.Printf("Tier1: Capturing last 1000 lines of transcript from session %s", session.SessionName)

	transcript, err := session.CaptureBufferLines(1000)
	if err != nil {
		return "", fmt.Errorf("failed to capture buffer: %w", err)
	}

	// Limit to last 100KB to avoid huge payloads
	if len(transcript) > 100*1024 {
		transcript = transcript[len(transcript)-100*1024:]
	}

	log.Printf("Tier1: Captured %d bytes of transcript", len(transcript))
	return transcript, nil
}

// analyzeTranscript performs pattern matching to identify failure modes
func (t *Tier1Watchdog) analyzeTranscript(agentID string, transcript string) *TriageAnalysis {
	analysis := &TriageAnalysis{
		AgentID:    agentID,
		Transcript: transcript,
	}

	// Pattern matching for different failure modes
	lowerTranscript := strings.ToLower(transcript)
	lastLines := transcript
	if len(transcript) > 5000 {
		lastLines = transcript[len(transcript)-5000:]
	}
	lowerLastLines := strings.ToLower(lastLines)

	// Check for infinite loops
	if t.detectInfiniteLoop(lowerTranscript) {
		analysis.FailureMode = FailureModeInfiniteLoop
		analysis.Confidence = 0.85
		analysis.Recommendation = "Detected potential infinite loop. Consider adding iteration limits or timeout checks."
		return analysis
	}

	// Check for input required
	if t.detectInputRequired(lowerLastLines) {
		analysis.FailureMode = FailureModeInputRequired
		analysis.Confidence = 0.90
		analysis.Recommendation = "Agent appears to be waiting for user input. Consider providing defaults or auto-confirm flags."
		return analysis
	}

	// Check for build errors
	if t.detectBuildError(lowerLastLines) {
		analysis.FailureMode = FailureModeBuildError
		analysis.Confidence = 0.80
		analysis.Recommendation = "Build error detected. Review compilation errors and fix syntax or dependency issues."
		return analysis
	}

	// Check for test failures
	if t.detectTestFailure(lowerLastLines) {
		analysis.FailureMode = FailureModeTestFailure
		analysis.Confidence = 0.80
		analysis.Recommendation = "Test failures detected. Review test output and fix failing assertions."
		return analysis
	}

	// Check for resource exhaustion
	if t.detectResourceExhaustion(lowerLastLines) {
		analysis.FailureMode = FailureModeResourceExhaustion
		analysis.Confidence = 0.75
		analysis.Recommendation = "Resource exhaustion detected (memory/disk). Consider cleanup or resource limits."
		return analysis
	}

	// Check for network timeouts
	if t.detectNetworkTimeout(lowerLastLines) {
		analysis.FailureMode = FailureModeNetworkTimeout
		analysis.Confidence = 0.75
		analysis.Recommendation = "Network timeout detected. Check connectivity or increase timeout values."
		return analysis
	}

	// Unknown failure mode
	analysis.FailureMode = FailureModeUnknown
	analysis.Confidence = 0.50
	analysis.Recommendation = "Unable to determine specific failure mode. Manual intervention may be required."

	return analysis
}

// Pattern detection helpers
func (t *Tier1Watchdog) detectInfiniteLoop(transcript string) bool {
	// Look for repeated patterns or excessive iterations
	patterns := []string{
		"for i := 0; i <",
		"infinite loop",
		"loop detected",
		"iteration",
		"retrying",
	}

	// Count occurrences of loop-related terms
	loopCount := 0
	for _, pattern := range patterns {
		loopCount += strings.Count(transcript, pattern)
	}

	// If we see loop patterns many times, likely an infinite loop
	if loopCount > 20 {
		return true
	}

	// Check for repeating error messages (indicating a retry loop)
	lines := strings.Split(transcript, "\n")
	if len(lines) > 100 {
		// Check last 50 lines for repetition
		recentLines := lines[len(lines)-50:]
		errorCounts := make(map[string]int)
		for _, line := range recentLines {
			if strings.Contains(line, "error") || strings.Contains(line, "Error") {
				errorCounts[line]++
				if errorCounts[line] > 5 {
					return true
				}
			}
		}
	}

	return false
}

func (t *Tier1Watchdog) detectInputRequired(transcript string) bool {
	patterns := []string{
		"input required",
		"enter selection",
		"[y/n]",
		"(y/n)",
		"continue?",
		"proceed?",
		"confirm",
		"password:",
		"username:",
		"are you sure?",
		"overwrite?",
		"delete?",
		"force?",
		"commit message:",
	}

	for _, pattern := range patterns {
		if strings.Contains(transcript, pattern) {
			return true
		}
	}

	// Check for question marks at the end of lines (prompts)
	if strings.Contains(transcript, "?\n") {
		return true
	}

	return false
}

func (t *Tier1Watchdog) detectBuildError(transcript string) bool {
	patterns := []string{
		"build failed",
		"compilation error",
		"syntax error",
		"undefined:",
		"cannot find package",
		"no such file or directory",
		"exit status 1",
		"make: ***",
	}

	for _, pattern := range patterns {
		if strings.Contains(transcript, pattern) {
			return true
		}
	}

	return false
}

func (t *Tier1Watchdog) detectTestFailure(transcript string) bool {
	patterns := []string{
		"test failed",
		"fail:",
		"--- fail",
		"assertion failed",
		"expected",
		"got:",
		"mismatch",
	}

	for _, pattern := range patterns {
		if strings.Contains(transcript, pattern) {
			return true
		}
	}

	return false
}

func (t *Tier1Watchdog) detectResourceExhaustion(transcript string) bool {
	patterns := []string{
		"out of memory",
		"oom",
		"cannot allocate",
		"no space left",
		"disk full",
		"too many open files",
		"resource temporarily unavailable",
	}

	for _, pattern := range patterns {
		if strings.Contains(transcript, pattern) {
			return true
		}
	}

	return false
}

func (t *Tier1Watchdog) detectNetworkTimeout(transcript string) bool {
	patterns := []string{
		"timeout",
		"connection refused",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"deadline exceeded",
		"context deadline",
	}

	for _, pattern := range patterns {
		if strings.Contains(transcript, pattern) {
			return true
		}
	}

	return false
}

// sendStimulus sends appropriate stimulus based on the failure mode
func (t *Tier1Watchdog) sendStimulus(ctx context.Context, agentID string, analysis *TriageAnalysis) error {
	log.Printf("Tier1: Sending stimulus for failure mode %s to agent %s", analysis.FailureMode, agentID)

	// Get session info
	prefix := sandbox.ProjectPrefix(t.PWD)
	taskID := strings.TrimPrefix(agentID, "builder-")
	sessionName := prefix + taskID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	switch analysis.FailureMode {
	case FailureModeInputRequired:
		// Send 'y' and Enter (C-m) to auto-confirm
		log.Printf("Tier1: Sending auto-confirmation keystrokes (y + C-m) to %s", agentID)
		err := session.SendKeys("y", "C-m")
		if err != nil {
			return fmt.Errorf("failed to send keystrokes: %w", err)
		}
		t.DB.SendMail("Coordinator", agentID, "Triage: Input Detected",
			"Tier 1 Watchdog detected you may be waiting for input. Sent 'y' keystroke. If you need specific input, please request it explicitly via mail.",
			db.MailTypeStatus, db.PriorityHigh)

	case FailureModeInfiniteLoop:
		// Send interrupt signal
		log.Printf("Tier1: Sending Ctrl+C to interrupt potential infinite loop in %s", agentID)
		err := session.SendCtrlC()
		if err != nil {
			return fmt.Errorf("failed to send Ctrl+C: %w", err)
		}
		t.DB.SendMail("Coordinator", agentID, "Triage: Loop Detected",
			"Tier 1 Watchdog detected a potential infinite loop. Sent interrupt signal. Please review your iteration logic and add proper termination conditions.",
			db.MailTypeEscalation, db.PriorityCritical)

	case FailureModeBuildError, FailureModeTestFailure:
		// Send guidance via mail
		t.DB.SendMail("Coordinator", agentID, "Triage: Build/Test Failure",
			fmt.Sprintf("Tier 1 Watchdog detected %s. Recommendation: %s", analysis.FailureMode, analysis.Recommendation),
			db.MailTypeStatus, db.PriorityHigh)

	case FailureModeResourceExhaustion, FailureModeNetworkTimeout:
		// Send warning and suggestion
		t.DB.SendMail("Coordinator", agentID, "Triage: Resource/Network Issue",
			fmt.Sprintf("Tier 1 Watchdog detected %s. Recommendation: %s", analysis.FailureMode, analysis.Recommendation),
			db.MailTypeEscalation, db.PriorityHigh)

	default:
		// Unknown failure - send generic probe
		log.Printf("Tier1: Unknown failure mode, sending probe to %s", agentID)
		t.DB.SendMail("Coordinator", agentID, "Triage: Status Check",
			"Tier 1 Watchdog detected stall but could not determine specific failure mode. Please send a status update or request assistance if stuck.",
			db.MailTypeQuestion, db.PriorityNormal)
	}

	return nil
}

// SpawnScoutAgent spawns a transient Scout agent to analyze complex failure modes
// This is used when pattern matching is insufficient
func (t *Tier1Watchdog) SpawnScoutAgent(ctx context.Context, agentID string, transcript string) error {
	log.Printf("Tier1: Spawning Scout agent to analyze %s", agentID)

	// Create a temporary worktree for the Scout
	scoutID := fmt.Sprintf("scout-triage-%d", time.Now().Unix())
	worktreeDir := filepath.Join(t.PWD, ".assistant-to", "worktrees", scoutID)

	// For now, we'll just log this - full implementation would spawn an actual agent
	log.Printf("Tier1: Would spawn Scout agent %s in %s to analyze transcript (%d bytes)",
		scoutID, worktreeDir, len(transcript))

	// Write transcript to file for analysis
	transcriptPath := filepath.Join(t.PWD, ".assistant-to", fmt.Sprintf("triage-%s.txt", agentID))
	err := os.WriteFile(transcriptPath, []byte(transcript), 0644)
	if err != nil {
		return fmt.Errorf("failed to write transcript: %w", err)
	}

	log.Printf("Tier1: Transcript saved to %s", transcriptPath)

	// TODO: Actually spawn a Scout agent here with a prompt to analyze the transcript
	// This would require integration with the orchestrator to spawn agents

	return nil
}
