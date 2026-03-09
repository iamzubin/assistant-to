# MCP Tools - Coordinator

MCP server at 127.0.0.1:{{.MCPPort}} provides swarm management tools.

## Available Tools

**Mail:** `mail_list`, `mail_check`, `mail_send`

**Tasks:** `task_list` (status), `task_update` (task_id, status)

**Sessions:** `session_list`, `session_send`, `session_kill`, `session_clear`, `buffer_capture`

**Worktrees:** `worktree_merge`, `worktree_teardown`

**Utils:** `cleanup`, `log_event`

## Guidelines
- Check mail frequently for agent updates
- Use `cleanup` after task completion
- Agent stuck? → `buffer_capture` → `session_send` or escalate
- Log significant actions
- Spawn agents via available tooling (not CLI)
