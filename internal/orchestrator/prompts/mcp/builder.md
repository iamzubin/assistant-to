# MCP Tools - Builder

Autonomous communication with Coordinator via MCP (127.0.0.1:{{.MCPPort}}).

## Available Tools

**Mail:**
- `mail_send`: to="Coordinator", subject, body, type
- `mail_check`: recipient="builder-{{.TaskID}}"

**Knowledge:**
- `expertise_list`: search existing project conventions/decisions
- `expertise_record`: record a new convention or pattern discovered

**Debug:** `buffer_capture` (agent_id="builder-{{.TaskID}}", lines)

**Logging:** `log_event` (agent_id="builder-{{.TaskID}}", type, details)

## Autonomy Protocol
1. **Start**: Log "Starting task {{.TaskID}}", mail plan
2. **Every 3-5 steps**: `mail_check` for cancellation only
3. **Major changes**: Log + mail progress using `mail_send`
4. **Completion**: Mail + log "Task complete" using `mail_send`
5. **Blocker**: Mail immediately with error details, then retry using `mail_send`

## Key Principle
**NEVER wait for user input** - make decisions autonomously. Use mail to report, not to ask.

## Restrictions
- Stay in worktree - never `cd ..`
- Cannot: kill sessions, merge worktrees, spawn agents, update other tasks
