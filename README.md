# Observe Your Estimates - Daily Slack Updates

This application provides daily Slack updates for task changes and time tracking with estimation analysis, integrating with TimeCamp API for real-time data synchronization.

## Features

- **Daily Slack Updates**: Automatically sends daily reports at 6 AM (configurable) showing task progress
- **Real-Time TimeCamp Integration**: Syncs tasks and time entries from TimeCamp API
- **Estimation Analysis**: Parses task names for estimation patterns like `[8-12]` and calculates usage percentage
- **Time Tracking**: Shows start time, yesterday's time, and today's time for each task using real TimeCamp data
- **Change Detection**: Tracks task changes and only reports on tasks that have been modified
- **Broken Estimation Detection**: Identifies when optimistic estimates are higher than pessimistic ones
- **Configurable Environment**: All schedules, API endpoints, and settings are configurable via environment variables
- **Robust Error Handling**: Comprehensive error handling with structured logging and graceful failure recovery

## Quick Setup

For first-time users:

1. **Configure Environment**: Set up your `.env` file with API keys (see Setup section below)
2. **Initial Data Sync**: Run full synchronization to populate your database
   ```bash
   ./observe-yor-estimates full-sync
   ```
3. **Start Automatic Mode**: Run the application continuously
   ```bash
   ./observe-yor-estimates
   ```

## Setup

1. **Environment Configuration**: Copy `.env.example` to `.env` and configure:
   ```bash
   cp .env.example .env
   ```

2. **Required Environment Variables**:
   - `SLACK_TOKEN`: Your Slack bot token
   - `SLACK_WEBHOOK_URL`: Slack webhook URL for notifications  
   - `TIMECAMP_API_KEY`: Your TimeCamp API key

3. **Optional Environment Variables** (with defaults):
   - `DATABASE_PATH`: Database file path (default: `./oye.db`)
   - `SLACK_API_URL`: Slack API endpoint (default: `https://slack.com/api/apps.connections.open`)
   - `TIMECAMP_API_URL`: TimeCamp API base URL (default: `https://app.timecamp.com/third_party/api`)
   - `TASK_SYNC_SCHEDULE`: Task sync cron schedule (default: `*/5 * * * *` - every 5 minutes)
   - `TIME_ENTRIES_SYNC_SCHEDULE`: Time entries sync schedule (default: `*/10 * * * *` - every 10 minutes)
   - `DAILY_UPDATE_SCHEDULE`: Daily update schedule (default: `0 6 * * *` - 6 AM daily)
   - `PROGRESS_BAR_LENGTH`: Progress bar length in Slack messages (default: `10`)
   - `MID_POINT`: Color threshold percentage (default: `50`)
   - `HIGH_POINT`: Color threshold percentage (default: `90`)

4. **Build the Application**:
   ```bash
   go build
   ```

## Usage

### Automatic Operation (Recommended)
The application runs continuously with automatic synchronization and daily updates:
```bash
./observe-yor-estimates
```
This will:
- Sync tasks from TimeCamp every 5 minutes (configurable)
- Sync time entries from TimeCamp every 10 minutes (configurable)  
- Send daily Slack updates at 6 AM (configurable)

### Manual Commands
For testing and manual operations:

**Manual Daily Update**:
```bash
./observe-yor-estimates daily-update
```

**Manual Recent Sync (Incremental)**:
```bash
# Sync recent time entries (last day only - fast)
./observe-yor-estimates sync-time-entries

# Sync all tasks (always complete sync)
./observe-yor-estimates sync-tasks
```

**Manual Full Sync (Initial Setup)**:
```bash
# Full sync of everything (for initial setup or recovery)
./observe-yor-estimates full-sync

# Full sync of tasks only
./observe-yor-estimates full-sync-tasks

# Full sync of time entries only (last 6 months)
./observe-yor-estimates full-sync-time-entries
```

### When to Use Each Command

**For Daily Operations** (Use these regularly):
- `sync-time-entries` - Fast, only syncs yesterday and today's entries
- `sync-tasks` - Always syncs all tasks (needed for change detection)
- `daily-update` - Sends Slack notification with recent changes

**For Initial Setup or Recovery** (Use these occasionally):
- `full-sync` - Complete synchronization, use when setting up for the first time
- `full-sync-time-entries` - Get 6 months of historical time entries
- `full-sync-tasks` - Same as regular task sync but with clearer intent

**Performance Impact**:
- Regular sync: ~1-2 seconds for time entries, ~30 seconds for all tasks
- Full sync: ~30 seconds for tasks, ~10 seconds for 6 months of time entries

## Task Name Estimation Format

The system recognizes estimation patterns in task names:

- **Valid Estimation**: `Task name [8-12]` - Shows "Estimation: 8-12 hours (35% used)"
- **Broken Estimation**: `Task name [15-10]` - Shows "Estimation: 15-10 hours (broken estimation)"
- **No Estimation**: `Task name` - Shows "no estimation given"

## Database Schema

The application uses SQLite database with the following main tables:

