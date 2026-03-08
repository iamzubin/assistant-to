# Assistant-to Implementation Plan

This document tracks the implementation status of features defined in SPEC.md, organized by priority.

## 📊 Current Status

**Phase 1: Foundation** - ✅ COMPLETED (6/6 items)
**Phase 2: Watchdog System** - ✅ COMPLETED (4/4 items)
**Phase 3: Merge Resolution** - 🔄 READY TO START
**Phase 4: Intelligence & Polish** - ⏳ PENDING

---

---

## 🔴 Critical - Database Schema Updates (Required for New Spec)

### 1. Mail Table Schema Expansion ✅ COMPLETED
**Files:** `internal/db/db.go:37-45`, `internal/db/mail.go`
**Status:** COMPLETED
**Implementation:**
- ✅ Added columns: `type TEXT`, `priority INTEGER DEFAULT 5`
- ✅ Created constants for 8 message types: dispatch, worker_done, merge_ready, escalation, status, question, result, error
- ✅ Updated Mail struct and all mail-related functions
- ✅ Added indexes for performance

### 2. Tasks Table Enhancement ✅ COMPLETED
**Files:** `internal/db/db.go:47-53`, `internal/db/tasks.go`
**Status:** COMPLETED
**Implementation:**
- ✅ Added column: `priority INTEGER DEFAULT 3`
- ✅ Defined status constants: pending, started, scouted, building, review, merging, complete, failed
- ✅ Added status validation to UpdateTaskStatus()
- ✅ Added helper functions: AddTaskWithPriority, UpdateTaskPriority, ListTasksByPriority

### 3. Create Expertise Table ✅ COMPLETED
**Files:** `internal/db/db.go` (schema), `internal/db/expertise.go` (new file)
**Status:** COMPLETED
**Implementation:**
- ✅ Created table: `id, domain, type, description, timestamp`
- ✅ Defined types: convention, pattern, failure, decision
- ✅ Implemented CRUD operations with filtering and search
- ✅ Added validation and type helpers

---

## 🔴 Critical - New CLI Commands

### 4. Implement `at prime` Command ✅ COMPLETED
**Files:** `internal/cli/prime.go` (new)
**Status:** COMPLETED
**Features:**
- ✅ Query expertise table for patterns/failures
- ✅ Output formatted expertise to stdout (human-readable format)
- ✅ Domain filtering (`--domain=db`)
- ✅ Type filtering (`--type=failure`)
- ✅ Recent filter (`--recent=7`)
- ✅ JSON output (`--json`)

### 5. Implement `at record` Command ✅ COMPLETED
**Files:** `internal/cli/record.go` (new)
**Status:** COMPLETED
**Features:**
- ✅ Interactive form using `huh` for domain, type, description
- ✅ CLI flag support for non-interactive use (`--domain`, `--type`, `--description`)
- ✅ Validate type is one of: convention, pattern, failure, decision
- ✅ Insert into expertise table with timestamp

### 6. Implement `at mail check --inject` Command ✅ COMPLETED
**Files:** `internal/cli/mail.go` (enhanced)
**Status:** COMPLETED
**Features:**
- ✅ `at mail check` subcommand
- ✅ `--inject` flag for agent tool consumption
- ✅ Format unread mail for shell/tool consumption (structured text)
- ✅ Mark messages as read after injection
- ✅ Support for mail type and priority in send command
- ✅ `AT_AGENT_ROLE` environment variable support

---

## 🟠 High Priority - Watchdog System (3-Tier)

### 7. Tier 0 Watchdog (Mechanical) ✅ COMPLETED
**Files:** `internal/orchestrator/watchdog.go` (enhanced), `internal/sandbox/sandbox.go`
**Status:** COMPLETED
**Enhancements:**
- ✅ PID liveness check via `GetPID()` and `IsProcessAlive()`
- ✅ Tmux connectivity check via `Ping()`
- ✅ Enhanced session validation before heartbeat checks
- ✅ Event logging for all Tier 0 checks

