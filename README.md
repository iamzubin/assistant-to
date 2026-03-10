# Dwight

<p align="center">
  <strong>The Managing Director's Autonomous Coding Swarm</strong><br>
  A multi-agent orchestrator for autonomous software development
</p>

<p align="center">
  <a href="https://github.com/iamzubin/dwight/releases">
    <img src="https://img.shields.io/github/v/release/iamzubin/dwight?include_prereleases&style=flat-square" alt="Release">
  </a>
  <a href="https://pkg.go.dev/dwight">
    <img src="https://img.shields.io/badge/Go-Reference-00ADD8?style=flat-square&logo=go" alt="Go Reference">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/github/license/iamzubin/dwight?style=flat-square" alt="License">
  </a>
</p>

---

Dwight is a multi-agent orchestrator that manages a swarm of autonomous AI coding agents. It provides complete task isolation using Git worktrees, coordinated communication via a SQLite mailbox, and intelligent supervision through tiered watchdogs.

## Why Dwight?

Traditional AI coding assistants work in isolation. Dwight brings a whole team to your codebase:

- **Parallel Development**: Multiple agents work simultaneously on different tasks
- **Complete Isolation**: Each task runs in its own Git worktree—no merge conflicts, no side effects
- **Self-Healing**: Watchdogs detect stuck agents and automatically intervene
- **Knowledge Sharing**: Agents learn from each other via a shared expertise database

## Features

### 🤖 Specialized Agent Roles
- **Scout**: Explores the codebase, identifies dependencies, maps relationships
- **Builder**: Implements features and fixes in isolated worktrees
- **Merger**: Handles branch integration with a 4-tier conflict resolution strategy
- **Watchdog**: Monitors health, detects hangs, manages timeouts

### 🔄 Intelligent Coordination
- **Dual-Coordinator Model**: Infrastructure coordinator + AI supervisor
- **Mailbox Protocol**: Async agent communication via typed messages
- **MCP Integration**: Native Model Context Protocol support for external AI tools

### 📊 Code Intelligence
- **Static Analysis**: Symbol indexing using Go's parser
- **Dependency Graph**: Maps files, packages, and type relationships
- **Impact Analysis**: Traces ripple effects of proposed changes

### 🛡️ Safe Execution
- **Git Worktrees**: Filesystem isolation per task
- **Tmux Sandboxing**: Process isolation with live monitoring
- **Deterministic Ports**: Conflict-free multi-project support

## Installation

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go   | 1.24+   | Runtime  |
| tmux | Latest  | Process sandboxing |
| git  | 2.30+   | Worktree management |

### Quick Install

```bash
go install github.com/iamzubin/dwight/cmd/dwight@latest
```

Or build from source:

```bash
git clone https://github.com/iamzubin/dwight.git
cd dwight
go build -o dwight ./cmd/dwight
```

Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is in your `PATH`.

## Quick Start

### 1. Initialize a project

```bash
cd your-project
dwight init
```

Creates `.dwight/` with configuration and SQLite state database.

### 2. Add a task

```bash
dwight task add
```

Interactive form to define task title, description, and difficulty.

### 3. Start the swarm

```bash
dwight start
```

Wakes up the Coordinator to begin processing the task queue.

### 4. Monitor progress

```bash
dwight dash
```

Opens the live TUI dashboard to watch agents in action.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Coordinator                              │
│  ┌─────────────────┐              ┌─────────────────────────┐  │
│  │  Infrastructure │              │    AI Supervisor        │  │
│  │  (Go Server)    │◄─────────────►│    (Gemini via MCP)     │  │
│  └────────┬────────┘              └───────────┬─────────────┘  │
│           │                                      │                │
│           │         ┌──────────────────┐         │                │
│           └────────►│   SQLite State   │◄────────┘                │
│                    │  ┌──────────────┐  │                          │
│                    │  │   Tasks DB   │  │                          │
│                    │  │   Mailbox    │  │                          │
│                    │  │   Events     │  │                          │
│                    │  │   Expertise  │  │                          │
│                    │  └──────────────┘  │                          │
│                    └─────────────────────┘                          │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│     Scout     │    │    Builder    │    │    Merger     │
│  (Read-only) │    │  (Implement)   │    │   (Merge)     │
│              │    │                │    │               │
│  - Explore   │    │  - Code        │    │  - Resolve    │
│  - Analyze   │    │  - Test        │    │  - Rebase     │
│  - Report    │    │  - Verify      │    │  - Integrate  │
└──────┬───────┘    └───────┬────────┘    └───────┬────────┘
       │                    │                    │
       │         ┌──────────┴──────────┐         │
       │         │   Git Worktree      │         │
       │         │   (Isolated Branch) │         │
       │         └─────────────────────┘         │
       │                   │                      │
       └───────────────────┼──────────────────────┘
                           ▼
                   ┌───────────────┐
                   │     tmux      │
                   │   Session     │
                   │ (Sandbox)     │
                   └───────────────┘
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `dwight init` | Interactive project setup |
| `dwight task add` | Queue a new task |
| `dwight start` | Start the Coordinator |
| `dwight dash` | Open live TUI dashboard |
| `dwight halt` | Kill all dwight-* tmux sessions |
| `dwight task list` | List all tasks |
| `dwight mail send` | Send a message to an agent |

## Project Structure

```
dwight/
├── cmd/dwight/           # CLI entry point
├── internal/
│   ├── api/              # MCP and HTTP servers
│   ├── cli/              # Cobra commands
│   ├── config/           # Configuration management
│   ├── constants/        # Status codes, events
│   ├── db/               # SQLite schema and queries
│   ├── intelligence/    # Code analysis (Mulch)
│   ├── merge/            # Conflict resolution
│   ├── metrics/          # Token tracking
│   ├── orchestrator/     # Coordinator and agents
│   ├── sandbox/          # Worktree and tmux management
│   ├── tasking/          # Prompt generation
│   └── tui/              # Dashboard UI
├── SPEC.md               # Detailed architecture spec
└── README.md             # This file
```

## Configuration

Dwight stores state in `.dwight/`:

```
.dwight/
├── config.yaml       # Project configuration
├── state.db          # SQLite database
└── logs/             # Agent execution logs
```

## Development

```bash
# Build
go build -o dwight ./cmd/dwight

# Test
go test ./...

# Run
./dwight --help
```

## Documentation

- [SPEC.md](SPEC.md) — Detailed architecture and design decisions
- [plan.md](plan.md) — Development roadmap

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  Built with Go, tmux, and SQLite
</p>
