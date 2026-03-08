package merge

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ResolutionTier represents the current tier in the 4-tier merge resolution
type ResolutionTier int

const (
	Tier1Mechanical ResolutionTier = iota
	Tier2Synthesis
	Tier3Rebase
	Tier4AIAssisted
)

func (t ResolutionTier) String() string {
	switch t {
	case Tier1Mechanical:
		return "Tier1_Mechanical"
	case Tier2Synthesis:
		return "Tier2_Algorithmic_Synthesis"
	case Tier3Rebase:
		return "Tier3_Contextual_Rebase"
	case Tier4AIAssisted:
		return "Tier4_AI_Assisted"
	default:
		return "Unknown"
	}
}

// ResolutionResult represents the outcome of a merge resolution attempt
type ResolutionResult struct {
	Tier      ResolutionTier
	Success   bool
	Message   string
	Conflicts []string
	Strategy  string
}

// Resolver handles the 4-tier merge resolution strategy
type Resolver struct {
	WorktreeDir string
	BaseBranch  string
}

// NewResolver creates a new merge resolver for a worktree
func NewResolver(worktreeDir, baseBranch string) *Resolver {
	return &Resolver{
		WorktreeDir: worktreeDir,
		BaseBranch:  baseBranch,
	}
}

// AttemptResolution tries to resolve conflicts using the 4-tier strategy
func (r *Resolver) AttemptResolution() (*ResolutionResult, error) {
	// Tier 1: Mechanical Merge
	result, err := r.attemptMechanicalMerge()
	if err == nil && result.Success {
		return result, nil
	}

	// Tier 2: Algorithmic Synthesis for structured files
	result, err = r.attemptAlgorithmicSynthesis()
	if err == nil && result.Success {
		return result, nil
	}

	// Tier 3: Contextual Rebase
	result, err = r.attemptContextualRebase()
	if err == nil && result.Success {
		return result, nil
	}

	// Tier 4: AI-Assisted Resolution
	result, err = r.attemptAIAssistedResolution()
	if err == nil && result.Success {
		return result, nil
	}

	// All tiers failed
	return &ResolutionResult{
		Tier:     Tier4AIAssisted,
		Success:  false,
		Message:  "All resolution tiers failed",
		Strategy: "manual_intervention_required",
	}, nil
}

// attemptMechanicalMerge tries a standard git merge
func (r *Resolver) attemptMechanicalMerge() (*ResolutionResult, error) {
	// Implementation would use git commands
	// For now, return as not successful to proceed to next tier
	return &ResolutionResult{
		Tier:     Tier1Mechanical,
		Success:  false,
		Message:  "Mechanical merge would be attempted here",
		Strategy: "git_merge",
	}, nil
}

// attemptAlgorithmicSynthesis tries union-merge for structured files
func (r *Resolver) attemptAlgorithmicSynthesis() (*ResolutionResult, error) {
	conflicts, err := r.detectConflicts()
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}

	if len(conflicts) == 0 {
		return &ResolutionResult{
			Tier:     Tier2Synthesis,
			Success:  true,
			Message:  "No conflicts detected",
			Strategy: "no_conflicts",
		}, nil
	}

	// Try to resolve structured file conflicts
	resolved := 0
	unresolved := []string{}

	for _, conflict := range conflicts {
		strategy := r.getSynthesisStrategy(conflict)
		if strategy == nil {
			unresolved = append(unresolved, conflict)
			continue
		}

		err := strategy.Resolve(conflict)
		if err != nil {
			unresolved = append(unresolved, conflict)
			continue
		}
		resolved++
	}

	if resolved > 0 && len(unresolved) == 0 {
		return &ResolutionResult{
			Tier:      Tier2Synthesis,
			Success:   true,
			Message:   fmt.Sprintf("Resolved %d structured file conflicts via algorithmic synthesis", resolved),
			Conflicts: unresolved,
			Strategy:  "union_merge",
		}, nil
	}

	return &ResolutionResult{
		Tier:      Tier2Synthesis,
		Success:   false,
		Message:   fmt.Sprintf("Resolved %d of %d conflicts, %d remain", resolved, len(conflicts), len(unresolved)),
		Conflicts: unresolved,
		Strategy:  "partial_union_merge",
	}, nil
}

// attemptContextualRebase tries to rebase onto latest base branch
func (r *Resolver) attemptContextualRebase() (*ResolutionResult, error) {
	// Implementation would attempt automatic rebase
	return &ResolutionResult{
		Tier:     Tier3Rebase,
		Success:  false,
		Message:  "Contextual rebase would be attempted here",
		Strategy: "git_rebase",
	}, nil
}

// attemptAIAssistedResolution spawns a Merger agent
func (r *Resolver) attemptAIAssistedResolution() (*ResolutionResult, error) {
	// Implementation would spawn Merger agent
	return &ResolutionResult{
		Tier:     Tier4AIAssisted,
		Success:  false,
		Message:  "AI-assisted resolution would be attempted here",
		Strategy: "merger_agent",
	}, nil
}

