package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"assistant-to/internal/db"

	"github.com/spf13/cobra"
)

var (
	primeDomain string
	primeType   string
	primeJSON   bool
	primeRecent int
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Load config and relevant Expertise records into the current context",
	Long: `Prime loads project expertise, conventions, patterns, and failures
from the database to prepare an agent's context before starting work.

This command outputs formatted expertise that agents can consume
to avoid repeating past mistakes and follow project conventions.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		var expertise []db.Expertise

		// Determine what to load based on flags
		switch {
		case primeDomain != "":
			// Load by specific domain
			expertise, err = database.GetExpertiseByDomain(primeDomain)
			if err != nil {
				fmt.Printf("Failed to load expertise: %v\n", err)
				os.Exit(1)
			}
		case primeType != "":
			// Validate and load by type
			if !db.ValidateExpertiseType(primeType) {
				fmt.Printf("Invalid expertise type: %s\n", primeType)
				fmt.Printf("Valid types: %v\n", db.GetExpertiseTypes())
				os.Exit(1)
			}
			expertise, err = database.GetExpertiseByType(primeType)
			if err != nil {
				fmt.Printf("Failed to load expertise: %v\n", err)
				os.Exit(1)
			}
		case primeRecent > 0:
			// Load recent entries
			expertise, err = database.GetRecentExpertise(primeRecent)
			if err != nil {
				fmt.Printf("Failed to load expertise: %v\n", err)
				os.Exit(1)
			}
		default:
			// Load all expertise
			expertise, err = database.GetAllExpertise()
			if err != nil {
				fmt.Printf("Failed to load expertise: %v\n", err)
				os.Exit(1)
			}
		}

		// Output based on format preference
		if primeJSON {
			outputJSON(expertise)
		} else {
			outputText(expertise)
		}
	},
}

func outputJSON(expertise []db.Expertise) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(expertise); err != nil {
		fmt.Printf("Failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

func outputText(expertise []db.Expertise) {
	if len(expertise) == 0 {
		fmt.Println("No expertise records found. Use 'at record' to add knowledge.")
		return
	}

	// Group by type for better readability
	grouped := make(map[string][]db.Expertise)
	for _, e := range expertise {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	// Output in order: conventions, patterns, failures, decisions
	typeOrder := []string{db.ExpertiseTypeConvention, db.ExpertiseTypePattern, db.ExpertiseTypeFailure, db.ExpertiseTypeDecision}
	typeNames := map[string]string{
		db.ExpertiseTypeConvention: "📋 Conventions",
		db.ExpertiseTypePattern:    "🔧 Patterns",
		db.ExpertiseTypeFailure:    "⚠️  Failures to Avoid",
		db.ExpertiseTypeDecision:   "🏛️  Architectural Decisions",
	}

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           PROJECT EXPERTISE CONTEXT                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	for _, t := range typeOrder {
		if items, ok := grouped[t]; ok && len(items) > 0 {
			fmt.Printf("%s\n", typeNames[t])
			fmt.Println("─" + string(make([]byte, 60)))
			for _, e := range items {
				fmt.Printf("\n[%s] %s\n", e.Domain, e.Timestamp.Format("2006-01-02"))
				fmt.Printf("  %s\n", e.Description)
			}
			fmt.Println()
		}
	}

	fmt.Printf("Total records: %d\n", len(expertise))
}

func init() {
	primeCmd.Flags().StringVar(&primeDomain, "domain", "", "Filter by domain (e.g., 'db', 'api', 'ui')")
	primeCmd.Flags().StringVar(&primeType, "type", "", "Filter by type (convention, pattern, failure, decision)")
	primeCmd.Flags().BoolVar(&primeJSON, "json", false, "Output as JSON for programmatic consumption")
	primeCmd.Flags().IntVar(&primeRecent, "recent", 0, "Show only entries from last N days")

	RootCmd.AddCommand(primeCmd)
}
