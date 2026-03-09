package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProjectPrefix generates a unique, readable prefix for tmux sessions based on the project path.
func ProjectPrefix(pwd string) string {
	absPath, err := filepath.Abs(pwd)
	if err != nil {
		absPath = pwd
	}
	base := filepath.Base(absPath)

	// Create a stable 6-character hash of the full path
	hash := sha256.Sum256([]byte(absPath))
	hashStr := hex.EncodeToString(hash[:])[:6]

	return fmt.Sprintf("at-%s-%s-", base, hashStr)
}

// TmuxSession represents an isolated tmux session for an agent builder
type TmuxSession struct {
	SessionName string
	WorktreeDir string
	Command     string            // Optional command to execute inside the session
	EnvVars     map[string]string // Environment variables to set in the session
	ReadOnly    bool              // If true, sets up read-only environment guards
}

// Start spawns a detached tmux session locked to the worktree directory.
// The -d flag is critical to ensure the orchestrator doesn't get blocked
// by the child agent's terminal.
func (t *TmuxSession) Start(ctx context.Context) error {
	// Construct the tmux new-session command
	// -d: detach (run in background)
	// -s: session name
	// -c: start directory (the sandboxed worktree)
	args := []string{"new-session", "-d", "-s", t.SessionName, "-c", t.WorktreeDir}

	if t.Command != "" {
		// If a command is given, we append the execute string.
		// Note: A bare tmux command will close the session immediately after completion.
		// To prevent tmux from instantly closing on abrupt panics or process crashes,
		// we keep the pane open using a fallback loop.
		fallbackCmd := fmt.Sprintf("%s || { echo '\n[Agent Crash/Exit] The agent process failed with a non-zero exit code. Tmux session kept open for debugging...'; sleep 86400; }", t.Command)
		args = append(args, "bash", "-c", fallbackCmd)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start tmux session %q: %v (output: %s)", t.SessionName, err, output)
	}

	// Set environment variables if provided
	if t.EnvVars != nil {
		for key, value := range t.EnvVars {
			envCmd := exec.Command("tmux", "set-environment", "-t", t.SessionName, key, value)
			if err := envCmd.Run(); err != nil {
				// Log but don't fail - not critical
				fmt.Printf("Warning: failed to set env var %s: %v\n", key, err)
			}
		}
	}

	// Set up read-only environment guards if enabled
	if t.ReadOnly {
		t.setupReadOnlyGuards()
	}

	return nil
}

// setupReadOnlyGuards configures environment to prevent file writes
func (t *TmuxSession) setupReadOnlyGuards() {
	// Set environment variables that tools might respect
	readOnlyEnv := map[string]string{
		"AT_READ_ONLY":   "1",
		"READ_ONLY_MODE": "1",
		"EDITOR":         "cat", // Prevent editors from opening
		"GIT_EDITOR":     "cat", // Prevent git from opening editor
	}

	for key, value := range readOnlyEnv {
		cmd := exec.Command("tmux", "set-environment", "-t", t.SessionName, key, value)
		cmd.Run() // Ignore errors, best effort
	}

	// Send a warning message to the session
	warning := "\n╔════════════════════════════════════════════════════════════╗\n" +
		"║  READ-ONLY MODE ENABLED                                    ║\n" +
		"║  This agent is in exploration mode. File writes are        ║\n" +
		"║  discouraged. Use 'dwight mail' to report findings.            ║\n" +
		"╚════════════════════════════════════════════════════════════╝\n"
	t.SendKeys(warning)
}

// SendInput sends keystrokes directly to the tmux session, followed by Enter
// CaptureBuffer reads the last N lines of the tmux pane's output
func (t *TmuxSession) CaptureBuffer(lines int) (string, error) {
	// -p: print to stdout
	// -t: target session
	// -S: start line (negative for history)
	// -E: end line
	args := []string{"capture-pane", "-p", "-t", t.SessionName}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux buffer for session %q: %v (output: %s)", t.SessionName, err, output)
	}

	return string(output), nil
}

func (t *TmuxSession) SendInput(keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.SessionName, keys, "C-m")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send keys to session %q: %v (output: %s)", t.SessionName, err, output)
	}
	return nil
}

// SendKeys sends raw keystrokes to the tmux session without appending Enter
func (t *TmuxSession) SendKeys(keys ...string) error {
	args := append([]string{"send-keys", "-t", t.SessionName}, keys...)
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send keys to session %q: %v (output: %s)", t.SessionName, err, output)
	}
	return nil
}

// SendEscape sends the Escape key N times to interrupt the current process
func (t *TmuxSession) SendEscape(count int) error {
	keys := make([]string, 0, count)
	for i := 0; i < count; i++ {
		keys = append(keys, "Escape")
	}
	return t.SendKeys(keys...)
}

// SendCtrlC sends Ctrl+C to interrupt the current process
func (t *TmuxSession) SendCtrlC() error {
	return t.SendKeys("C-c")
}

// HasSession checks if the tmux session exists
func (t *TmuxSession) HasSession() bool {
	cmd := exec.Command("tmux", "has-session", "-t", t.SessionName)
	err := cmd.Run()
	return err == nil
}

// Kill destroys the tmux session
func (t *TmuxSession) Kill() error {
	cmd := exec.Command("tmux", "kill-session", "-t", t.SessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill tmux session %q: %v (output: %s)", t.SessionName, err, output)
	}

	return nil
}

// GetPID returns the process ID of the tmux session's active pane
func (t *TmuxSession) GetPID() (int, error) {
	// tmux list-panes -t <session> -F "#{pane_pid}"
	cmd := exec.Command("tmux", "list-panes", "-t", t.SessionName, "-F", "#{pane_pid}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get PID for session %q: %v", t.SessionName, err)
	}

	var pid int
	_, err = fmt.Sscanf(string(output), "%d", &pid)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID from output %q: %v", string(output), err)
	}

	return pid, nil
}

// IsProcessAlive checks if the process with given PID is still running
func IsProcessAlive(pid int) bool {
	cmd := exec.Command("kill", "-0", fmt.Sprintf("%d", pid))
	err := cmd.Run()
	return err == nil
}

// Ping checks if tmux is responsive by running a simple command
func (t *TmuxSession) Ping() bool {
	cmd := exec.Command("tmux", "display-message", "-p", "-t", t.SessionName, "pong")
	err := cmd.Run()
	return err == nil
}

// CaptureBufferLines reads a specific number of lines from tmux buffer
// This is useful for Tier 1 AI Triage to analyze last 1000 lines
func (t *TmuxSession) CaptureBufferLines(lineCount int) (string, error) {
	args := []string{"capture-pane", "-p", "-t", t.SessionName, "-S", fmt.Sprintf("-%d", lineCount)}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture %d lines from tmux buffer: %v", lineCount, err)
	}

	return string(output), nil
}

// ClearBuffer clears the tmux pane by sending clear command
func (t *TmuxSession) ClearBuffer() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.SessionName, "C-l")
	return cmd.Run()
}

// ListSessions returns all tmux sessions matching the given prefix
func ListSessions(prefix string) ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.HasPrefix(line, prefix) {
			sessions = append(sessions, strings.TrimPrefix(line, prefix))
		}
	}
	return sessions, nil
}
