# Documentation Index

This directory contains comprehensive documentation for the Observe Your Estimates application.

## Configuration & Setup

### [Environment Variables Configuration](ENVIRONMENT_VARIABLES.md)
Complete guide to all configurable environment variables, their defaults, and usage examples. Essential reading for deployment and customization.

**Key Topics:**
- Required vs. Optional environment variables
- Database, API, and scheduling configuration
- Development vs. Production settings
- Configuration examples

## Implementation Details

### [Socket Mode to REST API Migration](SOCKET_TO_REST_MIGRATION.md)
Complete documentation of the migration from Slack Socket Mode to REST API for handling slash commands.

**Key Topics:**
- Migration summary and rationale
- Removed and added components
- New Slack app configuration requirements
- Benefits and architectural improvements

### [Time Entries Implementation](TIME_ENTRIES_IMPLEMENTATION.md)
Detailed documentation of the TimeCamp time entries integration, API handling, and database synchronization.

**Key Topics:**
- Real-time TimeCamp API integration
- Database schema for time entries
- Synchronization processes
- Usage examples and benefits

## Error Handling & Best Practices

### [Error Handling Summary](ERROR_HANDLING_SUMMARY.md)
Comprehensive overview of error handling patterns and best practices implemented throughout the application.

**Key Topics:**
- Structured logging system
- Error classification (Fatal, Recoverable, Warnings)
- Best practices and patterns
- Environment variable validation

### [Close Error Handling](CLOSE_ERROR_HANDLING.md)
Detailed guide on resource cleanup patterns and close error handling strategies.

**Key Topics:**
- Resource cleanup patterns
- When to handle close errors
- Utility functions for error handling
- Best practices by resource type

## Quick Reference

### Main Application Features
- **Real-time TimeCamp Integration**: Automatic task and time entry synchronization
- **Daily Slack Updates**: Configurable automated reports with progress visualization
- **Estimation Analysis**: Intelligent parsing of task estimation patterns
- **Robust Error Handling**: Comprehensive error management with graceful degradation
- **Configurable Environment**: All aspects configurable via environment variables

### Key Files
- **Main Application**: `../main.go` - WebSocket handling and cron scheduling
- **Database Setup**: `../db_setup.go` - Database operations and migrations
- **Task Sync**: `../sync_tasks_to_db.go` - TimeCamp task synchronization
- **Time Entries**: `../sync_time_entries_to_db.go` - TimeCamp time entries sync
- **Slack Updates**: `../daily_slack_update.go` - Daily report generation
- **Error Utilities**: `../error_handling_utils.go` - Centralized error handling
- **Logging**: `../logger.go` - Structured logging system

### Environment Setup
1. Copy `.env.example` to `.env`
2. Configure required variables: `SLACK_TOKEN`, `SLACK_WEBHOOK_URL`, `TIMECAMP_API_KEY`
3. Optionally customize schedules, paths, and UI settings
4. Build and run: `go build && ./observe-yor-estimates`

For complete setup instructions, see the main [README.md](../README.md).
