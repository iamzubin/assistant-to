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
- **Expertise**: A shared knowledge base where agents record and search for project-specific conventions, patterns, and decisions.

### 3.2 The Coordinator
The system utilizes a dual-coordinator model:
1. **Passive Infrastructure (Go)**: A long-running process that provides the API/MCP servers, manages worktrees, and handles tmux session lifecycle. It does NOT make autonomous decisions.
2. **AI Coordinator (Gemini)**: An autonomous AI agent that manages the swarm via MCP tools. It queries tasks, spawns sub-agents, intercepts mail for quality control, and intervenes when agents are stuck.

### 3.3 Agent Roster
- **Scout**: Read-only agent that explores the codebase, identifies dependencies, and reports findings.
- **Builder**: The primary implementation agent. It performs the actual coding tasks within its worktree.
- **Merger**: Specialist agent responsible for merging completed task branches into the main branch, resolving conflicts using a multi-tier strategy.
- **Watchdog (Tier 1 & 2)**: Monitoring goroutines that track agent activity, detect hangs, and handle timeouts.

### 3.4 Communication Protocol
Agents interact with the system via:
- **Mailbox API**: Agents send/receive "Mail" objects (Dispatch, Status, Question, Result, Error).
- **MCP (Model Context Protocol)**: Provides a standardized interface for agents to use local tools (shell, git, file system). The system automatically generates project-specific MCP configurations for tools like `gemini` CLI and `opencode`.
- **REST API**: Internal server providing status updates and configuration. Ports are deterministically calculated based on the project path to prevent conflicts between multiple instances.

### 3.5 Code Intelligence (Mulch)
`assistant-to` includes a dedicated code intelligence engine called "Mulch":
- **Static Analysis**: Parses Go source code using `go/parser` to build a symbol index.
- **Dependency Graph**: Maps relationships between files, packages, and types (structs, interfaces).
- **Impact Analysis**: Helps agents understand the ripple effects of proposed changes by tracing function calls and type usages.
- **Persistence**: Symbols and dependencies are stored in a SQLite database for fast querying.

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
