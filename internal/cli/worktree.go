package cli

import (
	"fmt"
	"os"

	"assistant-to/internal/sandbox"

	"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage isolated git worktrees for tasks",
	Long:  `Provide commands to create, merge, and teardown git worktrees for individual tasks.`,
}

var createWorktreeCmd = &cobra.Command{
	Use:   "create <task-id> [base-branch]",
	Short: "Create a sandboxed git worktree",
	Long:  `Initializes a new git worktree located in .assistant-to/worktrees/<task-id>, checked out to a new branch for the task based on the specified base branch (defaulting to 'main').`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		baseBranch := "main"
		if len(args) > 1 {
			baseBranch = args[1]
		}

		pwd, _ := os.Getwd()
		fmt.Printf("Creating worktree for task %s based on %s...\n", taskID, baseBranch)
		dir, err := sandbox.CreateWorktree(pwd, taskID, baseBranch)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Success! Worktree created at: %s\n", dir)
	},
}

var mergeWorktreeCmd = &cobra.Command{
	Use:   "merge <task-id> [base-branch]",
	Short: "Merge a task's worktree back into the base branch",
	Long:  `Merges the task's isolated branch back into the specified base branch (defaulting to 'main') in the primary project repository.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		baseBranch := "main"
		if len(args) > 1 {
			baseBranch = args[1]
		}

		pwd, _ := os.Getwd()
		fmt.Printf("Merging worktree for task %s into %s...\n", taskID, baseBranch)
		err := sandbox.MergeWorktree(taskID, baseBranch, pwd)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Success! Worktree successfully merged.")
	},
}

var teardownAll bool

var teardownWorktreeCmd = &cobra.Command{
	Use:   "teardown [task-id]",
	Short: "Remove a worktree and its branch",
	Long:  `Deletes the git worktree associated with the specified task and removes its local branch from the project directory. Use --all to remove all managed worktrees.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if teardownAll {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		pwd, _ := os.Getwd()

		if teardownAll {
			fmt.Println("Tearing down all worktrees...")
			err := sandbox.TeardownAllWorktrees(pwd)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Success! All managed worktrees successfully torn down.")
			return
		}

		taskID := args[0]
		fmt.Printf("Tearing down worktree for task %s...\n", taskID)
		err := sandbox.TeardownWorktree(taskID, pwd)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Success! Worktree successfully torn down.")
	},
}

func init() {
	teardownWorktreeCmd.Flags().BoolVarP(&teardownAll, "all", "a", false, "Teardown all worktrees")
	worktreeCmd.AddCommand(createWorktreeCmd)
	worktreeCmd.AddCommand(mergeWorktreeCmd)
	worktreeCmd.AddCommand(teardownWorktreeCmd)
	rootCmd.AddCommand(worktreeCmd)
}
