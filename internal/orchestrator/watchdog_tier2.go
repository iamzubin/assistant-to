package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dwight/internal/db"
	"dwight/internal/sandbox"
)

// Tier2Watchdog (Monitor Agent) detects objective drift in Builder agents
type Tier2Watchdog struct {
	DB       *db.DB
	PWD      string
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewTier2Watchdog creates a new Tier 2 Monitor watchdog
func NewTier2Watchdog(database *db.DB, pwd string) *Tier2Watchdog {
	return &Tier2Watchdog{
		DB:       database,
		PWD:      pwd,
		interval: 5 * time.Minute, // Check every 5 minutes
		stopCh:   make(chan struct{}),
	}
}

// Start begins the monitoring loop
func (t *Tier2Watchdog) Start(ctx context.Context) {
	log.Println("Tier2: Starting Monitor Agent watchdog")
	t.DB.RecordEvent("Monitor", "monitor_started", "Tier 2 Monitor Agent started")

	t.wg.Add(1)
	go t.monitoringLoop(ctx)
}

// Stop gracefully stops the monitoring loop
func (t *Tier2Watchdog) Stop() {
	log.Println("Tier2: Stopping Monitor Agent watchdog")
	close(t.stopCh)
	t.wg.Wait()
	t.DB.RecordEvent("Monitor", "monitor_stopped", "Tier 2 Monitor Agent stopped")
}

// monitoringLoop runs the periodic drift detection
func (t *Tier2Watchdog) monitoringLoop(ctx context.Context) {
	defer t.wg.Done()

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	// Run immediately on start
	t.runDriftChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Tier2: Context cancelled, stopping monitor")
			return
		case <-t.stopCh:
			log.Println("Tier2: Stop signal received, stopping monitor")
			return
		case <-ticker.C:
			t.runDriftChecks(ctx)
		}
	}
}

// runDriftChecks performs drift detection on all active builders
func (t *Tier2Watchdog) runDriftChecks(ctx context.Context) {
	// Get all tasks in "building" or "scouted" status
	tasks, err := t.getActiveTasks()
	if err != nil {
		log.Printf("Tier2: Failed to get active tasks: %v", err)
		return
	}

	if len(tasks) == 0 {
		return // Nothing to monitor
	}

	log.Printf("Tier2: Monitoring %d active task(s) for drift", len(tasks))

	for _, task := range tasks {
		agentID := fmt.Sprintf("builder-%d", task.ID)

		// Check if agent is still active (has recent events)
		isActive, err := t.isAgentActive(agentID)
		if err != nil {
			log.Printf("Tier2: Failed to check activity for %s: %v", agentID, err)
			continue
		}

		if !isActive {
			continue // Skip inactive agents
		}

		// Perform drift check
		drift, err := t.checkForDrift(ctx, task, agentID)
		if err != nil {
			log.Printf("Tier2: Drift check failed for %s: %v", agentID, err)
			continue
		}

		if drift != nil && drift.Detected {
			t.reportDrift(ctx, agentID, task, drift)
		}
	}
}

// getActiveTasks retrieves tasks that are currently being worked on
func (t *Tier2Watchdog) getActiveTasks() ([]db.Task, error) {
	// Get building tasks
	buildingTasks, err := t.DB.ListTasksByStatus(db.TaskStatusBuilding)
	if err != nil {
		return nil, err
	}

	// Get scouted tasks
	scoutedTasks, err := t.DB.ListTasksByStatus(db.TaskStatusScouted)
	if err != nil {
		return nil, err
	}

	// Get started tasks (just started, not yet building)
	startedTasks, err := t.DB.ListTasksByStatus(db.TaskStatusStarted)
	if err != nil {
		return nil, err
	}

	// Combine all active tasks
	allTasks := append(buildingTasks, scoutedTasks...)
	allTasks = append(allTasks, startedTasks...)

	return allTasks, nil
}

// isAgentActive checks if an agent has been active in the last 2 minutes
func (t *Tier2Watchdog) isAgentActive(agentID string) (bool, error) {
	lastHeartbeat, err := t.DB.GetLastHeartbeat(agentID)
	if err != nil {
		return false, err
	}

	// If no heartbeat, agent is not active
	if lastHeartbeat.IsZero() {
		return false, nil
	}

	// Check if active in last 2 minutes
	return time.Since(lastHeartbeat) < 2*time.Minute, nil
}

// DriftReport contains information about detected objective drift
type DriftReport struct {
	Detected          bool
	DriftType         DriftType
	Severity          DriftSeverity
	Evidence          []string
	FilesModified     []string
	FilesOutsideScope []string
	Recommendation    string
}

// DriftType categorizes the type of objective drift
type DriftType int

const (
	DriftNone DriftType = iota
	DriftScopeCreep
	DriftWrongDirection
	DriftOverEngineering
	DriftYakShaving
	DriftMissingRequirements
)

