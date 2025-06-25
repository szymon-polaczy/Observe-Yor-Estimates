# CLI Commands Reference

This document provides a complete reference for all command-line interface (CLI) commands available in the OYE time tracker.

## 📋 Command Overview

| Command | Purpose | Example |
|---------|---------|---------|
| `--help` | Show help information | `./oye --help` |
| `--version` | Display version | `./oye --version` |
| `--init-db` | Initialize database | `./oye --init-db` |
| `update` | Send Slack updates | `./oye update daily` |
| `sync-tasks` | Sync tasks from TimeCamp | `./oye sync-tasks` |
| `sync-time-entries` | Sync time entries | `./oye sync-time-entries` |
| `full-sync` | Complete data sync | `./oye full-sync` |
| `add-user` | Add user to database | `./oye add-user 123 john "John Doe"` |
| `list-users` | List all users | `./oye list-users` |
| `active-users` | Show active user IDs | `./oye active-users` |
| `populate-users` | Add sample users | `./oye populate-users` |
| `threshold-check` | Check threshold violations | `./oye threshold-check` |
| `process-orphaned` | Process orphaned entries | `./oye process-orphaned` |
| `cleanup-orphaned` | Clean old orphaned entries | `./oye cleanup-orphaned 30` |
| `sync-users` | Sync users from TimeCamp | `./oye sync-users` |

## 🔧 System Commands

### Help and Version

#### `--help` / `-h` / `help`
Display comprehensive help information.

```bash
./oye --help
./oye -h
./oye help
```

**Output**: Complete command reference with examples.

#### `--version` / `version`
Show the application version.

```bash
./oye --version
./oye version
```

**Output**: `Observe-Yor-Estimates v1.0.0`

#### `--build-test` / `build-test`
Verify the binary is working correctly.

```bash
./oye --build-test
./oye build-test
```

**Output**: `Build test successful - binary is working correctly`

### Database Management

#### `--init-db` / `init-db`
Initialize the database schema and tables.

```bash
./oye --init-db
./oye init-db
```

**What it does**:
- Creates all required database tables
- Sets up indexes and constraints
- Initializes sync status tracking

**Requirements**: Valid `DATABASE_URL` environment variable.

## 📊 Update Commands

### `update <period> [public]`
Generate and send Slack time tracking updates.

**Syntax**:
```bash
./oye update <period> [public]
```

**Parameters**:
- `<period>`: Required. One of `daily`, `weekly`, or `monthly`
- `[public]`: Optional. Makes the update visible to the entire channel

**Examples**:
```bash
# Private daily update
./oye update daily

# Public weekly update  
./oye update weekly public

# Monthly summary
./oye update monthly
```

**Environment Variables**:
- `SLACK_BOT_TOKEN`: Required for Slack API access
- `CHANNEL_ID`: Target Slack channel (if using direct API)
- `RESPONSE_URL`: Slack webhook URL (alternative method)

**Output Format**: 
- Console: Progress messages and completion status
- Slack: Formatted time tracking report

## 🔄 Synchronization Commands

### `sync-tasks`
Synchronize tasks and projects from TimeCamp to the local database.

```bash
./oye sync-tasks
```

**What it syncs**:
- Project hierarchy from TimeCamp
- Task details and metadata
- Project-task relationships
- Task estimates and budgets

**Requirements**: Valid `TIMECAMP_API_KEY`

### `sync-time-entries`
Synchronize time entries from TimeCamp.

```bash
./oye sync-time-entries
```

**What it syncs**:
- Time entries for recent periods
- User time tracking data
- Task associations
- Entry timestamps and durations

**Default Sync Period**: Last 30 days

### `full-sync`
Perform complete data synchronization.

```bash
./oye full-sync
```

**Process**:
1. Sync all tasks and projects
2. Sync time entries for extended period
3. Update user information
4. Clean orphaned data
5. Update sync status

**Duration**: 1-10 minutes depending on data volume

## 👥 User Management Commands

### `add-user <user_id> <username> <display_name>`
Add a user to the local database.

**Syntax**:
```bash
./oye add-user <user_id> <username> <display_name>
```

**Parameters**:
- `<user_id>`: Numeric user ID from TimeCamp
- `<username>`: Username (e.g., "john.doe")  
- `<display_name>`: Full display name (e.g., "John Doe")

**Example**:
```bash
./oye add-user 1820471 "john.doe" "John Doe"
```

### `list-users`
Display all users in the database.

```bash
./oye list-users
```

**Output Format**:
```
Users in database:
ID: 1820471, Username: john.doe, Display: John Doe
ID: 1721068, Username: mary.smith, Display: Mary Smith
```