// detectConflicts identifies files with merge conflicts
func (r *Resolver) detectConflicts() ([]string, error) {
	// This would run git diff --name-only --diff-filter=U
	// For now, return empty
	return []string{}, nil
}

// getSynthesisStrategy returns the appropriate synthesis strategy for a file
func (r *Resolver) getSynthesisStrategy(filepath string) SynthesisStrategy {
	ext := strings.ToLower(filepath)

	if strings.HasSuffix(ext, ".json") || strings.HasSuffix(ext, ".jsonl") {
		return &JSONSynthesis{}
	}

	if strings.HasSuffix(ext, ".yaml") || strings.HasSuffix(ext, ".yml") {
		return &YAMLSynthesis{}
	}

	return nil
}

// SynthesisStrategy defines the interface for algorithmic conflict resolution
type SynthesisStrategy interface {
	Resolve(filepath string) error
}

// JSONSynthesis handles union-merge for JSON/JSONL files
type JSONSynthesis struct{}

func (j *JSONSynthesis) Resolve(filepath string) error {
	// Read conflicting versions
	ours, theirs, base, err := readConflictVersions(filepath)
	if err != nil {
		return err
	}

	// Try to parse as JSON
	var oursObj, theirsObj, baseObj map[string]interface{}

	if err := json.Unmarshal([]byte(ours), &oursObj); err != nil {
		return fmt.Errorf("failed to parse ours as JSON: %w", err)
	}

	if err := json.Unmarshal([]byte(theirs), &theirsObj); err != nil {
		return fmt.Errorf("failed to parse theirs as JSON: %w", err)
	}

	if base != "" {
		if err := json.Unmarshal([]byte(base), &baseObj); err != nil {
			// Base might not be valid, that's okay
			baseObj = nil
		}
	}

	// Perform union merge
	merged := unionMergeJSON(oursObj, theirsObj, baseObj)

	// Write result
	result, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal merged JSON: %w", err)
	}

	if err := os.WriteFile(filepath, result, 0644); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	return nil
}

// YAMLSynthesis handles union-merge for YAML files
type YAMLSynthesis struct{}

func (y *YAMLSynthesis) Resolve(filepath string) error {
	// Similar to JSON but for YAML
	// For now, return error to skip
	return fmt.Errorf("YAML synthesis not yet implemented")
}

// readConflictVersions reads the three versions of a conflicted file
func readConflictVersions(filepath string) (ours, theirs, base string, err error) {
	// This would parse git conflict markers
	// For now, return empty strings
	content, err := os.ReadFile(filepath)
	if err != nil {
		return "", "", "", err
	}

	// Parse conflict markers
	// <<<<<<< ours
	// content
	// =======
	// content
	// >>>>>>> theirs
	parts := parseConflictMarkers(string(content))
	return parts[0], parts[1], parts[2], nil
}

// parseConflictMarkers extracts the three versions from conflicted file content
func parseConflictMarkers(content string) [3]string {
	var result [3]string

	lines := strings.Split(content, "\n")
	var currentSection string
	var oursLines, theirsLines, baseLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "<<<<<<< ") {
			currentSection = "ours"
			continue
		}

		if trimmed == "=======" {
			currentSection = "theirs"
			continue
		}

		if strings.HasPrefix(trimmed, ">>>>>>> ") {
			currentSection = ""
			continue
		}

		switch currentSection {
		case "ours":
			oursLines = append(oursLines, line)
		case "theirs":
			theirsLines = append(theirsLines, line)
		default:
			// Before conflict markers, treat as base
			if len(oursLines) == 0 {
				baseLines = append(baseLines, line)
			}
		}
	}

	result[0] = strings.Join(oursLines, "\n")
	result[1] = strings.Join(theirsLines, "\n")
	result[2] = strings.Join(baseLines, "\n")

	return result
}

// unionMergeJSON performs a union merge on two JSON objects
func unionMergeJSON(ours, theirs, base map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Add all keys from ours
	for k, v := range ours {
		result[k] = v
	}

	// Add keys from theirs that don't conflict
	for k, v := range theirs {
		if existing, ok := result[k]; ok {
			// Key exists in both, check if values are the same
			if !jsonEqual(existing, v) {
				// Conflict - if base exists and one matches base, use the other
				if base != nil {
					if baseVal, ok := base[k]; ok {
						if jsonEqual(existing, baseVal) {
							// Ours matches base, use theirs
							result[k] = v
						}
						// Theirs matches base, keep ours (already in result)
						// Neither matches - keep ours as default
					}
				}
			}
		} else {
			result[k] = v
		}
	}

	return result
}

// jsonEqual checks if two JSON values are equal
func jsonEqual(a, b interface{}) bool {
	// Simple comparison for basic types
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case nil:
		return b == nil
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if bv[k] == nil || !jsonEqual(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