func (d DriftType) String() string {
	switch d {
	case DriftScopeCreep:
		return "scope_creep"
	case DriftWrongDirection:
		return "wrong_direction"
	case DriftOverEngineering:
		return "over_engineering"
	case DriftYakShaving:
		return "yak_shaving"
	case DriftMissingRequirements:
		return "missing_requirements"
	default:
		return "none"
	}
}

// DriftSeverity indicates how serious the drift is
type DriftSeverity int

const (
	SeverityLow DriftSeverity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

func (s DriftSeverity) String() string {
	switch s {
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// checkForDrift analyzes a task and its worktree for objective drift
func (t *Tier2Watchdog) checkForDrift(ctx context.Context, task db.Task, agentID string) (*DriftReport, error) {
	// Get worktree path
	worktreeDir := filepath.Join(t.PWD, ".dwight", "worktrees", fmt.Sprintf("%d", task.ID))

	// Check if worktree exists
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		return nil, nil // No worktree yet, nothing to check
	}

	report := &DriftReport{
		Detected: false,
		Evidence: []string{},
	}

	// 1. Get list of modified files
	modifiedFiles, err := t.getModifiedFiles(worktreeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get modified files: %w", err)
	}
	report.FilesModified = modifiedFiles

	// 2. Check if files are within target scope
	scopeViolations := t.checkScopeCompliance(task, modifiedFiles)
	report.FilesOutsideScope = scopeViolations

	if len(scopeViolations) > 3 {
		report.Detected = true
		report.DriftType = DriftScopeCreep
		report.Severity = SeverityHigh
		report.Evidence = append(report.Evidence,
			fmt.Sprintf("Modified %d files outside target scope", len(scopeViolations)))
		report.Recommendation = "Stop and redirect - too many files touched outside scope"
	} else if len(scopeViolations) > 0 {
		report.Evidence = append(report.Evidence,
			fmt.Sprintf("Modified %d files outside target scope (may be acceptable)", len(scopeViolations)))
	}

	// 3. Analyze commit messages for drift indicators
	commits, err := t.getRecentCommits(worktreeDir)
	if err != nil {
		log.Printf("Tier2: Failed to get commits for %s: %v", agentID, err)
	} else {
		driftIndicators := t.analyzeCommits(commits, task)
		report.Evidence = append(report.Evidence, driftIndicators...)

		if len(driftIndicators) > 2 {
			report.Detected = true
			if report.DriftType == DriftNone {
				report.DriftType = DriftWrongDirection
			}
			if report.Severity < SeverityMedium {
				report.Severity = SeverityMedium
			}
		}
	}

	// 4. Check for over-engineering indicators
	if t.detectOverEngineering(worktreeDir, modifiedFiles) {
		if !report.Detected {
			report.Detected = true
			report.DriftType = DriftOverEngineering
			report.Severity = SeverityMedium
		}
		report.Evidence = append(report.Evidence, "Detected potential over-engineering (complex abstractions)")
		if report.Recommendation == "" {
			report.Recommendation = "Review for unnecessary complexity"
		}
	}

	// 5. Check for yak shaving (lots of small unrelated changes)
	if len(modifiedFiles) > 10 && len(scopeViolations) > 5 {
		if !report.Detected {
			report.Detected = true
			report.DriftType = DriftYakShaving
			report.Severity = SeverityHigh
		}
		report.Evidence = append(report.Evidence,
			fmt.Sprintf("Yak shaving detected: %d files modified with many outside scope", len(modifiedFiles)))
		report.Recommendation = "Stop and refocus on core task"
	}

	return report, nil
}

// getModifiedFiles returns a list of files modified in the worktree
func (t *Tier2Watchdog) getModifiedFiles(worktreeDir string) ([]string, error) {
	cmd := []string{"-C", worktreeDir, "status", "--porcelain"}
	output, err := sandbox.RunGitCommand(cmd...)
	if err != nil {
		return nil, err
	}

	var files []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		// Parse porcelain format: XY filename
		// where X and Y are status codes
		file := strings.TrimSpace(line[2:])
		if file != "" {
			files = append(files, file)
		}
	}

	return files, nil
}

// checkScopeCompliance checks if modified files are within the task's target scope
func (t *Tier2Watchdog) checkScopeCompliance(task db.Task, modifiedFiles []string) []string {
	if task.TargetFiles == "" {
		return nil // No target files specified, can't check scope
	}

	targetFiles := strings.Split(task.TargetFiles, ",")
	for i := range targetFiles {
		targetFiles[i] = strings.TrimSpace(targetFiles[i])
	}

	var violations []string
	for _, file := range modifiedFiles {
		inScope := false
		for _, target := range targetFiles {
			if strings.Contains(file, target) || strings.Contains(target, file) {
				inScope = true
				break
			}
		}
		if !inScope {
			violations = append(violations, file)
		}
	}

	return violations
}

// getRecentCommits gets the last 10 commit messages
func (t *Tier2Watchdog) getRecentCommits(worktreeDir string) ([]string, error) {
	cmd := []string{"-C", worktreeDir, "log", "--oneline", "-10"}
	output, err := sandbox.RunGitCommand(cmd...)
	if err != nil {
		return nil, err
	}

	var commits []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line != "" {
			commits = append(commits, line)
		}
	}

	return commits, nil
}