### 8. Tier 1 Watchdog (AI Triage) ✅ COMPLETED
**Files:** `internal/orchestrator/watchdog_tier1.go` (new)
**Status:** COMPLETED
**Features:**
- ✅ Transcript capture (last 1,000 lines via `CaptureBufferLines()`)
- ✅ Pattern-based failure detection (infinite loops, input required, build errors, etc.)
- ✅ Intelligent stimulus based on failure mode:
  - Input required: Send keystrokes
  - Infinite loop: Send Ctrl+C
  - Build/test failures: Send guidance mail
  - Resource/network issues: Escalate
- ✅ Integration with main watchdog on first recovery attempt

### 9. Tier 2 Watchdog (Monitor Agent) ✅ COMPLETED
**Files:** `internal/orchestrator/watchdog_tier2.go` (new), `internal/orchestrator/coordinator.go`
**Status:** COMPLETED
**Features:**
- ✅ Periodic drift detection every 5 minutes
- ✅ Monitors all active tasks (building, scouted, started)
- ✅ Drift detection algorithms:
  - Scope compliance checking
  - Commit message analysis
  - Over-engineering detection
  - Yak shaving identification
- ✅ Automatic drift reporting to Coordinator via mail
- ✅ Integrated into Coordinator lifecycle

---

## 🟠 High Priority - Merge Resolution Strategy (4-Tier)

### 10. Tier 1: Mechanical Merge - Exists
**Files:** `internal/sandbox/git.go` (exists)
**Current:** Basic merge support
**Status:** Adequate

### 11. Tier 2: Algorithmic Synthesis for Structured Files - NEW
**Files:** `internal/merge/synthesis.go` (new)
**Required by Spec:** Union-merge for JSONL/YAML files
**Implementation:**
- Detect file type (JSONL, YAML)
- Implement union-merge algorithm for concurrent metadata updates
- Called before standard merge to resolve non-conflicting changes

### 12. Tier 3: Contextual Rebase - NEW
**Files:** `internal/merge/rebase.go` (new)
**Required by Spec:** Rebase feature branch onto latest main
**Implementation:**
- Detect stale commits causing conflicts
- Attempt automatic rebase
- Handle rebase conflicts gracefully

### 13. Tier 4: AI-Assisted Resolution - NEW
**Files:** `internal/merge/ai_resolve.go` (new), `internal/orchestrator/coordinator.go` (spawn)
**Required by Spec:** Spawn Merger agent with full diff context
**Implementation:**
- Collect full diff of conflicting files
- Spawn Merger agent in isolated worktree
- Inject diff context into prompt
- Apply resolved changes back

### 14. Integrate 4-Tier Merge into Coordinator
**Files:** `internal/orchestrator/coordinator.go`
**Current:** Spawns Builders but no merge logic
**Required by Spec:** Trigger merge resolution when task completes
**Implementation:**
- When Builder completes, attempt Tier 1 merge
- Progress through Tiers 2-4 as needed
- Update task status to "merging" during resolution

---

## 🟡 Medium Priority - Advanced Prompt Management

### 15. Prompt Composition System
**Files:** `internal/orchestrator/prompts.go` (enhance), `.assistant-to/prompts/` (directory)
**Current:** Simple prompt loading from files
**Required by Spec:** Composition/Inheritance with overlays
**Implementation:**
- Support base prompt + overlay composition
- Define AT_INSTRUCTIONS.md format
- Render final prompt at worktree injection time
- Support variable substitution (task ID, files, etc.)

### 16. Monitor Agent Prompt
**Files:** `internal/orchestrator/prompts/monitor.md` (new)
**Required by Spec:** Monitor agent needs dedicated prompt
**Implementation:**
- Create prompt defining objective drift detection
- Include examples of drift vs. valid adaptations

---

## 🟡 Medium Priority - Local Intelligence Layer (MCP)

