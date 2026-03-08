package merge

import (
	"fmt"
	"os/exec"
	"strings"
)

// RebaseAttempt performs a contextual rebase to resolve conflicts
type RebaseAttempt struct {
	WorktreeDir string
	BaseBranch  string
}

// NewRebaseAttempt creates a new rebase attempt
func NewRebaseAttempt(worktreeDir, baseBranch string) *RebaseAttempt {
	return &RebaseAttempt{
		WorktreeDir: worktreeDir,
		BaseBranch:  baseBranch,
	}
}

// AttemptRebase tries to rebase the current branch onto the base branch
func (r *RebaseAttempt) AttemptRebase() (*ResolutionResult, error) {
	// Check for stale commits that might be causing conflicts
	staleCommits, err := r.detectStaleCommits()
	if err != nil {
		return nil, fmt.Errorf("failed to detect stale commits: %w", err)
	}

	if len(staleCommits) > 0 {
		// Try to drop or reorder stale commits
		if err := r.handleStaleCommits(staleCommits); err != nil {
			return &ResolutionResult{
				Tier:     Tier3Rebase,
				Success:  false,
				Message:  fmt.Sprintf("Failed to handle stale commits: %v", err),
				Strategy: "rebase_with_stale_handling",
			}, nil
		}
	}

	// Attempt the rebase
	if err := r.performRebase(); err != nil {
		// Check if it's a conflict we can auto-resolve
		if r.canAutoResolve() {
			if resolveErr := r.autoResolveConflicts(); resolveErr != nil {
				return &ResolutionResult{
					Tier:     Tier3Rebase,
					Success:  false,
					Message:  fmt.Sprintf("Rebase conflicts could not be auto-resolved: %v", resolveErr),
					Strategy: "rebase_auto_resolve_failed",
				}, nil
			}
			return &ResolutionResult{
				Tier:     Tier3Rebase,
				Success:  true,
				Message:  "Rebase completed with auto-resolved conflicts",
				Strategy: "rebase_auto_resolved",
			}, nil
		}

		// Rebase has unresolvable conflicts
		conflicts, _ := r.getConflictFiles()
		return &ResolutionResult{
			Tier:      Tier3Rebase,
			Success:   false,
			Message:   fmt.Sprintf("Rebase failed with conflicts: %v", err),
			Conflicts: conflicts,
			Strategy:  "rebase_failed",
		}, nil
	}

	return &ResolutionResult{
		Tier:     Tier3Rebase,
		Success:  true,
		Message:  "Rebase completed successfully",
		Strategy: "rebase_clean",
	}, nil
}

// detectStaleCommits identifies commits that may be causing stale conflicts
func (r *RebaseAttempt) detectStaleCommits() ([]string, error) {
	// Get commits that are in our branch but not in base
	cmd := exec.Command("git", "-C", r.WorktreeDir, "log",
		fmt.Sprintf("%s..HEAD", r.BaseBranch),
		"--oneline")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %v", err)
	}

	commits := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Filter for potentially stale commits (e.g., merge commits, fixups)
	var stale []string
	for _, commit := range commits {
		if strings.Contains(commit, "merge") ||
			strings.Contains(commit, "fixup") ||
			strings.Contains(commit, "WIP") ||
			strings.Contains(commit, "temp") {
			stale = append(stale, commit)
		}
	}

	return stale, nil
}

// handleStaleCommits attempts to remove or reorder stale commits
func (r *RebaseAttempt) handleStaleCommits(commits []string) error {
	// Use interactive rebase to drop stale commits
	// For now, just log what we would do
	// In a full implementation, this would use git rebase -i
	return fmt.Errorf("stale commit handling not yet implemented")
}

// performRebase executes the git rebase command
func (r *RebaseAttempt) performRebase() error {
	cmd := exec.Command("git", "-C", r.WorktreeDir, "rebase", r.BaseBranch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rebase failed: %v (output: %s)", err, string(output))
	}
	return nil
}

// canAutoResolve checks if current conflicts can be auto-resolved
func (r *RebaseAttempt) canAutoResolve() bool {
	// Check if all conflicts are in files we can auto-resolve
	conflicts, err := r.getConflictFiles()
	if err != nil {
		return false
	}

	// Try to resolve each conflict
	for _, conflict := range conflicts {
		if !isAutoResolvable(conflict) {
			return false
		}
	}

	return len(conflicts) > 0
}

// autoResolveConflicts attempts to automatically resolve simple conflicts
func (r *RebaseAttempt) autoResolveConflicts() error {
	conflicts, err := r.getConflictFiles()
	if err != nil {
		return err
	}

	for _, conflict := range conflicts {
		// Try to resolve using "ours" strategy for now
		// In a full implementation, this would be smarter
		cmd := exec.Command("git", "-C", r.WorktreeDir, "checkout", "--ours", conflict)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to resolve %s: %v", conflict, err)
		}

		// Stage the resolved file
		cmd = exec.Command("git", "-C", r.WorktreeDir, "add", conflict)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stage %s: %v", conflict, err)
		}
	}

	// Continue the rebase
	cmd := exec.Command("git", "-C", r.WorktreeDir, "rebase", "--continue")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to continue rebase: %v (output: %s)", err, string(output))
	}

	return nil
}

// getConflictFiles returns the list of files with conflicts
func (r *RebaseAttempt) getConflictFiles() ([]string, error) {
	cmd := exec.Command("git", "-C", r.WorktreeDir, "diff", "--name-only", "--diff-filter=U")
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

// isAutoResolvable checks if a file conflict can be automatically resolved
func isAutoResolvable(filepath string) bool {
	// Check file extension
	extensions := []string{
		".lock", // Lock files
		".sum",  // Go sum files
		".mod",  // Go mod files
	}

	for _, ext := range extensions {
		if strings.HasSuffix(filepath, ext) {
			return true
		}
	}

	return false
}

// AbortRebase aborts the current rebase operation
func (r *RebaseAttempt) AbortRebase() error {
	cmd := exec.Command("git", "-C", r.WorktreeDir, "rebase", "--abort")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to abort rebase: %v (output: %s)", err, string(output))
	}
	return nil
}
