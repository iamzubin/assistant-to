package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dwight/internal/db"
	"dwight/internal/intelligence"

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

		dbPath := filepath.Join(pwd, ".dwight", "code-intelligence.db")
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

		dbPath := filepath.Join(pwd, ".dwight", "code-intelligence.db")
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

		dbPath := filepath.Join(pwd, ".dwight", "code-intelligence.db")
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

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Index codebase and store as project knowledge",
	Long: `Indexes the codebase and stores the code structure as expertise entries
that agents can query. This builds a map of all functions, types, and files
that can be searched via expertise_list.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		codeIndexPath := filepath.Join(pwd, ".dwight", "code-intelligence.db")
		if intelligenceIndexFile != "" {
			codeIndexPath = intelligenceIndexFile
		}

		index, err := intelligence.NewCodeIndex(codeIndexPath)
		if err != nil {
			fmt.Printf("Failed to create code index: %v\n", err)
			os.Exit(1)
		}
		defer index.Close()

		projectPath := pwd
		if intelligenceProject != "" {
			projectPath = intelligenceProject
		}

		fmt.Println("Indexing codebase for knowledge map...")
		if err := index.IndexProject(projectPath); err != nil {
			fmt.Printf("Failed to index project: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".dwight", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		if err := database.InitSchema(); err != nil {
			fmt.Printf("Failed to initialize database schema: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Storing code map as expertise...")
		entryCount, err := storeCodeMapAsExpertise(database, index, projectPath)
		if err != nil {
			fmt.Printf("Failed to store code map: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Code map indexed and stored: %d entries\n", entryCount)
		fmt.Printf("  - Use 'dwight prime --domain <domain>' to query specific packages\n")
		fmt.Printf("  - Agents will automatically receive relevant expertise when working on tasks\n")
	},
}

func storeCodeMapAsExpertise(database *db.DB, index *intelligence.CodeIndex, projectPath string) (int, error) {
	var totalCount int

	packages, err := index.GetAllPackages()
	if err != nil {
		return 0, fmt.Errorf("failed to get packages: %w", err)
	}

	for _, pkg := range packages {
		files, err := index.GetPackageFiles(pkg.Path)
		if err != nil {
			continue
		}

		for _, file := range files {
			fileInfo, err := index.GetFileInfo(file)
			if err != nil {
				continue
			}

			domain := filepath.Dir(file)
			if domain == "." {
				domain = pkg.Path
			}

			if len(fileInfo.Types) > 0 {
				desc := fmt.Sprintf("Types in %s: %s", file, strings.Join(fileInfo.Types, ", "))
				_, err := database.RecordExpertise(domain, db.ExpertiseTypePattern, desc)
				if err == nil {
					totalCount++
				}
			}

			if len(fileInfo.Functions) > 0 {
				desc := fmt.Sprintf("Functions in %s: %s", file, strings.Join(fileInfo.Functions, ", "))
				_, err := database.RecordExpertise(domain, db.ExpertiseTypePattern, desc)
				if err == nil {
					totalCount++
				}
			}

			for _, method := range fileInfo.Methods {
				desc := fmt.Sprintf("Method %s.%s defined in %s", method.Receiver, method.Name, file)
				_, err := database.RecordExpertise(domain, db.ExpertiseTypePattern, desc)
				if err == nil {
					totalCount++
				}
			}

			if len(fileInfo.Imports) > 0 {
				desc := fmt.Sprintf("Imports in %s: %s", file, strings.Join(fileInfo.Imports, ", "))
				_, err := database.RecordExpertise(domain, db.ExpertiseTypePattern, desc)
				if err == nil {
					totalCount++
				}
			}
		}

		if len(pkg.Exported) > 0 {
			desc := fmt.Sprintf("Exported from %s: %s", pkg.Path, strings.Join(pkg.Exported, ", "))
			_, err := database.RecordExpertise(pkg.Path, db.ExpertiseTypePattern, desc)
			if err == nil {
				totalCount++
			}
		}
	}

	return totalCount, nil
}

func init() {
	intelligenceCmd.Flags().StringVar(&intelligenceIndexFile, "index", "", "Path to code intelligence database")
	intelligenceCmd.Flags().StringVar(&intelligenceProject, "project", "", "Project path to index")

	intelligenceCmd.AddCommand(indexCmd)
	intelligenceCmd.AddCommand(impactCmd)
	intelligenceCmd.AddCommand(depsCmd)
	intelligenceCmd.AddCommand(mapCmd)

	RootCmd.AddCommand(intelligenceCmd)
}
