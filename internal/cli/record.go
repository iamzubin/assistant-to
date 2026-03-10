package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dwight/internal/db"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	recordDomain      string
	recordType        string
	recordDescription string
	recordInteractive bool
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a new learning (pattern/failure/decision) into the Expertise table",
	Long: `Record captures tribal knowledge about the project:
- Conventions: Coding standards and style preferences
- Patterns: Successful solution patterns that work well
- Failures: Known pitfalls and mistakes to avoid  
- Decisions: Architectural decisions and their rationale

This knowledge is automatically loaded by 'dwight prime' before agents start work.`,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		dbPath := filepath.Join(pwd, ".dwight", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		// Use interactive form if no flags provided or explicitly requested
		if recordInteractive || (recordDomain == "" && recordType == "" && recordDescription == "") {
			err := runInteractiveForm()
			if err != nil {
				if err == huh.ErrUserAborted {
					fmt.Println("Cancelled.")
					return
				}
				fmt.Printf("Form error: %v\n", err)
				os.Exit(1)
			}
		}

		// Validate inputs
		if recordDomain == "" {
			fmt.Println("Error: Domain is required")
			os.Exit(1)
		}

		if recordType == "" {
			fmt.Println("Error: Type is required")
			os.Exit(1)
		}

		if !db.ValidateExpertiseType(recordType) {
			fmt.Printf("Error: Invalid type '%s'\n", recordType)
			fmt.Printf("Valid types: %v\n", db.GetExpertiseTypes())
			os.Exit(1)
		}

		if recordDescription == "" {
			fmt.Println("Error: Description is required")
			os.Exit(1)
		}

		// Insert into database
		id, err := database.RecordExpertise(recordDomain, recordType, recordDescription)
		if err != nil {
			fmt.Printf("Failed to record expertise: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Recorded %s in domain '%s' (ID: %d)\n", recordType, recordDomain, id)
	},
}

func runInteractiveForm() error {
	// Type selection options
	typeOptions := []huh.Option[string]{
		huh.NewOption("📋 Convention - Coding standard or style preference", db.ExpertiseTypeConvention),
		huh.NewOption("🔧 Pattern - Successful solution pattern", db.ExpertiseTypePattern),
		huh.NewOption("⚠️  Failure - Pitfall or mistake to avoid", db.ExpertiseTypeFailure),
		huh.NewOption("🏛️  Decision - Architectural decision", db.ExpertiseTypeDecision),
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Domain").
				Description("What area does this knowledge apply to? (e.g., 'db', 'api', 'auth')").
				Value(&recordDomain).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("domain is required")
					}
					return nil
				}),

			huh.NewSelect[string]().
				Title("Type of Knowledge").
				Description("What kind of expertise are you recording?").
				Options(typeOptions...).
				Value(&recordType),

			huh.NewText().
				Title("Description").
				Description("Describe the convention, pattern, failure, or decision").
				Lines(5).
				Value(&recordDescription).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("description is required")
					}
					return nil
				}),
		),
	)

	return form.Run()
}

func init() {
	recordCmd.Flags().StringVar(&recordDomain, "domain", "", "Domain this knowledge applies to (e.g., 'db', 'api')")
	recordCmd.Flags().StringVar(&recordType, "type", "", "Type of knowledge (convention, pattern, failure, decision)")
	recordCmd.Flags().StringVar(&recordDescription, "description", "", "Description of the knowledge")
	recordCmd.Flags().BoolVarP(&recordInteractive, "interactive", "i", false, "Force interactive form mode")

	RootCmd.AddCommand(recordCmd)
}
