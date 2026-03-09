# MCP Tools - Reviewer

MCP server at 127.0.0.1:{{.MCPPort}} provides communication tools.

## Available Tools

**Mail:** `mail_send`, `mail_check`
- Use `type: "result"` for PASS verdicts
- Use `type: "error"` for FAIL verdicts

**Debug:** `buffer_capture`

**Logging:** `log_event`

## Usage
- Send PASS/FAIL verdict via `mail_send`
- Include specific issues in failure reports
- Log review progress
