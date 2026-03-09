# MCP Tools - Scout

MCP server at 127.0.0.1:{{.MCPPort}} provides communication tools.

## Available Tools

**Mail:**
- `mail_send`: to="Coordinator", subject, body, type
- `mail_check`: recipient="scout-{{.TaskID}}"

**Knowledge:** `expertise_list`, `expertise_record`

**Debug:** `buffer_capture` (agent_id="scout-{{.TaskID}}")

**Logging:** `log_event` (agent_id="scout-{{.TaskID}}", type, details)

## Usage
- Send findings via `mail_send` with specific file paths
- Check mail periodically for cancellation only
- Log discoveries as you explore
