package merge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dwight/internal/db"
	"dwight/internal/sandbox"
)

// AIAssistedResolution handles Tier 4: spawning a Merger agent to resolve conflicts
type AIAssistedResolution struct {
	DB          *db.DB
	PWD         string
	WorktreeDir string
	BaseBranch  string
	TaskID      string
}

// NewAIAssistedResolution creates a new AI-assisted resolution handler
func NewAIAssistedResolution(database *db.DB, pwd, worktreeDir, baseBranch, taskID string) *AIAssistedResolution {
	return &AIAssistedResolution{
		DB:          database,
		PWD:         pwd,
		WorktreeDir: worktreeDir,
		BaseBranch:  baseBranch,
		TaskID:      taskID,
	}
}

// AttemptResolution spawns a Merger agent to resolve semantic conflicts
func (a *AIAssistedResolution) AttemptResolution(ctx context.Context) (*ResolutionResult, error) {
	// Collect full diff of conflicting files
	diff, err := a.collectFullDiff()
	if err != nil {
		return nil, fmt.Errorf("failed to collect diff: %w", err)
	}

	if len(diff) == 0 {
		return &ResolutionResult{
			Tier:     Tier4AIAssisted,
			Success:  false,
			Message:  "No conflicts to resolve",
			Strategy: "no_conflicts",
		}, nil
	}

	// Create resolution worktree
	resolutionWorktree, err := a.createResolutionWorktree()
	if err != nil {
		return nil, fmt.Errorf("failed to create resolution worktree: %w", err)
	}

	// Write diff context to file for the Merger agent
	diffPath := filepath.Join(resolutionWorktree, ".merge_context.md")
	if err := os.WriteFile(diffPath, []byte(diff), 0644); err != nil {
		return nil, fmt.Errorf("failed to write diff context: %w", err)
	}

	// Spawn Merger agent
	if err := a.spawnMergerAgent(ctx, resolutionWorktree, diffPath); err != nil {
		return &ResolutionResult{
			Tier:     Tier4AIAssisted,
			Success:  false,
			Message:  fmt.Sprintf("Failed to spawn Merger agent: %v", err),
			Strategy: "merger_spawn_failed",
		}, nil
	}

	return &ResolutionResult{
		Tier:     Tier4AIAssisted,
		Success:  true,
		Message:  "Merger agent spawned to resolve conflicts",
		Strategy: "merger_agent_spawned",
	}, nil
}

// collectFullDiff collects the complete diff of all conflicting files
func (a *AIAssistedResolution) collectFullDiff() (string, error) {
	var diff strings.Builder

	// Get list of conflicted files
	files, err := a.getConflictFiles()
	if err != nil {
		return "", err
	}

	diff.WriteString("# Merge Conflict Resolution Context\n\n")
	diff.WriteString(fmt.Sprintf("Task: %s\n", a.TaskID))
	diff.WriteString(fmt.Sprintf("Base Branch: %s\n", a.BaseBranch))
	diff.WriteString(fmt.Sprintf("Worktree: %s\n\n", a.WorktreeDir))

	// For each conflicted file, get detailed diff
	for _, file := range files {
		diff.WriteString(fmt.Sprintf("## File: %s\n\n", file))

		// Get the diff for this file
		fileDiff, err := a.getFileDiff(file)
		if err != nil {
			diff.WriteString(fmt.Sprintf("(Error getting diff: %v)\n\n", err))
			continue
		}

		diff.WriteString("```diff\n")
		diff.WriteString(fileDiff)
		diff.WriteString("\n```\n\n")

		// Get conflict markers content
		conflictContent, err := a.getConflictContent(file)
		if err != nil {
			diff.WriteString(fmt.Sprintf("(Error reading conflict: %v)\n\n", err))
			continue
		}

		diff.WriteString("### Conflict Details:\n```\n")
		diff.WriteString(conflictContent)
		diff.WriteString("\n```\n\n")
	}

	return diff.String(), nil
}

// getConflictFiles returns the list of files with conflicts
func (a *AIAssistedResolution) getConflictFiles() ([]string, error) {
	cmd := exec.Command("git", "-C", a.WorktreeDir, "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get conflict files: %v", err)
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	return files, nil
}

