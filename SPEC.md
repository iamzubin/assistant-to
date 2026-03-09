# SPEC.md - Assistant-To Specification

## 1. Overview
`assistant-to` (CLI: `dwight`) is a multi-agent orchestrator designed for autonomous software development. It enables a "swarm" of AI agents to work on a codebase safely and concurrently by utilizing Git worktrees and tmux for isolation.

## 2. Core Philosophy
- **Isolation**: Every task is executed in a dedicated Git worktree and a sandboxed tmux session.
- **Asynchronicity**: Agents communicate via a central mailbox (SQLite), allowing for non-blocking coordination.
- **Supervision**: A central Coordinator manages the lifecycle of tasks and agents, with multi-tier Watchdogs monitoring health and performance.

## 3. System Architecture

### 3.1 State Management (SQLite)
The system state is persisted in `.assistant-to/state.db`:
- **Tasks**: Lifecycle management (Pending -> Scouting -> Building -> Merging -> Complete).
- **Mailbox**: A typed message queue for agent-to-agent and agent-to-system communication.
- **Events**: Audit log of all system and agent actions.

### 3.2 The Coordinator
The `Coordinator` is the central engine of the system. It:
1. Polls the database for new tasks.
2. Initializes Git worktrees for task isolation.
3. Spawns appropriate agents (Scout, Builder) in tmux sessions.
4. Starts the API and MCP servers for agent communication.
5. Monitors agent completion and initiates the merge process.

### 3.3 Agent Roster
- **Scout**: Read-only agent that explores the codebase, identifies dependencies, and reports findings.
- **Builder**: The primary implementation agent. It performs the actual coding tasks within its worktree.
- **Merger**: Specialist agent responsible for merging completed task branches into the main branch, resolving conflicts using a multi-tier strategy.
- **Watchdog (Tier 1 & 2)**: Monitoring goroutines that track agent activity, detect hangs, and handle timeouts.

### 3.4 Communication Protocol
Agents interact with the system via:
- **Mailbox API**: Agents send/receive "Mail" objects (Dispatch, Status, Question, Result, Error).
- **MCP (Model Context Protocol)**: Provides a standardized interface for agents to use local tools (shell, git, file system).
- **REST API**: Internal server providing status updates and configuration.

## 4. Execution Sandbox
- **Git Worktrees**: Prevents concurrent tasks from interfering with each other's file system state.
- **Tmux Sessions**: Provides process isolation and allows the user to "attach" to any running agent to monitor logs and terminal output.

## 5. Development & CLI
- **Language**: Go 1.25+
- **CLI Framework**: `spf13/cobra`
- **UI Framework**: `charmbracelet/bubbletea` (TUI Dashboard), `huh` (Interactive Forms).
- **Merge Strategy**: 4-Tier resolution (Clean -> Simple -> Complex -> AI-Assisted).

## 6. Project Structure
- `cmd/dwight/`: CLI Entry point.
- `internal/orchestrator/`: Coordinator logic and agent prompts.
- `internal/db/`: SQLite schema and data access layer.
- `internal/sandbox/`: Worktree and tmux management.
- `internal/api/`: MCP and HTTP server implementations.
- `internal/tui/`: Dashboard and terminal UI components.
