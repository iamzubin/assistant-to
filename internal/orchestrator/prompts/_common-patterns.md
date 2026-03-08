# Common Development Patterns

## Testing Patterns
- Write tests before or alongside implementation
- Use table-driven tests for multiple cases
- Mock external dependencies
- Aim for >80% code coverage

## Error Handling
- Always check errors
- Wrap errors with context using fmt.Errorf
- Never ignore errors silently
- Return errors rather than logging and continuing

## Documentation
- Add comments for exported functions
- Include examples in package documentation
- Document complex algorithms
- Keep README up to date

## Code Organization
- One concern per function
- Keep functions under 50 lines
- Group related types together
- Use meaningful variable names
