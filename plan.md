# Assistant-to Roadmap & Implementation Plan

This document defines the strategic roadmap for `dwight`, evolving it from a task runner into a sophisticated multi-agent orchestration swarm inspired by the Overstory ecosystem.

## 📊 Current Status
- **Foundation**: ✅ COMPLETED (Database, Basic CLI, Multi-instance support)
- **Watchdog System**: ✅ COMPLETED (3-Tier liveness and triage)
- **Merge Resolution**: ✅ COMPLETED (4-Tier mechanical to AI-assisted)
- **Dashboard & Logs**: 🔄 IN PROGRESS (Heartbeat fixes, Tool call logging)

---

## 🚀 Phase 1: Dashboard & Observation (Current)
*Focus: Real-time visibility into the swarm's activity.*

- [x] **Tool Call Logging**: Automatically record every MCP tool invocation in the `events` table.
- [x] **Heartbeat Synchronization**: Fix Dashboard "Last Seen" logic by using prefixed session names consistently.
- [x] **Task Context Mapping**: Display the associated task title for each active agent in the Dashboard.
- [ ] **Live Buffer Tailing**: Add a view to the TUI to tail the terminal of the active agent session.
- [ ] **Metric Aggregation**: Count tool usage (e.g., "Builder called `bash` 15 times") for efficiency analysis.

## 🏗️ Phase 2: Orchestration Hierarchy
*Focus: Managing complexity through specialized agent layers.*

- [ ] **Supervisor/Lead Role**: Implement a "Supervisor" agent that can manage its own sub-tasks and workers.
- [ ] **FIFO Merge Queue**: Transition from batch-merging to a continuous merge queue that integrates changes as soon as they pass review.
- [ ] **Sequential Dependency Locking**: Ensure a task cannot start if its `parent_id` or declared dependency is still in `building` or `merging`.

## 🧬 Phase 3: Blueprints & Expertise
*Focus: Structured context and environment configuration.*

- [ ] **Project-Level Blueprints**: Support `.dwight/blueprints/` directory for project-specific agent overlays.
- [ ] **Prompt Inheritance**: Implement Canopy-style inheritance in `PromptComposer` (e.g., `PythonBuilder` extends `BaseBuilder`).
- [ ] **Expertise Injection**: Automatically prime agents with relevant `expertise` records from the DB based on the files they are touching.

## 🛡️ Phase 4: Infrastructure-Level Safety
*Focus: Moving safety from "instructions" to "mechanical guards".*

- [ ] **Mechanical Role Guards**: Use shell aliases or environment hooks to physically block restricted operations (e.g., block `git push` for Scouts).
- [ ] **State-File Meta-Commits**: Automatically commit mission state and logs into a hidden branch before merging to ensure a clean working tree.
- [ ] **Automatic Rebase**: Automatically rebase active worktrees when `main` is updated by a successful merge queue operation.

---

## 📋 Ongoing Tasks

### 1. Refinement & Technical Debt
- [ ] **Exported Documentation**: Add Go Doc comments to all exported symbols.
- [ ] **Magic String Removal**: Move all remaining event and status strings to constants packages.
- [ ] **Error Wrapping**: Audit codebase for consistent `fmt.Errorf("...: %w", err)` usage.

### 2. Testing
- [ ] **Integration Test Suite**: Create E2E tests for the full task lifecycle.
- [ ] **Merge Conflict Simulations**: Automated tests for Tiers 2-4 of the merge resolver.
- [ ] **Watchdog Stall Tests**: Simulate agent hangs to verify Triage and Recovery.

---

## 🛠️ Implementation Guidelines

### 1. Agent Autonomy
- **NEVER** wait for user input in any agent loop.
- **ALWAYS** use non-interactive flags (`--yes`, `-y`, `--approval-mode=yolo`).
- **ESCALATE** via mail when a definitive blocker is reached, but only after internal retries.

### 2. Isolation & Safety
- Each agent MUST stay in its assigned worktree.
- Changes MUST be merged only after passing a `Review` phase.
- Credentials and secrets MUST never be logged in the `events` table.
