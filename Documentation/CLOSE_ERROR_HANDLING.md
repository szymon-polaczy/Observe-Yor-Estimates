# Close Error Handling Patterns in Go

## The Question: Do We Need to Handle Close Errors?

**Short Answer**: It depends on the context and the type of resource.

## Best Practices by Resource Type

### 1. **Database Connections - Always Handle**
```go
// ✅ GOOD - Log close errors
defer CloseWithErrorLog(db, "database connection")

// ❌ BAD - Ignoring close errors
defer db.Close()
```
**Why**: Database close errors can indicate connection pool issues, deadlocks, or resource leaks.

### 2. **HTTP Response Bodies - Usually Handle**
```go
// ✅ GOOD - Log close errors
defer CloseWithErrorLog(response.Body, "HTTP response body")

// ⚠️ ACCEPTABLE - Silent close for simple operations
defer response.Body.Close()
```
**Why**: HTTP close errors are usually not critical but can help with debugging.

### 3. **Files - Always Handle for Writes**
```go
// ✅ CRITICAL - Return close errors for file writes
func writeToFile() error {
    file, err := os.Create("important.txt")
    if err != nil {
        return err
    }
    defer func() {
        if closeErr := file.Close(); closeErr != nil {
            // For writes, close errors can mean data loss!
            return fmt.Errorf("failed to close file: %w", closeErr)
        }
    }()
    
    _, err = file.Write(data)
    return err
}

// ✅ GOOD - Log close errors for reads
defer CloseWithErrorLog(file, "config file")
```

### 4. **WebSocket Connections - Always Handle**
```go
// ✅ GOOD - Log close errors
defer CloseWithErrorLog(conn, "WebSocket connection")
```

## Utility Functions We Created

### `CloseWithErrorLog()` - For Non-Critical Operations
```go
func CloseWithErrorLog(closer interface{ Close() error }, resourceName string) {
    if closer == nil {
        return
    }
    if err := closer.Close(); err != nil {
        appLogger.Errorf("Error closing %s: %v", resourceName, err)
    }
}
```
**Use when**: Close errors should be logged but don't affect the operation's success.

### `CloseWithErrorReturn()` - For Critical Operations
```go
func CloseWithErrorReturn(closer interface{ Close() error }) error {
    if closer == nil {
        return nil
    }
    return closer.Close()
}
```
**Use when**: Close errors must be handled by the caller (e.g., file writes, transactions).

## Decision Matrix

| Resource Type | Operation | Pattern | Reason |
|---------------|-----------|---------|--------|
| Database Connection | Any | `CloseWithErrorLog` | Connection pool health |
| File | Read | `CloseWithErrorLog` | Debugging assistance |
| File | Write | `CloseWithErrorReturn` | Data integrity |
| HTTP Response | Any | `CloseWithErrorLog` | Connection management |
| WebSocket | Any | `CloseWithErrorLog` | Connection state tracking |
| Temporary Resources | Any | `defer resource.Close()` | Performance optimization |

## Common Anti-Patterns

### ❌ DON'T: Ignore All Close Errors
```go
defer db.Close() // Bad - misses important errors
```

### ❌ DON'T: Panic on Close Errors
```go
defer func() {
    if err := db.Close(); err != nil {
        panic(err) // Bad - too aggressive
    }
}()
```

### ❌ DON'T: Complex Logic in Defer
```go
defer func() {
    if err := db.Close(); err != nil {
        // 20 lines of complex error handling... Bad!
    }
}()
```

## ✅ DO: Use Consistent Patterns
```go
// Simple, clear, consistent
defer CloseWithErrorLog(resource, "descriptive name")
```

## Go Features Demonstrated

1. **Interface Types**: `interface{ Close() error }` - allows any type with a Close method
2. **Method Sets**: Types automatically satisfy interfaces if they have the required methods
3. **Nil Checking**: Always check for nil before calling methods
4. **Error Wrapping**: Using `%w` verb to maintain error chains
5. **Defer Functions**: Automatic cleanup when function exits
6. **Anonymous Functions**: `defer func() { ... }()` pattern
7. **Named Return Values**: Can be useful for complex cleanup scenarios

## Performance Considerations

- `CloseWithErrorLog()` is very lightweight
- Logging is asynchronous in most systems
- The interface{} approach has minimal runtime overhead
- Defer has negligible performance impact for cleanup

## Summary

**Your current approach was already quite good!** The improvements we made:

1. **Consistency**: Standardized the close error handling pattern
2. **Reusability**: Created utility functions to reduce code duplication  
3. **Clarity**: Made the intent explicit with descriptive resource names
4. **Maintainability**: Centralized the logging logic

The key insight is that **close errors are usually about resource management and debugging, not business logic failures**. Handle them appropriately based on the criticality of the operation.

## Related Documentation

- [Error Handling Summary](ERROR_HANDLING_SUMMARY.md) - Overview of all error handling patterns in the application
- [Environment Variables Configuration](ENVIRONMENT_VARIABLES.md) - Configurable settings that affect error handling behavior
- [Time Entries Implementation](TIME_ENTRIES_IMPLEMENTATION.md) - Specific examples of close error handling in API integrations
