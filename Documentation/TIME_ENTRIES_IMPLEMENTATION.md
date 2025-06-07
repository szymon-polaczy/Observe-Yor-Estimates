# Time Entries Implementation

## Overview

This document describes the implementation of real TimeCamp time entries integration, replacing the previous sample data system.

## Changes Made

### 1. Removed Sample Data System
- **Deleted**: `sample_data.go` file
- **Removed**: All references to `AddSampleData()` function
- **Removed**: Simulation functions (`simulateTimeData`, `simulatePercentageUsed`)

### 2. Created Time Entries Sync System
- **New File**: `sync_time_entries_to_db.go`
- **Purpose**: Fetches real time entries from TimeCamp API and stores them in database
- **Frequency**: Runs every 10 minutes via cron job

### 3. Database Schema Updates
- **New Table**: `time_entries` with the following structure:
  ```sql
  CREATE TABLE time_entries (
      id INTEGER PRIMARY KEY,
      task_id INTEGER NOT NULL,
      user_id INTEGER NOT NULL,
      date TEXT NOT NULL,
      start_time TEXT,
      end_time TEXT,
      duration INTEGER NOT NULL,
      description TEXT,
      billable INTEGER DEFAULT 0,
      locked INTEGER DEFAULT 0,
      modify_time TEXT,
      FOREIGN KEY (task_id) REFERENCES tasks(task_id)
  );
  ```

### 4. Updated Daily Slack Updates
- **Modified**: `daily_slack_update.go` to use real time entries data
- **Removed**: Simulation functions
- **Enhanced**: Real-time calculation of yesterday/today time entries
- **Improved**: Estimation parsing to handle actual task names

### 5. New Command Line Options
- `sync-time-entries`: Manually sync time entries from TimeCamp
- `sync-tasks`: Manually sync tasks from TimeCamp
- `daily-update`: Send daily Slack update
- Invalid commands now show available options and exit

### 6. Cron Job Integration
- **Tasks Sync**: Every 5 minutes (`*/5 * * * *`)
- **Time Entries Sync**: Every 10 minutes (`*/10 * * * *`)
- **Daily Slack Update**: Every day at 6 AM (`0 6 * * *`)

## API Integration

### TimeCamp Time Entries API
- **Endpoint**: `https://app.timecamp.com/third_party/api/entries`
- **Method**: GET
- **Parameters**:
  - `from`: Start date (YYYY-MM-DD)
  - `to`: End date (YYYY-MM-DD)
  - `format`: json
- **Authentication**: Bearer token via `TIMECAMP_API_KEY` environment variable

### Data Handling
- **Flexible JSON Parsing**: Handles mixed string/number types from API
- **Scientific Notation Support**: Converts IDs in scientific notation (e.g., `2.43102227e+08`)
- **Error Resilience**: Continues processing even if individual entries fail
- **Data Validation**: Validates required fields before database insertion

## Time Duration Formatting

The system formats time durations from seconds to human-readable format:
- `3600 seconds` → `1h 0m`
- `5400 seconds` → `1h 30m`
- `900 seconds` → `15m`

## Slack Message Format

Daily updates now include:
- **Task Name**: With parsed estimation information
- **Start Time**: First recorded time entry for the task
- **Yesterday**: Total time spent yesterday
- **Today**: Total time spent today
- **Estimation**: Parsed from task name in format `[min-max]` hours

## Error Handling

All functions use the project's standard Logger for consistent error reporting:
- **API Errors**: HTTP status codes and response bodies logged
- **Database Errors**: Connection and query failures handled gracefully
- **Parsing Errors**: Invalid data logged with warning level, processing continues
- **Cron Failures**: Logged but don't crash the application

## Usage Examples

```bash
# Sync time entries manually
./observe-yor-estimates sync-time-entries

# Sync tasks manually
./observe-yor-estimates sync-tasks

# Send daily update manually
./observe-yor-estimates daily-update

# Show available commands
./observe-yor-estimates help
```

## Environment Variables Required

```env
TIMECAMP_API_KEY=your_timecamp_api_key
SLACK_WEBHOOK_URL=your_slack_webhook_url
SLACK_TOKEN=your_slack_app_token
```
