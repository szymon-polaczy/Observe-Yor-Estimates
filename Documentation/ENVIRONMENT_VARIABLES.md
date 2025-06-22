# Environment Variables Configuration

This document outlines the environment variables that have been extracted from hardcoded values in the application to improve configurability and deployment flexibility.

## Changes Made

### 1. Database Configuration
- **Variable**: `DATABASE_PATH`
- **Default**: `./oye.db`
- **File**: `db_setup.go`
- **Description**: Path to the SQLite database file
- **Before**: `const dbPath = "./oye.db"`
- **After**: Configurable via environment variable with fallback to default

### 2. API Endpoints
- **Variable**: `SLACK_API_URL`
- **Default**: `https://slack.com/api/apps.connections.open`
- **File**: `main.go`
- **Description**: Slack API endpoint for socket connections

- **Variable**: `TIMECAMP_API_URL`
- **Default**: `https://app.timecamp.com/third_party/api`
- **Files**: `sync_tasks_to_db.go`, `sync_time_entries_to_db.go`
- **Description**: TimeCamp API base URL

### 3. Cron Schedules
- **Variable**: `TASK_SYNC_SCHEDULE`
- **Default**: `*/5 * * * *` (every 5 minutes)
- **File**: `main.go`
- **Description**: Cron schedule for task synchronization

- **Variable**: `TIME_ENTRIES_SYNC_SCHEDULE`
- **Default**: `*/10 * * * *` (every 10 minutes)
- **File**: `main.go`
- **Description**: Cron schedule for time entries synchronization

- **Variable**: `DAILY_UPDATE_SCHEDULE`
- **Default**: `0 6 * * *` (6 AM daily)
- **File**: `main.go`
- **Description**: Cron schedule for daily Slack updates

- **Variable**: `WEEKLY_UPDATE_SCHEDULE`
- **Default**: `0 8 * * 1` (8 AM on Mondays)
- **File**: `main.go`
- **Description**: Cron schedule for weekly Slack updates

### 4. UI Configuration
- **Variable**: `PROGRESS_BAR_LENGTH`
- **Default**: `10`
- **File**: `daily_slack_update.go`
- **Description**: Length of progress bars in Slack messages

## Required Environment Variables

The following environment variables are still required (unchanged from before):
- `SLACK_TOKEN`: Slack bot token
- `SLACK_WEBHOOK_URL`: Slack webhook URL for notifications
- `TIMECAMP_API_KEY`: TimeCamp API key

## Optional Environment Variables (with defaults)

All the newly added environment variables are optional and have sensible defaults:
- `DATABASE_PATH`
- `SLACK_API_URL`
- `TIMECAMP_API_URL`
- `TASK_SYNC_SCHEDULE`
- `TIME_ENTRIES_SYNC_SCHEDULE`
- `DAILY_UPDATE_SCHEDULE`
- `WEEKLY_UPDATE_SCHEDULE`
- `PROGRESS_BAR_LENGTH`

## Previously Configurable Variables

These were already configurable (no changes needed):
- `MID_POINT`: Color indicator threshold (default: 50)
- `HIGH_POINT`: Color indicator threshold (default: 90)

## Benefits

1. **Environment Flexibility**: Different database paths for dev/test/prod
2. **API Endpoint Flexibility**: Support for different API environments or proxies
3. **Schedule Customization**: Adjust sync frequencies without code changes
4. **UI Customization**: Configurable progress bar length
5. **Backward Compatibility**: All changes maintain existing defaults

## Usage Example

```bash
# Use custom database path
export DATABASE_PATH="/data/production.db"

# Use custom API endpoints
export TIMECAMP_API_URL="https://api-proxy.company.com/timecamp"
export SLACK_API_URL="https://api-proxy.company.com/slack/apps.connections.open"

# Custom sync schedules
export TASK_SYNC_SCHEDULE="*/2 * * * *"  # Every 2 minutes
export TIME_ENTRIES_SYNC_SCHEDULE="*/15 * * * *"  # Every 15 minutes
export DAILY_UPDATE_SCHEDULE="0 8 * * *"  # 8 AM instead of 6 AM

# Custom UI
export PROGRESS_BAR_LENGTH="20"  # Longer progress bars

# Run the application
./observe-yor-estimates
```

## Updated .env.example

The `.env.example` file has been updated to include all new configurable options with documentation and sensible defaults.
