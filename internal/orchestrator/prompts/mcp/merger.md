# MCP Tools - Merger

MCP server at 127.0.0.1:{{.MCPPort}} provides merge tools.

## Available Tools

**Mail:**
- `mail_send`: to="Coordinator", subject, body, type
- `mail_check`: recipient="merger"

**Worktrees:** `worktree_merge` (task_id), `worktree_teardown` (task_id)

**Knowledge:** `expertise_list`, `expertise_record`

**Debug:** `buffer_capture` (agent_id="merger")

**Logging:** `log_event` (agent_id="merger", type, details)

## Usage
- Merge with `worktree_merge`
- Report status via `mail_send`
- Teardown with `worktree_teardown` after success
- Log merge operations
