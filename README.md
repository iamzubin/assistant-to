# assistant-to

**The Managing Director's Autonomous Coding Swarm**

`assistant-to` is a multi-agent orchestrator built in Go. It allows you to manage tasks using a swarm of autonomous agents that communicate via a local SQLite mailbox, work in isolated Git worktrees, and are supervised by a coordinator.

## 📋 Prerequisites

Before installing, ensure you have the following available:
- **Go**: 1.25+
- **tmux**: Used for process sandboxing.
- **git**: Required for repository management and worktree isolation.

## 📦 Installation

To install `assistant-to` globally:

```bash
go install ./cmd/at
```

Make sure your `$GOPATH/bin` is in your `PATH`. You can then use the `at` command from any directory.

Alternatively, to build locally:
```bash
go build -o at ./cmd/at
```

## 🚀 Quick Start

### 1. Initialize a project
Run this in the root of the git repository you want to manage.

```bash
at init
```
This will create a `.assistant-to` directory with your configuration and state database.

### 2. Add a task
```bash
at task add
```
Follow the interactive form to define the task.

### 3. Start the swarm
```bash
at start
```
This wakes up the Coordinator to begin processing the task queue.

### 4. Monitor progress
```bash
at dash
```
Opens the TUI dashboard to see your agents in action.

---

## 🛠 Development

### Project Structure
- `cmd/at/`: Entry point for the CLI.
- `internal/`: Core logic, including orchestrator, prompts, and database management.
- `SPEC.md`: Detailed architectural specification.

### Building
```bash
go build -o at ./cmd/at
```

### Testing
```bash
go test ./...
```

---

## 📖 CLI Commands

- `at init` - Interactive setup (via `huh`).
- `at task add` - Interactive form to queue new work.
- `at start` - Starts the Coordinator.
- `at dash` - Opens the live TUI dashboard.
- `at halt` - Instantly kills all `assistant-to-*` tmux sessions.

---

## 🏛 Architecture
Refer to [SPEC.md](SPEC.md) for a deep dive into the philosophy, state management, and agent roster.
