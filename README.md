# dwight

**The Managing Director's Autonomous Coding Swarm**

`dwight` is a multi-agent orchestrator built in Go. It allows you to manage tasks using a swarm of autonomous agents that communicate via a local SQLite mailbox, work in isolated Git worktrees, and are supervised by a coordinator.

## 📋 Prerequisites

Before installing, ensure you have the following available:
- **Go**: 1.25+
- **tmux**: Used for process sandboxing.
- **git**: Required for repository management and worktree isolation.

## 📦 Installation

To install `dwight` globally:

```bash
go install ./cmd/dwight
```

Make sure your `$GOPATH/bin` is in your `PATH`. You can then use the `dwight` command from any directory.

Alternatively, to build locally:
```bash
go build -o dwight ./cmd/dwight
```

## 🚀 Quick Start

### 1. Initialize a project
Run this in the root of the git repository you want to manage.

```bash
dwight init
```
This will create a `.dwight` directory with your configuration and state database.

### 2. Add a task
```bash
dwight task add
```
Follow the interactive form to define the task.

### 3. Start the swarm
```bash
dwight start
```
This wakes up the Coordinator to begin processing the task queue.

### 4. Monitor progress
```bash
dwight dash
```
Opens the TUI dashboard to see your agents in action.

---

## 🛠 Development

### Project Structure
- `cmd/dwight/`: Entry point for the CLI.
- `internal/`: Core logic, including orchestrator, prompts, and database management.
- `SPEC.md`: Detailed architectural specification.

### Building
```bash
go build -o dwight ./cmd/dwight
```

### Testing
```bash
go test ./...
```

---

## 📖 CLI Commands

- `dwight init` - Interactive setup (via `huh`).
- `dwight task add` - Interactive form to queue new work.
- `dwight start` - Starts the Coordinator.
- `dwight dash` - Opens the live TUI dashboard.
- `dwight halt` - Instantly kills all `dwight-*` tmux sessions.

---

## 🏛 Architecture
Refer to [SPEC.md](SPEC.md) for a deep dive into the philosophy, state management, and agent roster.
