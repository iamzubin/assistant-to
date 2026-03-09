# MCP Tools - Reviewer

MCP server at 127.0.0.1:{{.MCPPort}} provides communication tools.

## Available Tools

**Mail:**
- `mail_send`: to="Coordinator", subject, body, type
- `mail_check`: recipient="reviewer-{{.TaskID}}"

**Knowledge:** `expertise_list`, `expertise_record`

**Debug:** `buffer_capture` (agent_id="reviewer-{{.TaskID}}")

**Logging:** `log_event` (agent_id="reviewer-{{.TaskID}}", type, details)

## Usage
- Send PASS/FAIL verdict via `mail_send`
- Include specific issues in failure reports
- Log review progress
