package intelligence

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ImpactAnalyzer analyzes the impact of changes to specific files
type ImpactAnalyzer struct {
	Index *CodeIndex
}

// NewImpactAnalyzer creates a new impact analyzer
func NewImpactAnalyzer(index *CodeIndex) *ImpactAnalyzer {
	return &ImpactAnalyzer{
		Index: index,
	}
}

// ImpactReport represents the analysis of changes to a file
type ImpactReport struct {
	TargetFile       string
	DirectDependents []string
	TransitiveDeps   []string
	AffectedPackages []string
	RiskLevel        RiskLevel
	Recommendations  []string
}

// RiskLevel represents the risk of changing a file
type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// AnalyzeChangeImpact analyzes how changing a file would affect the codebase
func (ia *ImpactAnalyzer) AnalyzeChangeImpact(filePath string) (*ImpactReport, error) {
	report := &ImpactReport{
		TargetFile: filePath,
	}

	// Get direct dependents
	directDeps, err := ia.Index.SearchDependencies(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find dependencies: %w", err)
	}
	report.DirectDependents = directDeps

	// Get transitive dependencies (simplified - one level deep)
	transitiveMap := make(map[string]bool)
	for _, dep := range directDeps {
		deps, err := ia.Index.SearchDependencies(dep)
		if err != nil {
			continue
		}
		for _, d := range deps {
			if d != filePath && !contains(directDeps, d) {
				transitiveMap[d] = true
			}
		}
	}
	for dep := range transitiveMap {
		report.TransitiveDeps = append(report.TransitiveDeps, dep)
	}

	// Get affected packages
	affectedPkgs := make(map[string]bool)
	for _, dep := range directDeps {
		pkg := filepath.Dir(dep)
		affectedPkgs[pkg] = true
	}
	for pkg := range affectedPkgs {
		report.AffectedPackages = append(report.AffectedPackages, pkg)
	}

	// Calculate risk level
	report.RiskLevel = ia.calculateRiskLevel(report)

	// Generate recommendations
	report.Recommendations = ia.generateRecommendations(report)

	return report, nil
}

// calculateRiskLevel determines the risk of changing a file
func (ia *ImpactAnalyzer) calculateRiskLevel(report *ImpactReport) RiskLevel {
	totalDeps := len(report.DirectDependents) + len(report.TransitiveDeps)

	// Critical: affects many files across packages
	if totalDeps > 20 || len(report.AffectedPackages) > 5 {
		return RiskCritical
	}

	// High: affects multiple packages or many files
	if totalDeps > 10 || len(report.AffectedPackages) > 2 {
		return RiskHigh
	}

	// Medium: has some dependents
	if totalDeps > 0 {
		return RiskMedium
	}

	// Low: no dependents or isolated
	return RiskLow
}

// generateRecommendations creates advice based on the impact analysis
func (ia *ImpactAnalyzer) generateRecommendations(report *ImpactReport) []string {
	var recs []string

	switch report.RiskLevel {
	case RiskCritical:
		recs = append(recs, "⚠️  CRITICAL: This change affects many files across multiple packages")
		recs = append(recs, "   - Consider breaking into smaller changes")
		recs = append(recs, "   - Ensure comprehensive test coverage")
		recs = append(recs, "   - Coordinate with other developers")
		recs = append(recs, fmt.Sprintf("   - %d files directly affected", len(report.DirectDependents)))
		recs = append(recs, fmt.Sprintf("   - %d files transitively affected", len(report.TransitiveDeps)))

	case RiskHigh:
		recs = append(recs, "⚠️  HIGH RISK: This change has significant impact")
		recs = append(recs, "   - Run full test suite before committing")
		recs = append(recs, "   - Consider impact on dependent packages")
		recs = append(recs, fmt.Sprintf("   - Affects %d packages", len(report.AffectedPackages)))

	case RiskMedium:
		recs = append(recs, "⚡ MEDIUM RISK: Some dependent files")
		recs = append(recs, "   - Run package-level tests")
		recs = append(recs, fmt.Sprintf("   - %d dependent files", len(report.DirectDependents)))

	case RiskLow:
		recs = append(recs, "✅ LOW RISK: Isolated change")
		recs = append(recs, "   - Standard testing should suffice")
	}

	if len(report.DirectDependents) > 0 {
		recs = append(recs, "")
		recs = append(recs, "Directly affected files:")
		for i, dep := range report.DirectDependents {
			if i >= 10 {
				recs = append(recs, fmt.Sprintf("   ... and %d more", len(report.DirectDependents)-10))
				break
			}
			recs = append(recs, fmt.Sprintf("   - %s", dep))
		}
	}

	return recs
}

// contains checks if a string slice contains a value
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// FormatReport formats the impact report for display
func FormatReport(report *ImpactReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n╔════════════════════════════════════════════════════════════╗\n"))
	sb.WriteString(fmt.Sprintf("║           IMPACT ANALYSIS REPORT                           ║\n"))
	sb.WriteString(fmt.Sprintf("╚════════════════════════════════════════════════════════════╝\n\n"))

	sb.WriteString(fmt.Sprintf("Target File: %s\n", report.TargetFile))
	sb.WriteString(fmt.Sprintf("Risk Level:  %s\n", report.RiskLevel))
	sb.WriteString(fmt.Sprintf("\n"))

	sb.WriteString(fmt.Sprintf("Impact Summary:\n"))
	sb.WriteString(fmt.Sprintf("  Direct Dependents:  %d\n", len(report.DirectDependents)))
	sb.WriteString(fmt.Sprintf("  Transitive Impact:  %d\n", len(report.TransitiveDeps)))
	sb.WriteString(fmt.Sprintf("  Affected Packages:  %d\n", len(report.AffectedPackages)))
	sb.WriteString(fmt.Sprintf("\n"))

	sb.WriteString(fmt.Sprintf("Recommendations:\n"))
	for _, rec := range report.Recommendations {
		sb.WriteString(fmt.Sprintf("%s\n", rec))
	}

	return sb.String()
}
