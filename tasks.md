# assistant-to Session Tasks

This checklist breaks down the remaining work from `SPEC.md` into actionable items for a single development session.

## Stage 1: State Initialization & CLI Commands
- [x] Implement `internal/config/config.go` (Loading `config.yaml` settings: models, API keys, etc.)
- [x] Implement `at init` command:
  - [x] Scaffold `.assistant-to` directory structure.
  - [x] Generate default `config.yaml`.
  - [x] Initialize `state.db` using the existing `db.go` init code.
- [x] Implement `at halt` command:
  - [x] Retrieve all active tmux sessions and kill them instantly with `tmux kill-server` or iterating `tmux kill-session`.

## Stage 2: Database Wrappers
- [x] **Mail:** Write Go models and CRUD operations for the `mail` table (Send, Fetch Unread).
- [x] **Tasks:** Write Go models and CRUD operations for the `tasks` table (Add, UpdateStatus, List).
- [x] **Events:** Write Go models and DB operations for the `events` table (log `tool_call`, `file_write`, fetch log history).

## Stage 3: Interactive Task & Dashboard TUIs
- [x] Implement `at task add` command:
  - [x] Build interactive form using `charmbracelet/huh` prompting for **Title**, **Description**, and **Difficulty**.
  - [x] Auto-generate markdown payload to `.assistant-to/specs/<task-id>.md`.
  - [x] Insert the task record into the `tasks` table with status 'pending'.
- [x] Scaffold `at dash` command.
- [x] Design the BubbleTea layout:
  - [x] Task Board (Left Pane): Fetch from DB (uses `bubbles/list`).
  - [x] Agent Status (Top Right): List active tmux sessions + Last Heartbeat (uses `bubbles/list`).
  - [x] The Feed (Bottom Right): Rolling interactive log of `events` and `mail` (uses `bubbles/list`).
  - [x] Inline Task Creation: Embed the `huh` form directly over the dashboard via hotkey `n`.

## Stage 4: Agent Sandboxing (`tmux` & `git worktree`)
- [ ] Implement Git Worktree spawning in `internal/sandbox/git.go`:
  - [ ] `git worktree add .assistant-to/worktrees/<task-id> -b at-<task-id>`
  - [ ] Integration: Mount the previously created Tmux session exclusively into this directory.
- [ ] Worktree Teardown & Merge:
  - [ ] Build logic for the **Merger** to checkout the base branch and merge the feature branch cleanly.

## Stage 5: The Agent Orchestrator Main Loop
- [ ] Implement `at start` command (wakes up Coordinator):
  - [ ] Read pending tasks from DB.
  - [ ] Spawn Lead Agent, which spawns Builder agents natively.
  - [ ] Integrate Watchdog (already created) into the Lead agent loop.
