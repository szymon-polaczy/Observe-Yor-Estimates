# Configuration Guide

This guide covers all configuration options for the OYE (Observe-Yor-Estimates) application, including environment variables, scheduling, and customization options.

## 📋 Environment Variables Reference

### Required Variables

#### Database Configuration
```bash
# PostgreSQL connection string
DATABASE_URL=postgresql://username:password@hostname:port/database_name

# Examples:
DATABASE_URL=postgresql://your_user:your_password@localhost:5432/your_database
DATABASE_URL=postgresql://your_user:your_password@your_host:5432/your_database
DATABASE_URL=postgresql://your_user:your_password@your_production_host:5432/your_database
```

#### TimeCamp API Configuration
```bash
# TimeCamp API key (get from Account Settings > Add-ons > API)
TIMECAMP_API_KEY=your_api_key_here

# API key format: Long alphanumeric string
# Example: TIMECAMP_API_KEY=your_timecamp_api_key_here
```

#### Slack Integration Configuration
```bash
# Slack Bot User OAuth Token (starts with xoxb-)
SLACK_BOT_TOKEN=xoxb-your-bot-token-here

# Slack Verification Token (for request validation)
SLACK_VERIFICATION_TOKEN=your_verification_token_here
```

### Optional Variables

#### Cron Schedule Customization
```bash
# Task synchronization schedule (default: every 5 minutes)
TASK_SYNC_SCHEDULE="*/5 * * * *"

# Time entries synchronization schedule (default: every 10 minutes)
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"

# Daily update schedule (default: 6 AM daily)
DAILY_UPDATE_SCHEDULE="0 6 * * *"

# Weekly update schedule (default: 8 AM Monday)
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"

# Monthly update schedule (default: 9 AM 1st of month)
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"
```

#### Logging Configuration
```bash
# Log level: debug, info, warn, error (default: info)
LOG_LEVEL=info

# Enable JSON logging format (default: false)
OUTPUT_JSON=false
```

#### Server Configuration
```bash
# HTTP server port (default: 8080)
PORT=8080

# Server timeout settings
READ_TIMEOUT=30s
WRITE_TIMEOUT=30s
```

#### Slack Channel Configuration
```bash
# Default channel for direct API usage
SLACK_DEFAULT_CHANNEL=your_channel_id_here

# Custom response URL override
RESPONSE_URL=https://hooks.slack.com/commands/your/webhook/url
```

#### Data Sync Configuration
```bash
# Number of days to sync time entries (default: 30)
SYNC_DAYS_BACK=30

# Maximum batch size for API requests (default: 100)
API_BATCH_SIZE=100

# Retry attempts for failed API calls (default: 3)
API_RETRY_ATTEMPTS=3
```

## ⏰ Cron Schedule Format

Cron schedules use the standard 5-field format:
```
┌───────────── minute (0 - 59)
│ ┌─────────── hour (0 - 23)
│ │ ┌───────── day of month (1 - 31)
│ │ │ ┌─────── month (1 - 12)
│ │ │ │ ┌───── day of week (0 - 6) (Sunday to Saturday)
│ │ │ │ │
* * * * *
```

### Common Schedule Examples

```bash
# Every minute
"* * * * *"

# Every 5 minutes
"*/5 * * * *"

# Every hour at minute 0
"0 * * * *"

# Every day at 6 AM
"0 6 * * *"

# Every Monday at 8 AM
"0 8 * * 1"

# Every 1st of month at 9 AM
"0 9 1 * *"

# Every weekday at 9 AM
"0 9 * * 1-5"

# Twice daily (9 AM and 6 PM)
"0 9,18 * * *"
```

## 🔧 Configuration Files

### Environment File (.env)

Create a `.env` file in the project root:

```bash
# Database
DATABASE_URL=postgresql://your_user:your_password@localhost:5432/your_database

# APIs
TIMECAMP_API_KEY=your_timecamp_api_key_here
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here

# Schedules (optional)
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"

# Logging
LOG_LEVEL=info

# Server
PORT=8080
```

### Docker Environment File

For Docker deployments, use the same format:

```bash
# docker-compose.yml
version: '3.8'
services:
  oye:
    build: .
    env_file: .env
    ports:
      - "8080:8080"
    depends_on:
      - postgres
      
  postgres:
    image: postgres:14
    environment:
      POSTGRES_DB: your_database_name
      POSTGRES_USER: your_database_user
      POSTGRES_PASSWORD: your_database_password
```

## 🔐 Security Configuration

### API Key Security

**Best Practices:**
```bash
# Use environment variables, never hardcode
export TIMECAMP_API_KEY="your_timecamp_api_key_here"
export SLACK_BOT_TOKEN="xoxb-your-bot-token-here"

# Rotate keys regularly
# Monitor for unauthorized usage
# Use least privilege principle
```

