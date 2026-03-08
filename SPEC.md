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
* **`tasks`**: `id`, `title`, `description`, `target_files`, `status` (`pending`, `started`, `scouted` (optional), `building`, `review`, `complete`).
* **`events`**: `id`, `agent_id`, `event_type` (tool_call, file_write, error), `details`, `timestamp`.

---

## 3. Agent Roster

| Role | Model Tier | Capabilities | Spawns | Notes |
| --- | --- | --- | --- | --- |
| **Coordinator** | `large` | `dispatch`, `escalate` | Yes | Reads `tasks`, spawns Builders, triggers Merger. |
| **Builder** | `medium` | `implement`, `fix` | No | Works strictly inside a `.assistant-to/worktrees/<id>` directory. |
| **Reviewer** | `medium` | `review`, `validate` | No | Reads a completed worktree to verify logic. |
| **Merger** | `medium` | `merge`, `resolve` | No | Triggered when session completes to fold worktrees back to `main`. |
| **Scout** | `fast` | `explore` | No | Greps global repo for context. |

---

## 4. Core Workflows

### A. Interactive Task Management

1. Run `at task add`.
2. A `huh` form prompts for task details.
3. The tool writes to the `tasks` DB and auto-generates `.assistant-to/specs/<task-id>.md`.

### B. Git Worktree Sandboxing & Merging

1. `Builder` spawned -> `git worktree add .assistant-to/worktrees/<task-id> -b at-<task-id>`.
2. `tmux` session is strictly locked to this isolated directory.
3. **Completion:** Merger folds code back to `main` and cleans up the worktrees.

### C. The Watchdog (Mail-Based Heartbeat)

The Coordinator monitors child sessions for stalls using the `mail` table as the primary health signal.

1. **Detection:** If an agent has not sent a `mail` or logged an `event` for > 5 minutes, it is flagged as **STALLED**.
2. **System Stimulus:** The Coordinator sends an `at mail` message to the child:
* **Subject:** `SYSTEM_RECOVERY_STIMULUS`
* **Body:** *"No activity detected for 5 minutes. RECOVERY PROTOCOL: 1. Send an immediate summary mail of your current blocker to the Coordinator. 2. Continue work or request escalation."*


3. **Session Injection:** The Coordinator may also use `tmux send-keys` to inject a `continue` hint directly into the agent's buffer.
4. **Hard Recovery:** If the agent fails to respond to the stimulus within 2 minutes, the Coordinator kills the `tmux` session, flags the task for retry, and spawns a fresh Builder with the previous logs as "context of failure."

---

## 5. The Director's Dashboard (`at dash`)

Running `at dash` launches a `bubbletea` TUI:

1. **Task Board (Left):** Live view of the `tasks` table.
2. **Agent Status (Top Right):** Lists active tmux sessions and the **"Last Mail"** heartbeat timer. Stalled agents appear in **Bold Red**.
3. **The Feed (Bottom Right):** Interleaved log of `events` and `mail`.
* `[Coordinator -> Builder-A]` `Mail: "STALL DETECTED. Status report required."`
* `[Builder-A -> Coordinator]` `Mail: "Summary: Investigating circular dependency in pkg/auth."`



---

## 6. CLI Command Summary

* `at init` - Initialize the workspace and DB.
* `at task add` - Interactive form to queue new work.
* `at start` - Wakes up the Coordinator to process the queue and monitor stalls.
* `at dash` - Opens the live TUI dashboard.
* `at mail` - (Agent/User) Send or read IPC messages.
* `at log` - (Agent/User) Record an event to the global timeline.
* `at connect` - Connect to an active agent's tmux session for manual observation.
* `at halt` - The panic button. Instantly kills all `assistant-to-*` tmux sessions.
