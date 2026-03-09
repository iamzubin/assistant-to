# MCP Tools - Merger

MCP server at 127.0.0.1:{{.MCPPort}} provides merge tools.

## Available Tools

**Mail:** `mail_send`, `mail_check`

**Worktrees:** `worktree_merge` (task_id), `worktree_teardown` (task_id)

**Debug:** `buffer_capture`

**Logging:** `log_event`

## Usage
- Merge with `worktree_merge`
- Report status via `mail_send`
- Teardown with `worktree_teardown` after success
- Log merge operations