**Key Validation:**
```bash
# TimeCamp API key should be 20+ characters
echo $TIMECAMP_API_KEY | wc -c

# Slack bot token should start with xoxb-
echo $SLACK_BOT_TOKEN | grep "^xoxb-"

# Verification token should be 20+ characters
echo $SLACK_VERIFICATION_TOKEN | wc -c
```

### Database Security

**Connection Security:**
```bash
# Use SSL for production databases
DATABASE_URL=postgresql://your_user:your_password@your_host:5432/your_database?sslmode=require

# Restrict database user permissions
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO your_database_user;

# Use connection pooling
DATABASE_URL=postgresql://your_user:your_password@your_host:5432/your_database?pool_max_conns=20
```

## 🎛️ Advanced Configuration

### Custom Sync Intervals

Adjust sync frequencies based on your needs:

```bash
# High-frequency setup (for active teams)
TASK_SYNC_SCHEDULE="*/2 * * * *"        # Every 2 minutes
TIME_ENTRIES_SYNC_SCHEDULE="*/5 * * * *" # Every 5 minutes

# Low-frequency setup (for smaller teams)
TASK_SYNC_SCHEDULE="*/15 * * * *"       # Every 15 minutes
TIME_ENTRIES_SYNC_SCHEDULE="*/30 * * * *" # Every 30 minutes

# Off-hours only setup
TASK_SYNC_SCHEDULE="0 */2 8-18 * * 1-5" # Every 2 hours, 8AM-6PM, weekdays
```

### Multi-Environment Configuration

#### Development Environment
```bash
# .env.development
DATABASE_URL=postgresql://dev_user:dev_password@localhost:5432/dev_database
TIMECAMP_API_KEY=your_dev_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-dev-bot-token
LOG_LEVEL=debug
TASK_SYNC_SCHEDULE="*/1 * * * *"  # More frequent for testing
```

#### Staging Environment
```bash
# .env.staging
DATABASE_URL=postgresql://staging_user:staging_password@staging-db:5432/staging_database
TIMECAMP_API_KEY=your_staging_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-staging-bot-token
LOG_LEVEL=info
```

#### Production Environment
```bash
# .env.production
DATABASE_URL=postgresql://prod_user:secure_password@prod-db:5432/prod_database
TIMECAMP_API_KEY=your_production_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-production-bot-token
LOG_LEVEL=warn
```

### Load Configuration Script

```bash
#!/bin/bash
# load-config.sh

ENV=${1:-development}

if [ -f ".env.$ENV" ]; then
    export $(grep -v '^#' .env.$ENV | xargs)
    echo "Loaded $ENV configuration"
else
    echo "Configuration file .env.$ENV not found"
    exit 1
fi

go run .
```

## 📊 Monitoring Configuration

### Health Check Configuration
```bash
# Health check endpoint timeout
HEALTH_CHECK_TIMEOUT=5s

# Database connection timeout
DB_CONNECTION_TIMEOUT=10s

# API request timeout
API_REQUEST_TIMEOUT=30s
```

### Performance Tuning
```bash
# Database connection pool settings
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=300s

# API rate limiting
API_RATE_LIMIT=100  # requests per minute
API_BURST_LIMIT=10  # burst requests
```

## 🔧 Troubleshooting Configuration

### Debug Configuration
```bash
# Enable verbose logging
LOG_LEVEL=debug
OUTPUT_JSON=true

# Disable cron jobs for testing
DISABLE_CRON=true

# Test mode (shorter timeouts)
TEST_MODE=true
API_REQUEST_TIMEOUT=5s
```

### Validation Commands
```bash
# Validate configuration
go run . --validate-config

# Test database connection
go run . --test-db

# Test API connections
go run . --test-apis

# Verify cron schedules
go run . --validate-schedules
```

## 📖 Configuration Examples

### Minimal Configuration
```bash
# Absolute minimum required
DATABASE_URL=postgresql://your_user:your_password@your_host:5432/your_database
TIMECAMP_API_KEY=your_timecamp_api_key_here
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here
```

### Complete Configuration
```bash
# Full configuration with all options
DATABASE_URL=postgresql://your_user:your_password@your_host:5432/your_database?sslmode=require
TIMECAMP_API_KEY=your_timecamp_api_key_here
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here

# Schedules
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"

# Server
PORT=8080
READ_TIMEOUT=30s
WRITE_TIMEOUT=30s

# Logging
LOG_LEVEL=info
OUTPUT_JSON=false

# Sync settings
SYNC_DAYS_BACK=30
API_BATCH_SIZE=100
API_RETRY_ATTEMPTS=3

# Performance
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
```

## 📖 Related Documentation

- [Installation Guide](INSTALLATION.md) - Setting up environment variables
- [Deployment Guide](DEPLOYMENT_GUIDE.md) - Production configuration
- [Troubleshooting](TROUBLESHOOTING.md) - Configuration-related issues
- [Scheduled Tasks](SCHEDULED_TASKS.md) - Detailed cron configuration 