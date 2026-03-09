# MCP Tools - Coordinator

MCP server at 127.0.0.1:{{.MCPPort}} provides swarm management tools.

## Available Tools

**Mail:** `mail_list`, `mail_check`, `mail_send`

**Tasks:** `task_list` (status), `task_update` (task_id, status), `task_add` (title, description, target_files, difficulty, parent_id)

**Agents:** `agent_spawn` (task_id, role)

**Audit:** `event_list` (agent_id, limit), `expertise_list` (domain, query), `expertise_record` (domain, type, description)

**Sessions:** `session_list`, `session_send`, `session_kill`, `session_clear`, `buffer_capture`

**Worktrees:** `worktree_merge`, `worktree_teardown`

**Utils:** `cleanup`, `log_event`

## Guidelines
- Check `task_list` WITHOUT a status filter every loop to see the full swarm state
- Check `mail_list` (without filter) to intercept and monitor ALL agent communication
- Use `buffer_capture` to "snoop" on agent terminals without connecting
- Use `session_send` to intervene or rescue stuck agents
- Use `cleanup` after task completion
- Spawn agents via `agent_spawn`
- Log significant swarm events with `log_event`