// analyzeCommits looks for drift indicators in commit messages
func (t *Tier2Watchdog) analyzeCommits(commits []string, task db.Task) []string {
	var indicators []string

	// Keywords that suggest drift
	driftKeywords := []string{
		"refactor",
		"cleanup",
		"restructure",
		"redesign",
		"rewrite",
		"reorganize",
		"extract",
		"abstract",
		"framework",
		"generic",
		"universal",
		"all",
		"every",
	}

	// Check if commits seem unrelated to task
	taskKeywords := strings.Fields(strings.ToLower(task.Title + " " + task.Description))

	for _, commit := range commits {
		commitLower := strings.ToLower(commit)

		// Check for drift keywords
		for _, keyword := range driftKeywords {
			if strings.Contains(commitLower, keyword) {
				indicators = append(indicators, fmt.Sprintf("Commit suggests scope expansion: '%s'", commit))
				break
			}
		}

		// Check if commit mentions task keywords
		hasTaskContext := false
		for _, keyword := range taskKeywords {
			if len(keyword) > 3 && strings.Contains(commitLower, keyword) {
				hasTaskContext = true
				break
			}
		}

		if !hasTaskContext && len(taskKeywords) > 0 {
			indicators = append(indicators, fmt.Sprintf("Commit may be off-topic: '%s'", commit))
		}
	}

	return indicators
}

// detectOverEngineering checks for signs of over-engineering
func (t *Tier2Watchdog) detectOverEngineering(worktreeDir string, files []string) bool {
	// Check for common over-engineering patterns
	patterns := []string{
		"factory",
		"abstract",
		"interface",
		"strategy",
		"decorator",
		"proxy",
	}

	// Count pattern occurrences in modified files
	patternCount := 0
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(worktreeDir, file))
		if err != nil {
			continue
		}

		contentStr := strings.ToLower(string(content))
		for _, pattern := range patterns {
			patternCount += strings.Count(contentStr, pattern)
		}
	}

	// If many design patterns introduced, might be over-engineering
	return patternCount > 10
}

// reportDrift sends a drift report to the Coordinator
func (t *Tier2Watchdog) reportDrift(ctx context.Context, agentID string, task db.Task, drift *DriftReport) {
	log.Printf("Tier2: DRIFT DETECTED for %s - Type: %s, Severity: %s",
		agentID, drift.DriftType, drift.Severity)

	// Record event
	eventDetails := fmt.Sprintf("Drift detected: %s (severity: %s)", drift.DriftType, drift.Severity)
	t.DB.RecordEvent(agentID, "drift_detected", eventDetails)

	// Build mail body
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Agent: %s\n", agentID))
	body.WriteString(fmt.Sprintf("Task: %s\n", task.Title))
	body.WriteString(fmt.Sprintf("Drift Type: %s\n", drift.DriftType))
	body.WriteString(fmt.Sprintf("Severity: %s\n\n", drift.Severity))

	body.WriteString("Evidence:\n")
	for _, evidence := range drift.Evidence {
		body.WriteString(fmt.Sprintf("- %s\n", evidence))
	}

	if len(drift.FilesModified) > 0 {
		body.WriteString(fmt.Sprintf("\nFiles Modified (%d total):\n", len(drift.FilesModified)))
		for _, f := range drift.FilesModified[:min(10, len(drift.FilesModified))] {
			body.WriteString(fmt.Sprintf("  - %s\n", f))
		}
		if len(drift.FilesModified) > 10 {
			body.WriteString(fmt.Sprintf("  ... and %d more\n", len(drift.FilesModified)-10))
		}
	}

	if len(drift.FilesOutsideScope) > 0 {
		body.WriteString(fmt.Sprintf("\nFiles Outside Scope (%d):\n", len(drift.FilesOutsideScope)))
		for _, f := range drift.FilesOutsideScope[:min(5, len(drift.FilesOutsideScope))] {
			body.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	body.WriteString(fmt.Sprintf("\nRecommendation: %s\n", drift.Recommendation))

	// Determine priority based on severity
	priority := db.PriorityNormal
	switch drift.Severity {
	case SeverityHigh, SeverityCritical:
		priority = db.PriorityHigh
	}

	// Send mail to Coordinator
	err := t.DB.SendMail("Monitor", "Coordinator", fmt.Sprintf("DRIFT DETECTED: %s", agentID),
		body.String(), db.MailTypeEscalation, priority)
	if err != nil {
		log.Printf("Tier2: Failed to send drift report: %v", err)
	} else {
		log.Printf("Tier2: Drift report sent to Coordinator for %s", agentID)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