### 17. Code Intelligence Engine - MVP
**Files:** `internal/intelligence/` (new package)
**Required by Spec:** Local code indexing similar to KotaDB
**Implementation:**
- Create separate SQLite DB for code index
- Parse Go files for dependencies
- Expose search_dependencies tool
- Index on `at init` or file changes

### 18. Impact Analysis Tool
**Files:** `internal/intelligence/impact.go` (new)
**Required by Spec:** analyze_change_impact tool
**Implementation:**
- Given a file path, find all dependent files
- Generate impact map for Builder
- Inject into Builder's mission context

---

## 🟢 Low Priority - Code Quality & Polish

### 19. Magic Strings to Constants
**Files:** Throughout codebase
**Issue:** Status values, table names, event types hardcoded
**Fix:** Create constants packages:
- `internal/constants/status.go` for task statuses
- `internal/constants/events.go` for event types
- `internal/constants/mail.go` for mail types

### 20. Enhanced Error Handling
**Files:** Throughout codebase
**Issue:** Some errors logged but not wrapped with context
**Fix:** Audit and improve error wrapping using `fmt.Errorf("...: %w", err)`

### 21. Go Doc Comments
**Files:** Throughout codebase
**Issue:** Many exported functions lack proper doc comments
**Fix:** Add `// FunctionName ...` comments for all exported functions

### 22. Integration Tests
**Files:** `tests/integration/` (new)
**Issue:** No end-to-end workflow tests
**Fix:** Add tests for:
- Full task lifecycle
- Watchdog stall detection
- Merge resolution flow

---

## 📋 Implementation Checklist

### Phase 1: Foundation (Database & Basic CLI) ✅ COMPLETED
- [x] 1. Expand mail table schema (type, priority columns)
- [x] 2. Enhance tasks table (priority, status validation)
- [x] 3. Create expertise table and operations
- [x] 4. Implement `at prime` command
- [x] 5. Implement `at record` command
- [x] 6. Enhance `at mail check --inject`

### Phase 2: Watchdog & Monitoring ✅ COMPLETED
- [x] 7. Enhance Tier 0 Watchdog (PID/tmux checks)
- [x] 8. Implement Tier 1 Watchdog (AI Triage)
- [x] 9. Implement Tier 2 Watchdog (Monitor Agent)
- [x] 10. Create Monitor agent prompt

### Phase 3: Merge Resolution ✅ COMPLETED
- [x] 11. Implement Tier 2 Algorithmic Synthesis
- [x] 12. Implement Tier 3 Contextual Rebase
- [x] 13. Implement Tier 4 AI-Assisted Resolution
- [x] 14. Integrate 4-tier merge into Coordinator workflow

### Phase 4: Intelligence & Polish
- [ ] 15. Implement prompt composition system
- [ ] 16. Create code intelligence engine (MVP)
- [ ] 17. Implement impact analysis
- [ ] 18. Extract magic strings to constants
- [ ] 19. Add comprehensive tests
- [ ] 20. Documentation and polish

---

## 🎯 Recommended Implementation Order

1. **Start with database schema updates** (Items 1-3) - Required foundation
2. **Add new CLI commands** (Items 4-6) - Enables core workflows
3. **Implement Tier 1 Watchdog** (Item 8) - Critical for stability
4. **Build Monitor Agent** (Items 9, 10, 16) - Prevents objective drift
5. **Implement merge resolution** (Items 11-14) - Enables autonomous completion
6. **Add intelligence layer** (Items 17-18) - Prevents globally-broken changes
7. **Polish and test** (Items 19-22) - Production readiness

---

## Testing Strategy

- Run `go test ./...` after each package change
- Run `go build ./cmd/at` to ensure compilation
- Test CLI commands manually:
  ```bash
  at init
  at task add
  at prime
  at record
  at start
  at dash
  at mail check --inject
  at halt
  ```

---

## Notes

- Each major feature should have its own commit
- Database migrations should be handled automatically in `InitSchema()`
- Maintain backward compatibility where possible
- Document breaking changes in commit messages
