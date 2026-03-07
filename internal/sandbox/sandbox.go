package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
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
	Command     string // Optional command to execute inside the session
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

	return nil
}

// SendInput sends keystrokes directly to the tmux session, followed by Enter
func (t *TmuxSession) SendInput(keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.SessionName, keys, "C-m")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send keys to session %q: %v (output: %s)", t.SessionName, err, output)
	}
	return nil
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
