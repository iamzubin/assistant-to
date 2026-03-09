package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "-C", repoDir, "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	cmd = exec.Command("git", "-C", repoDir, "config", "user.email", "test@example.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", repoDir, "config", "user.name", "Test User")
	cmd.Run()

	// Create initial commit
	initialFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	cmd = exec.Command("git", "-C", repoDir, "add", "README.md")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	cmd = exec.Command("git", "-C", repoDir, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create main branch just in case
	cmd = exec.Command("git", "-C", repoDir, "branch", "-M", "main")
	cmd.Run()

	return repoDir
}

func TestGitWorktreeLifecycle(t *testing.T) {
	repoDir := setupTestRepo(t)
	taskID := "test-task-123"
	baseBranch := "main"

	// 1. Create Worktree
	worktreeDir, err := CreateWorktree(repoDir, taskID, baseBranch, "")
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	// Verify worktree dir exists
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		t.Errorf("Worktree directory was not created: %s", worktreeDir)
	}

	// 2. Modify in worktree and commit
	testFile := filepath.Join(worktreeDir, "test_file.txt")
	if err := os.WriteFile(testFile, []byte("hello from worktree\n"), 0644); err != nil {
		t.Fatalf("Failed to write test file in worktree: %v", err)
	}

	cmd := exec.Command("git", "-C", worktreeDir, "add", "test_file.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add file in worktree: %v", err)
	}

	cmd = exec.Command("git", "-C", worktreeDir, "commit", "-m", "Add test file")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit in worktree: %v", err)
	}

	// 3. Merge Worktree
	if err := MergeWorktree(taskID, baseBranch, repoDir); err != nil {
		t.Fatalf("MergeWorktree failed: %v", err)
	}

	// Verify file is in main repo now
	mainTestFile := filepath.Join(repoDir, "test_file.txt")
	if _, err := os.Stat(mainTestFile); os.IsNotExist(err) {
		t.Errorf("File from worktree was not merged into base branch")
	}

	// 4. Teardown Worktree
	if err := TeardownWorktree(taskID, repoDir, ""); err != nil {
		t.Fatalf("TeardownWorktree failed: %v", err)
	}

	// Verify branch is deleted
	cmd = exec.Command("git", "-C", repoDir, "branch", "--list", "at-test-task-123")
	output, _ := cmd.CombinedOutput()
	if string(output) != "" {
		t.Errorf("Branch at-test-task-123 was not deleted")
	}

	// Verify worktree dir is deleted
	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Errorf("Worktree directory still exists: %s", worktreeDir)
	}
}

func TestTeardownAllWorktrees(t *testing.T) {
	repoDir := setupTestRepo(t)
	taskIDs := []string{"task-1", "task-2"}
	baseBranch := "main"

	var worktreeDirs []string
	for _, id := range taskIDs {
		dir, err := CreateWorktree(repoDir, id, baseBranch, "")
		if err != nil {
			t.Fatalf("CreateWorktree failed for %s: %v", id, err)
		}
		worktreeDirs = append(worktreeDirs, dir)
	}

	// Teardown all
	if err := TeardownAllWorktrees(repoDir, ""); err != nil {
		t.Fatalf("TeardownAllWorktrees failed: %v", err)
	}

	// Verify all branches are deleted
	for _, id := range taskIDs {
		branchName := "at-" + id
		cmd := exec.Command("git", "-C", repoDir, "branch", "--list", branchName)
		output, _ := cmd.CombinedOutput()
		if string(output) != "" {
			t.Errorf("Branch %s was not deleted", branchName)
		}
	}

	// Verify all worktree dirs are deleted
	for _, dir := range worktreeDirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("Worktree directory still exists: %s", dir)
		}
	}
}
