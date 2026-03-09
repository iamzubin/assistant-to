package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"assistant-to/internal/intelligence"

	"github.com/spf13/cobra"
)

var (
	intelligenceIndexFile string
	intelligenceProject   string
)

var intelligenceCmd = &cobra.Command{
	Use:     "intelligence",
	Short:   "Code intelligence and analysis tools",
	Aliases: []string{"intel"},
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index the codebase for analysis",
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "code-intelligence.db")
		if intelligenceIndexFile != "" {
			dbPath = intelligenceIndexFile
		}

		index, err := intelligence.NewCodeIndex(dbPath)
		if err != nil {
			fmt.Printf("Failed to create code index: %v\n", err)
			os.Exit(1)
		}
		defer index.Close()

		projectPath := pwd
		if intelligenceProject != "" {
			projectPath = intelligenceProject
		}

		fmt.Println("Indexing codebase...")
		if err := index.IndexProject(projectPath); err != nil {
			fmt.Printf("Failed to index project: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Indexing complete: %s\n", dbPath)
	},
}

var impactCmd = &cobra.Command{
	Use:   "impact <file>",
	Short: "Analyze the impact of changing a file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]

		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "code-intelligence.db")
		if intelligenceIndexFile != "" {
			dbPath = intelligenceIndexFile
		}

		// Check if index exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("Code index not found. Run 'dwight intelligence index' first.")
			os.Exit(1)
		}

		index, err := intelligence.NewCodeIndex(dbPath)
		if err != nil {
			fmt.Printf("Failed to open code index: %v\n", err)
			os.Exit(1)
		}
		defer index.Close()

		analyzer := intelligence.NewImpactAnalyzer(index)

		// Make path relative if it's absolute
		if filepath.IsAbs(filePath) {
			relPath, err := filepath.Rel(pwd, filePath)
			if err == nil {
				filePath = relPath
			}
		}

		fmt.Printf("Analyzing impact of: %s\n", filePath)

		report, err := analyzer.AnalyzeChangeImpact(filePath)
		if err != nil {
			fmt.Printf("Failed to analyze impact: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(intelligence.FormatReport(report))
	},
}

var depsCmd = &cobra.Command{
	Use:   "deps <file>",
	Short: "Show dependencies of a file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]

		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "code-intelligence.db")
		if intelligenceIndexFile != "" {
			dbPath = intelligenceIndexFile
		}

		// Check if index exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("Code index not found. Run 'dwight intelligence index' first.")
			os.Exit(1)
		}

		index, err := intelligence.NewCodeIndex(dbPath)
		if err != nil {
			fmt.Printf("Failed to open code index: %v\n", err)
			os.Exit(1)
		}
		defer index.Close()

		// Make path relative if it's absolute
		if filepath.IsAbs(filePath) {
			relPath, err := filepath.Rel(pwd, filePath)
			if err == nil {
				filePath = relPath
			}
		}

		deps, err := index.SearchDependencies(filePath)
		if err != nil {
			fmt.Printf("Failed to search dependencies: %v\n", err)
			os.Exit(1)
		}

		if len(deps) == 0 {
			fmt.Printf("No dependencies found for: %s\n", filePath)
			return
		}

		fmt.Printf("\nDependencies of: %s\n", filePath)
		fmt.Println("─" + string(make([]byte, 60)))
		for _, dep := range deps {
			fmt.Printf("  • %s\n", dep)
		}
		fmt.Printf("\nTotal: %d files\n", len(deps))
	},
}

func init() {
	intelligenceCmd.Flags().StringVar(&intelligenceIndexFile, "index", "", "Path to code intelligence database")
	intelligenceCmd.Flags().StringVar(&intelligenceProject, "project", "", "Project path to index")

	intelligenceCmd.AddCommand(indexCmd)
	intelligenceCmd.AddCommand(impactCmd)
	intelligenceCmd.AddCommand(depsCmd)

	RootCmd.AddCommand(intelligenceCmd)
}
