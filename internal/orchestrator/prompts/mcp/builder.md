# MCP Tools - Builder

Autonomous communication with Coordinator via MCP (127.0.0.1:{{.MCPPort}}).

## Available Tools

**Mail:**
- `mail_send`: to="Coordinator", subject, body, type
- `mail_check`: recipient="builder-{{.TaskID}}"

**Debug:** `buffer_capture` (agent_id="builder-{{.TaskID}}", lines)

**Logging:** `log_event` (type, details)

## Autonomy Protocol
1. **Start**: Log "Starting task {{.TaskID}}", mail plan
2. **Every 3-5 steps**: `mail_check` for cancellation only
3. **Major changes**: Log + mail progress
4. **Completion**: Mail + log "Task complete"
5. **Blocker**: Mail immediately with error details, then retry

## Key Principle
**NEVER wait for user input** - make decisions autonomously. Use mail to report, not to ask.

## Restrictions
- Stay in worktree - never `cd ..`
- Cannot: kill sessions, merge worktrees, spawn agents, update other tasks