### `active-users`
Show user IDs that appear in time entries.

```bash
./oye active-users
```

**Purpose**: Identify which users need to be added to the users table.

**Output Format**:
```
Active user IDs in time entries:
- 1820471 (appears in 45 entries)
- 1721068 (appears in 32 entries)
```

### `populate-users`
Add sample users for testing.

```bash
./oye populate-users
```

**What it creates**:
- Sample user entries with common patterns
- Useful for development and testing
- Safe to run multiple times (upserts data)

### `sync-users`
Synchronize users from TimeCamp API.

```bash
./oye sync-users
```

**What it syncs**:
- User profiles from TimeCamp
- Display names and email addresses
- User status and permissions

## 🎯 Monitoring Commands

### `threshold-check`
Run threshold monitoring for task estimates.

```bash
./oye threshold-check
```

**What it checks**:
- Tasks exceeding estimated time
- Budget overruns
- Time allocation warnings

**Output**: Summary of threshold violations

### `process-orphaned`
Process orphaned time entries (entries without valid task associations).

```bash
./oye process-orphaned
```

**Process**:
1. Identify orphaned time entries
2. Attempt to match with existing tasks
3. Create placeholder tasks if needed
4. Update entry associations

**Before/After Count**: Shows number of orphaned entries before and after processing.

### `cleanup-orphaned <days>`
Remove old orphaned time entries.

**Syntax**:
```bash
./oye cleanup-orphaned <days>
```

**Parameters**:
- `<days>`: Number of days. Entries older than this will be removed.

**Example**:
```bash
# Remove orphaned entries older than 30 days
./oye cleanup-orphaned 30
```

**Safety**: Only removes entries that couldn't be processed after multiple attempts.

## 🌐 Environment Variables

### Required Variables
```bash
# Database connection
DATABASE_URL=postgresql://user:pass@host:port/dbname

# TimeCamp API access
TIMECAMP_API_KEY=your_api_key

# Slack integration  
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_verification_token
```

### Optional Variables
```bash
# Logging
LOG_LEVEL=info          # debug, info, warn, error

# Slack context (for direct API usage)
CHANNEL_ID=C1234567890  # Target channel
RESPONSE_URL=https://hooks.slack.com/...  # Webhook URL

# Output format
OUTPUT_JSON=true        # JSON output instead of console
```

## 🔍 Usage Patterns

### Daily Operations
```bash
# Morning sync and update
./oye sync-time-entries
./oye update daily

# Check for issues
./oye active-users
./oye threshold-check
```

### Weekly Maintenance
```bash
# Complete sync
./oye full-sync

# User management
./oye sync-users
./oye process-orphaned

# Send weekly report
./oye update weekly public
```

### Troubleshooting
```bash
# Check database connection
./oye --init-db

# Verify API access
./oye sync-tasks

# Test Slack integration
./oye update daily
```

## ❌ Error Handling

### Common Exit Codes
- `0`: Success
- `1`: General error (database, API, etc.)

### Common Error Messages

#### Database Connection Failed
```
Failed to initialize database: connection refused
```
**Solution**: Check `DATABASE_URL` and database availability.

#### TimeCamp API Error
```
Tasks sync failed: unauthorized
```
**Solution**: Verify `TIMECAMP_API_KEY` is valid.

#### Slack API Error
```
Failed to send update: invalid token
```
**Solution**: Check `SLACK_BOT_TOKEN` and permissions.

## 🚀 Advanced Usage

### Scripting Examples

#### Automated Daily Sync
```bash
#!/bin/bash
# daily-sync.sh

echo "Starting daily sync..."
./oye sync-time-entries || exit 1
./oye process-orphaned || exit 1
./oye update daily || exit 1
echo "Daily sync completed successfully"
```

#### User Setup Script
```bash
#!/bin/bash
# setup-users.sh

# Get active users and add them
./oye active-users | grep -E '^- [0-9]+' | while read -r line; do
    user_id=$(echo "$line" | grep -o '[0-9]*')
    echo "Adding user $user_id..."
    ./oye add-user "$user_id" "user$user_id" "User $user_id"
done
```

### Cron Job Examples

```bash
# Sync every 10 minutes
*/10 * * * * /opt/oye/oye-time-tracker sync-time-entries

# Daily update at 9 AM
0 9 * * * /opt/oye/oye-time-tracker update daily

# Weekly cleanup on Sundays
0 2 * * 0 /opt/oye/oye-time-tracker cleanup-orphaned 30
```

## 📖 Related Documentation

- [Installation Guide](INSTALLATION.md) - Setting up the application
- [User Management](USER_MANAGEMENT.md) - Detailed user management
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions 