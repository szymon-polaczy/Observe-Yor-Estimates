# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Observe-Yor-Estimates (OYE)** is a monolithic Go application that bridges TimeCamp time tracking with Slack notifications. It provides real-time reporting via Slack slash commands, automated data synchronization, threshold monitoring for budget overruns, and scheduled updates.

## Common Development Commands

### Build and Run
```bash
go run .                          # Start the application
go build -o bin/observe-yor-estimates   # Build binary
go run . --version               # Show version
go run . --help                  # Show help
```

### Database Management
```bash
go run . --init-db               # Initialize database schema
go run . --build-test            # Test binary build
```

### Data Synchronization
```bash
go run . sync-tasks              # Sync tasks from TimeCamp
go run . sync-time-entries       # Sync recent time entries
go run . full-sync               # Complete data synchronization
```

### Reporting Commands
```bash
go run . update daily            # Generate daily reports
go run . update weekly           # Generate weekly reports
go run . update monthly          # Generate monthly reports
```

### User Management
```bash
go run . list-users              # Show all users
go run . add-user <id> <user> <name>  # Add specific user
go run . active-users            # Show users with time entries
```

### Testing
```bash
go run . test-command "/oye daily"      # Test Slack commands locally
go run . test-command "/oye sync"       # Test sync command
go run . test-message-limits            # Test Slack message size limits (blocks/chars)
```

### Message Limits Testing
**IMPORTANT**: Always run message limits testing after making changes to Slack message formatting:

```bash
go run . test-message-limits
```

This command validates that all Slack messages respect:
- **Block limit**: 50 blocks maximum per message
- **Character limit**: 3000 characters maximum per message payload

The test generates mock data with various scenarios:
- Normal task messages (50 tasks)
- Large task sets (100 tasks) 
- Comment-heavy tasks (20 comments per task)

**Expected Results**: All normal messages should show ✅ VALID status. Some test scenarios may intentionally fail to verify limit detection works correctly.

## Architecture Overview

### Core Components
- **main.go**: CLI entry point and command routing
- **server.go**: HTTP server handling Slack slash commands via `/slack/oye` endpoint
- **smart_router.go**: Intelligent command parsing and job management with async processing
- **db_setup.go**: PostgreSQL database connection and schema management
- **full_sync.go**: Orchestrates complete TimeCamp to database synchronization
- **sync_*.go**: Individual sync modules for tasks, time entries, and data cleanup
- **slack_*.go**: Slack API integration utilities and message formatting
- **user_management.go**: User CRUD operations and name resolution

### Data Flow
1. **TimeCamp API** → **Sync modules** → **PostgreSQL database**
2. **Slack commands** → **Smart router** → **Database queries** → **Formatted responses**
3. **Scheduled jobs** → **Automated reports** → **Slack notifications**

### Key Design Patterns
- **Monolithic architecture**: Single binary for simplified deployment and debugging
- **Async job processing**: Long-running operations handled via background jobs
- **Smart routing**: Context-aware command parsing with project filtering
- **Progress tracking**: Real-time status updates for sync operations

## Environment Configuration

Required environment variables in `.env` file:
```bash
DATABASE_URL=postgresql://user:pass@host:port/dbname
TIMECAMP_API_KEY=your_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_slack_verification_token  # Optional but recommended
SLACK_WEBHOOK_URL=your_slack_webhook_url
```

## Database Schema

Key tables:
- **tasks**: Project/task hierarchy from TimeCamp
- **time_entries**: Individual time tracking records
- **users**: User management with TimeCamp ID mapping
- **sync_status**: Track last sync timestamps and progress

## Slack Integration

### Main Command Structure
```bash
/oye daily                    # Daily time summary
/oye weekly public           # Public weekly report
/oye "project name" today    # Project-specific reports
/oye over 80 weekly         # Tasks over 80% estimate
/oye sync                   # Full data synchronization
```

### Command Processing Flow
1. Slack sends request to `/slack/oye` endpoint
2. **smart_router.go** parses command and context
3. Background job processes request asynchronously
4. Progress updates sent to Slack during long operations
5. Final results delivered as formatted messages

## Scheduled Tasks (Cron Jobs)

Default schedules defined in cron initialization:
- **Task sync**: Every 3 hours
- **Time entries sync**: Every 10 minutes
- **Daily reports**: 6 AM daily
- **Weekly reports**: 8 AM Monday
- **Monthly reports**: 9 AM 1st of month
- **Threshold monitoring**: Every 15 minutes
- **Orphaned entry cleanup**: Every hour

## Development Patterns

### Error Handling
- Use `error_handling_utils.go` for consistent error responses
- Log errors with structured logging via `logger.go`
- Return user-friendly messages to Slack

### Database Operations
- Use parameterized queries to prevent SQL injection  
- Wrap operations in transactions where appropriate
- Connection pooling handled by `db_setup.go`

### API Integration
- TimeCamp API calls should handle rate limiting
- Slack API responses must be within 3-second timeout
- Use async processing for long-running operations

## File Structure Patterns

- **Single responsibility**: Each sync module handles one data type
- **Centralized utilities**: Common functions in dedicated utility files
- **Clear separation**: Database, API, and business logic separated
- **Configuration**: Environment-based config, no hardcoded values

## Testing and Validation

- Use `test-command` for local Slack command testing
- **REQUIRED**: Run `test-message-limits` after any Slack message formatting changes
- Check database connectivity before operations
- Validate API tokens and permissions during startup
- Monitor sync job progress and handle failures gracefully

### Build Validation Requirements
Before successful builds and deployments, ensure:
1. Code compiles without errors: `go build -o bin/observe-yor-estimates`
2. Message limits pass: `go run . test-message-limits` (all normal messages show ✅ VALID)
3. Basic functionality works: `go run . test-command "/oye daily"`

## Deployment Notes

- **Binary deployment**: Single executable with all dependencies
- **Docker ready**: Multi-stage Dockerfile for containerized deployment
- **Railway optimized**: Configured for Railway platform deployment
- **Health checks**: `/health` endpoint for monitoring
- **Port**: Defaults to 8080, configurable via PORT environment variable