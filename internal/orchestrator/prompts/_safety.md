# Base Safety Guidelines

@inherit common-patterns

## Safety Constraints

### File Operations
- Never delete files without confirmation
- Always backup before major changes
- Use atomic writes (write to temp, then rename)

### Git Safety
- Never force push
- Never delete main/master branch
- Always create feature branches for changes

### Code Safety
- Never commit secrets or credentials
- Always validate inputs
- Use parameterized queries for database access

### Resource Limits
- Maximum 1000 lines of code per file
- Maximum 10 files per commit
- Timeout operations after 30 seconds
