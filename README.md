# Observe Your Estimates - Daily Slack Updates

This application provides daily Slack updates for task changes and time tracking with estimation analysis.

## Features

- **Daily Slack Updates**: Automatically sends daily reports at 9 AM showing task progress
- **Estimation Analysis**: Parses task names for estimation patterns like `[8-12]` and calculates usage percentage
- **Time Tracking**: Shows start time, yesterday's time, and today's time for each task
- **Change Detection**: Tracks task changes and only reports on tasks that have been modified
- **Broken Estimation Detection**: Identifies when optimistic estimates are higher than pessimistic ones

## Setup

1. **Environment Configuration**: Copy `.env.example` to `.env` and configure:
   ```bash
   cp .env.example .env
   ```

2. **Configure Slack Webhook**: 
   - Create a Slack webhook URL in your workspace
   - Add it to the `.env` file as `SLACK_WEBHOOK_URL`

3. **Configure TimeCamp API**:
   - Get your TimeCamp API key
   - Add it to the `.env` file as `TIMECAMP_API_KEY`

## Usage

### Manual Daily Update
To test the daily update functionality:
```bash
./observe-yor-estimates daily-update
```

### Automatic Daily Updates
The application automatically sends daily updates at 6 AM when running in normal mode:
```bash
./observe-yor-estimates
```

## Task Name Estimation Format

The system recognizes estimation patterns in task names:

- **Valid Estimation**: `Task name [8-12]` - Shows "Estimation: 8-12 hours (35% used)"
- **Broken Estimation**: `Task name [15-10]` - Shows "Estimation: 15-10 hours (broken estimation)"
- **No Estimation**: `Task name` - Shows "no estimation given"

## Database Schema

The application creates two main tables:

### `tasks` table
- `task_id`: Primary key
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

## Slack Message Format

Daily updates include:
- **Header**: Date and title
- **Task Sections**: For each changed task:
  - Task name
  - Start time
  - Yesterday's time
  - Today's time
  - Estimation information with usage percentage or status

## Cron Schedule

- **Task Sync**: Every 5 minutes (`*/5 * * * *`)
- **Daily Updates**: Every day at 6 AM (`0 6 * * *`)

## Building and Running

```bash
# Build the application
go build

# Run with automatic scheduling
./observe-yor-estimates

# Test daily update manually
./observe-yor-estimates daily-update
```
