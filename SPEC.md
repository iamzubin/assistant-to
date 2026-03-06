# `assistant-to`

**The Managing Director's Autonomous Coding Swarm**

## 1. Philosophy & Architecture

`assistant-to` is a strictly bound, multi-agent orchestrator shipped as a single compiled Go binary. It prioritizes zero context bloat and total sandboxing. Agents communicate via a local SQLite mailbox, work strictly in isolated Git worktrees, and are supervised by an overarching timeout protocol.

### Tech Stack

* **Language:** Go (1.22+)
* **CLI / TUI:** `spf13/cobra` (routing), `charmbracelet/bubbletea` (dashboard), `charmbracelet/huh` (interactive forms), `charmbracelet/lipgloss` (styling).
* **State / IPC:** `modernc.org/sqlite` (Pure Go SQLite, CGO-free).
* **Isolation:** `tmux` (process sandboxing), `git worktree` (code sandboxing).

---

## 2. State & Directory Structure

Running `at init` scaffolds the project root:

```text
.assistant-to/
  ├── config.yaml          # Core settings (models, timeouts, API keys)
  ├── state.db             # Single SQLite file (Mail, Tasks, Events)
  ├── specs/               # Markdown task payloads
  └── worktrees/           # Isolated git worktrees for active agents

```

### Database Schema (`state.db`)

* **`mail`**: `id`, `sender`, `recipient`, `subject`, `body`, `is_read`, `timestamp`. (The IPC message bus).
* **`tasks`**: `id`, `title`, `description`, `target_files`, `status` (pending, active, review, complete).
* **`events`**: `id`, `agent_id`, `event_type` (tool_call, file_write, error), `details`, `timestamp`. (The heartbeat and audit log).

---

## 3. Agent Roster

| Role | Model Tier | Capabilities | Spawns | Notes |
| --- | --- | --- | --- | --- |
| **Coordinator** | `large` | `dispatch`, `escalate` | Yes | Reads `tasks`, spawns Leads, triggers Merger. |
| **Lead** | `large` | `plan`, `coordinate` | Yes | Manages Builders, monitors heartbeats. |
| **Builder** | `medium` | `implement`, `fix` | No | Works strictly inside a `.assistant-to/worktrees/<id>` directory. |
| **Reviewer** | `medium` | `review`, `validate` | No | Reads a completed worktree to verify logic. |
| **Merger** | `medium` | `merge`, `resolve` | No | Triggered when session completes to fold worktrees back to `main`. |
| **Scout** | `fast` | `explore` | No | Greps global repo for context. |

---

## 4. Core Workflows

### A. Interactive Task Management

Instead of manually typing files, you use the CLI.

1. Run `at task add`.
2. A `huh` form prompts for: **Task Title**, **Description**, **Difficulty** (determines if a single Builder or multiple Builders are needed), and **Target Files**.
3. The tool writes this to the `tasks` DB table and auto-generates `.assistant-to/specs/<task-id>.md`.

### B. Git Worktree Sandboxing & Merging

1. When a `Builder` is spawned, the orchestrator runs: `git worktree add .assistant-to/worktrees/<task-id> -b at-<task-id>`.
2. The Builder's `tmux` session is strictly locked to this isolated directory. It cannot break your main working tree.
3. **Session Completion:** Once all `tasks` in the DB hit `status: complete`, the Coordinator spawns the **Merger** agent. The Merger checks out your base branch, ingests the code from the `at-<task-id>` branches, resolves any git conflicts, and cleans up the worktrees.

### C. The Watchdog (Stuck Agent Recovery)

LLMs occasionally loop or hang. We enforce a strict 5-minute heartbeat.

1. Every time an agent uses a tool (e.g., reads a file, runs bash), it logs to the `events` table.
2. The **Lead** manager routinely queries `SELECT MAX(timestamp) FROM events WHERE agent_id = ?`.
3. **Timeout Protocol:** If an agent is idle for > 5 minutes, the Lead sends an `at mail` message: *"System alert: No activity detected for 5 minutes. Are you stuck? Please output your current blocker."*
4. If the agent fails to recover, the Lead updates the task state, kills the tmux session, and spawns a fresh Builder with the updated context.

---

## 5. The Director's Dashboard (`at dash`)

Running `at dash` launches a full-screen `bubbletea` terminal UI with three main panes:

1. **Task Board (Left):** A live view of the `tasks` table (Pending, Active, Complete).
2. **Agent Status (Top Right):** Lists active tmux sessions, current assigned task, and the "Last Seen" heartbeat timer.
3. **The Feed (Bottom Right):** An interleaved, real-time scrolling log merging the `events` and `mail` tables.
* *[Builder-A]* `Tool: bash "go test ./..."`
* *[Builder-A -> Lead]* `Mail: "Tests failing on nil pointer, investigating user.go"`
* *[Lead -> Builder-A]* `Mail: "Check the auth middleware for the missing context."`
* *[Builder-A]* `File Edit: auth/middleware.go`



---

## 6. CLI Command Summary

* `at init` - Interactive setup (via `huh`), generates `config.yaml` and empty DB.
* `at task add` - Interactive form to queue new work.
* `at start` - Wakes up the Coordinator to begin processing the task queue.
* `at dash` - Opens the live TUI dashboard.
* `at mail` - (Agent-only) CLI tool to send/fetch IPC messages.
* `at log` - (Agent-only) CLI tool to write to the `events` timeline.
* `at halt` - The panic button. Instantly kills all `assistant-to-*` tmux sessions.

