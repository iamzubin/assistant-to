# Reviving Stalled Agents with tmux send

When an agent becomes stalled (e.g., waiting for input or stuck), you can use the `session_send` tool to inject commands into its tmux terminal and get it moving again.

## Using session_send

The `session_send` tool requires:
- `agent_id`: The ID of the stalled agent (e.g., "builder-6")
- `input`: The command to inject

Example:
```
session_send(agent_id="builder-6", input="ls\n")
```

This sends `ls` followed by Enter to the agent's terminal.

## Common Use Cases

### 1. Send Ctrl+C to interrupt a stuck command
```
session_send(agent_id="builder-6", input="\x03")
```

### 2. Send a new command to continue work
```
session_send(agent_id="builder-6", input="git status\n")
```

### 3. Force exit a blocking process
```
session_send(agent_id="builder-6", input="exit\n")
```

## Finding the Agent Session

List active sessions to find the agent's tmux session name:
```
tmux list-sessions
```

Session names follow the format: `at-{worktree_id}-{role}-{number}`

## Checking Buffer

To see what's happening in the stalled terminal:
```
buffer_capture(agent_id="builder-6", lines=50)
```

This captures the last 50 lines of the agent's tmux buffer.

## Demo: Stall an Agent

To create a stalled agent for testing:
```
tmux new-session -d -s stall-test "sleep 3000"
```

This creates a detached session running `sleep 3000` - effectively "stalling" it. You can then use `session_send` to inject commands into it.

## Stalling an Agent via MCP/tmux

To stall an agent programmatically (without manually creating a session):

1. Run a blocking command like `sleep 60` to keep it occupied
2. Use tmux to send escape twice:
   ```
   tmux send-keys -t at-assistant-to-{worktree_id}-builder-{n} $'\e' ; sleep 1 ; tmux send-keys -t at-assistant-to-{worktree_id}-builder-{n} $'\e'
   ```
3. Send enter to confirm interrupt:
   ```
   tmux send-keys -t at-assistant-to-{worktree_id}-builder-{n} Enter
   ```
4. Then revive it using session_send MCP tool:
   ```
   session_send(agent_id="builder-6", input="your command")
   ```
