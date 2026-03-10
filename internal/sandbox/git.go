package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CreateWorktree creates a new git worktree for a specific task.
// It creates a new branch named "at-<taskID>" based on the provided baseBranch.
// If the worktree or branch already exists, they will be cleaned up first.
// Returns the absolute path to the created worktree directory.
func CreateWorktree(repoDir string, taskID string, baseBranch string, worktreesDir string) (string, error) {
	if worktreesDir == "" {
		worktreesDir = filepath.Join(repoDir, ".dwight", "worktrees")
	}
	worktreeDir := filepath.Join(worktreesDir, taskID)
	branchName := "at-" + taskID

	// First, try to remove any existing worktree (this also removes the branch if it's checked out there)
	rmCmd := exec.Command("git", "-C", repoDir, "worktree", "remove", worktreeDir, "--force")
	rmCmd.CombinedOutput() // Ignore error - worktree might not exist

	// Clean up the directory if it still exists
	os.RemoveAll(worktreeDir)

	// Check if branch still exists (might be checked out elsewhere or not associated with worktree)
	branchCheckCmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", branchName)
	if _, err := branchCheckCmd.CombinedOutput(); err == nil {
		// Branch exists, try to delete it
		delBranchCmd := exec.Command("git", "-C", repoDir, "branch", "-D", branchName)
		delBranchCmd.CombinedOutput() // Ignore error
	}

	// Now create the new worktree and branch
	cmd := exec.Command("git", "-C", repoDir, "worktree", "add", worktreeDir, "-b", branchName, baseBranch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create git worktree for task %s: %w\nOutput: %s", taskID, err, string(output))
	}

	return worktreeDir, nil
}

// MergeWorktree merges a task branch into a base branch within the main repository.
func MergeWorktree(taskID string, baseBranch string, repoDir string) error {
	branchName := "at-" + taskID

	// Checkout base branch
	checkoutCmd := exec.Command("git", "-C", repoDir, "checkout", baseBranch)
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout base branch %s: %w\nOutput: %s", baseBranch, err, string(output))
	}

	// Merge the task branch
	mergeCmd := exec.Command("git", "-C", repoDir, "merge", branchName)
	if output, err := mergeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to merge branch %s into %s: %w\nOutput: %s", branchName, baseBranch, err, string(output))
	}

	return nil
}

// TeardownWorktree removes the worktree and deletes the associated branch.
func TeardownWorktree(taskID string, repoDir string, worktreesDir string) error {
	if worktreesDir == "" {
		worktreesDir = filepath.Join(repoDir, ".dwight", "worktrees")
	}
	worktreeDir := filepath.Join(worktreesDir, taskID)
	branchName := "at-" + taskID

	// Remove worktree
	rmCmd := exec.Command("git", "-C", repoDir, "worktree", "remove", worktreeDir, "--force")
	if output, err := rmCmd.CombinedOutput(); err != nil {
		// Log but continue to allow branch deletion
		fmt.Printf("Warning: failed to remove worktree directory %s: %v\nOutput: %s\n", worktreeDir, err, string(output))
	}

	// Delete branch
	delBranchCmd := exec.Command("git", "-C", repoDir, "branch", "-D", branchName)
	if output, err := delBranchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch %s: %w\nOutput: %s", branchName, err, string(output))
	}

	return nil
}

// TeardownAllWorktrees removes all managed worktrees and their associated branches.
func TeardownAllWorktrees(repoDir string, worktreesDir string) error {
	if worktreesDir == "" {
		worktreesDir = filepath.Join(repoDir, ".dwight", "worktrees")
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read worktrees directory: %w", err)
	}

	var errors []error
	for _, entry := range entries {
		if entry.IsDir() {
			taskID := entry.Name()
			fmt.Printf("Tearing down worktree for task %s...\n", taskID)
			if err := TeardownWorktree(taskID, repoDir, worktreesDir); err != nil {
				errors = append(errors, fmt.Errorf("task %s: %w", taskID, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to teardown some worktrees: %v", errors)
	}

	return nil
}

// RunGitCommand runs a git command with the provided arguments and returns the output
func RunGitCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git command failed: %w\nOutput: %s", err, string(output))
	}
	return output, nil
}