// getFileDiff gets the diff for a specific file
func (a *AIAssistedResolution) getFileDiff(file string) (string, error) {
	cmd := exec.Command("git", "-C", a.WorktreeDir, "diff", "HEAD", fmt.Sprintf("%s...HEAD", a.BaseBranch), "--", file)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %v", err)
	}
	return string(output), nil
}

// getConflictContent reads the file with conflict markers
func (a *AIAssistedResolution) getConflictContent(file string) (string, error) {
	content, err := os.ReadFile(filepath.Join(a.WorktreeDir, file))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// createResolutionWorktree creates an isolated worktree for the Merger agent
func (a *AIAssistedResolution) createResolutionWorktree() (string, error) {
	resolutionID := fmt.Sprintf("merger-%s-%d", a.TaskID, time.Now().Unix())
	worktreeDir := filepath.Join(a.PWD, ".dwight", "worktrees", resolutionID)

	_, err := sandbox.CreateWorktree(a.PWD, resolutionID, a.BaseBranch, "")
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %w", err)
	}

	return worktreeDir, nil
}

// spawnMergerAgent spawns a Merger agent in the resolution worktree
func (a *AIAssistedResolution) spawnMergerAgent(ctx context.Context, worktreeDir, diffPath string) error {
	// Build the merge prompt
	prompt := a.buildMergerPrompt(diffPath)

	// Write prompt to mission file
	missionPath := filepath.Join(worktreeDir, ".mission.md")
	if err := os.WriteFile(missionPath, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("failed to write mission file: %w", err)
	}

	// Create tmux session
	sessionName := sandbox.ProjectPrefix(a.PWD) + "merger-" + a.TaskID
	session := sandbox.TmuxSession{
		SessionName: sessionName,
		WorktreeDir: worktreeDir,
		Command:     fmt.Sprintf("opencode --model google/gemini-2.5-pro --prompt \"$(cat .mission.md)\""),
	}

	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start merger session: %w", err)
	}

	// Record event
	a.DB.RecordEvent("merger-"+a.TaskID, "spawn", fmt.Sprintf("Merger agent spawned for task %s", a.TaskID))

	return nil
}

// buildMergerPrompt creates the prompt for the Merger agent
func (a *AIAssistedResolution) buildMergerPrompt(diffPath string) string {
	return fmt.Sprintf(`# Merger Agent - Conflict Resolution

You are a **Merger Agent** specialized in resolving complex git merge conflicts.

## Your Task

Resolve semantic conflicts in the worktree. The full diff context has been written to:
%s

## Conflict Resolution Guidelines

1. **Understand Both Sides**: Carefully read both versions of the conflicting code
2. **Preserve Intent**: Maintain the original intent of both changes when possible
3. **Semantic Correctness**: Ensure the merged code is semantically correct
4. **Test Compatibility**: Make sure the resolution doesn't break existing functionality

## Steps

1. Read the diff context from .merge_context.md
2. Examine each conflicted file
3. Resolve conflicts by editing the files directly
4. Stage resolved files with git add
5. Complete the merge with git commit
6. Send a completion mail to the Coordinator

## Constraints

- You are in an isolated worktree - feel free to make changes
- Focus ONLY on resolving the conflicts, not refactoring
- Do not introduce new features
- Ensure the code compiles/tests pass after resolution

When complete, mark the task as resolved and notify the Coordinator.
`, diffPath)
}

// WaitForResolution waits for the Merger agent to complete (optional polling mechanism)
func (a *AIAssistedResolution) WaitForResolution(ctx context.Context, timeout time.Duration) (*ResolutionResult, error) {
	// Poll for completion
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return &ResolutionResult{
				Tier:     Tier4AIAssisted,
				Success:  false,
				Message:  "Timeout waiting for Merger agent",
				Strategy: "merger_timeout",
			}, nil
		case <-ticker.C:
			// Check if merge is complete by looking for completion indicator
			// This would check for specific file or mail message
			if a.isResolutionComplete() {
				return &ResolutionResult{
					Tier:     Tier4AIAssisted,
					Success:  true,
					Message:  "Merger agent completed resolution",
					Strategy: "merger_completed",
				}, nil
			}
		}
	}
}

// isResolutionComplete checks if the Merger agent has finished
func (a *AIAssistedResolution) isResolutionComplete() bool {
	// Check if there are still conflicts
	files, err := a.getConflictFiles()
	if err != nil {
		return false
	}
	return len(files) == 0
}
