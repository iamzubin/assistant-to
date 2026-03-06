package sandbox

import (
	"fmt"
	"os/exec"
)

// TmuxSession represents an isolated tmux session for an agent builder
type TmuxSession struct {
	SessionName string
	WorktreeDir string
}

// Start spawns a detached tmux session locked to the worktree directory.
// The -d flag is critical to ensure the orchestrator doesn't get blocked
// by the child agent's terminal.
func (t *TmuxSession) Start() error {
	// Construct the tmux new-session command
	// -d: detach (run in background)
	// -s: session name
	// -c: start directory (the sandboxed worktree)
	cmd := exec.Command("tmux", "new-session", "-d", "-s", t.SessionName, "-c", t.WorktreeDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start tmux session %q: %v (output: %s)", t.SessionName, err, output)
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