### `tasks` table
- `task_id`: Primary key (from TimeCamp)
- `parent_id`: Parent task ID
- `assigned_by`: User who assigned the task
- `name`: Task name (may contain estimation)
- `level`: Task hierarchy level
- `root_group_id`: Root group identifier

### `task_history` table
- `id`: Auto-increment primary key
- `task_id`: Reference to tasks table
- `name`: Task name at time of change
- `timestamp`: When the change occurred
- `change_type`: Type of change (created, name_changed, etc.)
- `previous_value`: Previous value
- `current_value`: New value

### `time_entries` table
- `id`: Primary key (from TimeCamp)
- `task_id`: Reference to tasks table
- `user_id`: TimeCamp user ID
- `date`: Entry date (YYYY-MM-DD)
- `start_time`: Start time (HH:MM:SS)
- `end_time`: End time (HH:MM:SS)
- `duration`: Duration in seconds
- `description`: Entry description
- `billable`: Whether entry is billable (0/1)
- `last_modify`: Last modification timestamp

## Slack Message Format

Daily updates include:
- **Header**: Date and title with summary statistics
- **Task Sections**: For each changed task:
  - Task name with progress bar visualization
  - Start time from first time entry
  - Yesterday's total time from TimeCamp
  - Today's total time from TimeCamp
  - Estimation information with usage percentage or status
  - Color-coded progress indicators (green/yellow/red based on thresholds)

## Documentation

For detailed information about the application's architecture and configuration:

- **[Environment Variables Configuration](Documentation/ENVIRONMENT_VARIABLES.md)** - Complete guide to all configurable environment variables, their defaults, and usage examples
- **[Error Handling Summary](Documentation/ERROR_HANDLING_SUMMARY.md)** - Comprehensive overview of error handling patterns and best practices implemented in the application
- **[Close Error Handling](Documentation/CLOSE_ERROR_HANDLING.md)** - Detailed guide on resource cleanup patterns and close error handling strategies
- **[Time Entries Implementation](Documentation/TIME_ENTRIES_IMPLEMENTATION.md)** - Detailed documentation of the TimeCamp time entries integration, API handling, and database synchronization

## Project Structure
```
/home/haven/Documents/Observe-Yor-Estimates/
├── Documentation/
│   ├── ENVIRONMENT_VARIABLES.md      # Environment configuration guide
│   ├── ERROR_HANDLING_SUMMARY.md     # Error handling best practices
│   ├── CLOSE_ERROR_HANDLING.md       # Resource cleanup patterns
│   └── TIME_ENTRIES_IMPLEMENTATION.md # TimeCamp integration details
├── error_handling_utils.go           # Centralized error handling utilities
├── logger.go                         # Structured logging system
├── main.go                           # Main application with WebSocket handling
├── sync_tasks_to_db.go              # Task synchronization with TimeCamp API
├── sync_time_entries_to_db.go       # Recent time entries synchronization
├── full_sync.go                     # Full historical data synchronization
├── daily_slack_update.go            # Slack notifications with real-time data
├── db_setup.go                      # Database operations with error handling
├── .env.example                     # Environment variables template
├── go.mod                           # Go module dependencies
├── go.sum                           # Go dependency checksums
└── README.md                        # This documentation
```

## Synchronization Schedules

The application runs three main synchronization processes:

- **Task Sync**: Every 5 minutes (`*/5 * * * *`) - Syncs all tasks from TimeCamp API
- **Time Entries Sync**: Every 10 minutes (`*/10 * * * *`) - Syncs recent time entries (last day) from TimeCamp API  
- **Daily Updates**: Every day at 6 AM (`0 6 * * *`) - Sends Slack notifications

All schedules are configurable via environment variables using standard cron format.

**Performance Optimization**: 
- Regular cron jobs only sync recent time entries (last day) for efficiency
- Use `full-sync` commands for initial setup or when you need to sync historical data

## Building and Running

```bash
# Install dependencies
go mod tidy

# Build the application
go build

# Run with automatic scheduling (recommended)
./observe-yor-estimates

# Test daily update manually
./observe-yor-estimates daily-update

# Test time entries sync manually
./observe-yor-estimates sync-time-entries
```

## Configuration Examples

### Development Environment
```bash
# Quick sync for development
export TASK_SYNC_SCHEDULE="*/1 * * * *"        # Every minute
export TIME_ENTRIES_SYNC_SCHEDULE="*/2 * * * *" # Every 2 minutes
export DAILY_UPDATE_SCHEDULE="*/5 * * * *"     # Every 5 minutes for testing
```

### Production Environment
```bash
# Standard production settings
export DATABASE_PATH="/var/lib/oye/production.db"
export TASK_SYNC_SCHEDULE="*/10 * * * *"       # Every 10 minutes
export TIME_ENTRIES_SYNC_SCHEDULE="*/15 * * * *" # Every 15 minutes
export DAILY_UPDATE_SCHEDULE="0 8 * * *"       # 8 AM daily
```
