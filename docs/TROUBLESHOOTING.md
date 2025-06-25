# Troubleshooting Guide

This guide helps diagnose and resolve common issues with the OYE (Observe-Yor-Estimates) application.

## 🚨 Common Issues

### Database Connection Problems

#### Issue: "Failed to initialize database"
**Symptoms**:
```
Failed to initialize database: connection refused
ERROR: database connection failed
```

**Debugging Steps**:
```bash
# 1. Check database URL format
echo $DATABASE_URL
# Should be: postgresql://user:pass@host:port/dbname

# 2. Test direct connection
psql $DATABASE_URL -c "SELECT 1;"

# 3. Check if database server is running
pg_isready -h localhost -p 5432

# 4. Verify credentials
psql -h localhost -U your_user -d your_db
```

**Solutions**:
- **Invalid URL**: Fix format in `DATABASE_URL`
- **Server down**: Start PostgreSQL service
- **Wrong credentials**: Update username/password
- **Network issues**: Check firewall/network connectivity

#### Issue: "Database tables not found"
**Symptoms**:
```
ERROR: relation "tasks" does not exist
ERROR: table doesn't exist
```

**Solution**:
```bash
# Initialize database schema
./oye --init-db
```

### TimeCamp API Issues

#### Issue: "TimeCamp API unauthorized"
**Symptoms**:
```
Tasks sync failed: unauthorized
ERROR 401: Invalid API key
```

**Debugging Steps**:
```bash
# 1. Check API key format
echo $TIMECAMP_API_KEY
# Should be a long alphanumeric string

# 2. Test API key directly
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  https://www.timecamp.com/third_party/api/users/format/json

# 3. Verify account access
# Log into TimeCamp web interface
# Go to Account Settings > Add-ons > API
```

**Solutions**:
- **Invalid key**: Generate new API key in TimeCamp
- **Expired key**: Refresh API key
- **Account issues**: Check TimeCamp subscription/permissions

#### Issue: "TimeCamp API rate limiting"
**Symptoms**:
```
ERROR 429: Too Many Requests
Sync failed: rate limit exceeded
```

**Solutions**:
- Wait 1 hour before retrying
- Reduce sync frequency
- Contact TimeCamp support for rate limit increase

### Slack Integration Issues

#### Issue: "Slash command not working"
**Symptoms**:
- `/oye` command not recognized in Slack
- "Unknown command" error

**Debugging Steps**:
```bash
# 1. Verify app installation
# Check Slack workspace apps list

# 2. Test request URL accessibility
curl -X POST https://your-domain.com/slack/oye

# 3. Check slash command configuration
# Slack App > Slash Commands > /oye
```

**Solutions**:
- **App not installed**: Install/reinstall Slack app
- **Wrong URL**: Update request URL in Slack app config
- **URL not accessible**: Check firewall/DNS settings

#### Issue: "Application did not respond"
**Symptoms**:
```
This app took too long to respond
Timeout error in Slack
```

**Debugging Steps**:
```bash
# 1. Check application health
curl https://your-domain.com/health

# 2. Check application logs
./oye  # Look for error messages

# 3. Test response time
time curl -X POST https://your-domain.com/slack/oye \
  -d "token=test" -d "text=help"
```

**Solutions**:
- **App down**: Restart the application
- **Slow response**: Optimize database queries
- **Invalid URL**: Verify and update Slack app configuration

#### Issue: "No response after acknowledgment"
**Symptoms**:
- Get "Processing..." message
- No final response received

**Debugging Steps**:
```bash
# 1. Test CLI command directly
./oye update daily

# 2. Check database connection
./oye --init-db

# 3. Verify all environment variables
env | grep -E "(SLACK_|TIMECAMP_|DATABASE_)"

# 4. Check for background job processing
# Look for job queue or processing errors in logs
```

**Solutions**:
- **Database issues**: Fix database connection
- **API failures**: Check TimeCamp/Slack API access
- **Environment vars**: Ensure all required variables are set

### Synchronization Issues

#### Issue: "No data syncing from TimeCamp"
**Symptoms**:
```
Sync completed but no data updated
Empty time entries after sync
```

**Debugging Steps**:
```bash
# 1. Check TimeCamp data directly
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/entries/format/json/from/2024-11-01/to/2024-11-30"

# 2. Verify database contents
psql $DATABASE_URL -c "SELECT COUNT(*) FROM time_entries;"

# 3. Check sync date ranges
./oye sync-time-entries  # Note any date range messages
```

**Solutions**:
- **No TimeCamp data**: Verify time entries exist in TimeCamp
- **Wrong date range**: Adjust sync parameters
- **Data filtering**: Check if data is being filtered out

#### Issue: "Orphaned time entries"
**Symptoms**:
```
Found X orphaned time entries
Time entries without valid tasks
```

**Solution**:
```bash
# Process orphaned entries
./oye process-orphaned

# If too many orphaned entries
./oye cleanup-orphaned 30  # Remove entries older than 30 days
```

### User Management Issues

#### Issue: "Users showing as user[ID]"
**Symptoms**:
```
Report shows: user1820471 instead of John Doe
User names not resolving
```

**Solutions**:
```bash
# 1. Check current users
./oye list-users

# 2. Add missing users
./oye active-users  # See which users need to be added
./oye add-user 1820471 "john.doe" "John Doe"

# 3. Bulk populate users
./oye populate-users  # Add sample users

# 4. Sync users from TimeCamp
./oye sync-users
```

## 🔧 Debugging Techniques

### Enable Debug Logging

```bash
# Set debug log level
export LOG_LEVEL=debug

# Run application with verbose output
./oye

# For specific operations
LOG_LEVEL=debug ./oye sync-tasks
```

### Database Debugging

```bash
# Connect to database directly
psql $DATABASE_URL

# Check table contents
\dt  # List tables
SELECT COUNT(*) FROM tasks;
SELECT COUNT(*) FROM time_entries;
SELECT * FROM users LIMIT 5;

# Check recent sync status
SELECT * FROM sync_status ORDER BY last_sync DESC;
```

### API Debugging

```bash
# Test TimeCamp API
curl -v -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  https://www.timecamp.com/third_party/api/users/format/json

# Test Slack API (if using bot token directly)
curl -X POST https://slack.com/api/auth.test \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN"
```

### Network Debugging

```bash
# Check if service is accessible
curl -I https://your-domain.com/health

# Test from different network
# Use online tools like httpstat.us or curl from VPS

# Check DNS resolution
nslookup your-domain.com
dig your-domain.com
```

## 📊 Performance Issues

### High Memory Usage

**Symptoms**:
```
Application using >500MB RAM
Out of memory errors
```

**Solutions**:
```bash
# Check memory usage
ps aux | grep oye
top -p $(pgrep oye)

# Reduce batch sizes (if configurable)
# Restart application periodically
# Increase server memory
```

### Slow Response Times

**Symptoms**:
```
Slack commands timing out
Long wait times for reports
```

**Debugging**:
```bash
# Time specific operations
time ./oye update daily
time ./oye sync-tasks

# Profile database queries
# Enable query logging in PostgreSQL
```

**Solutions**:
- Add database indexes
- Optimize query patterns
- Cache frequently accessed data
- Reduce data volume in reports

### High CPU Usage

**Symptoms**:
```
Application consuming 100% CPU
Server becoming unresponsive
```

**Solutions**:
```bash
# Identify CPU-intensive operations
top -p $(pgrep oye)

# Check for infinite loops in logs
# Reduce sync frequency
# Optimize data processing algorithms
```

## 🚨 Error Codes Reference

### Exit Codes
- `0`: Success
- `1`: General error (database, API, configuration)
- `2`: Invalid command line arguments

### HTTP Status Codes
- `200`: Success
- `400`: Bad request (invalid command format)
- `401`: Unauthorized (invalid Slack token)
- `405`: Method not allowed (wrong HTTP method)
- `500`: Internal server error

### Database Error Patterns
```
connection refused     → Database server down
relation does not exist → Tables not initialized
permission denied      → Wrong database credentials
timeout               → Network/performance issues
```

### API Error Patterns
```
401 Unauthorized      → Invalid API key
429 Too Many Requests → Rate limiting
503 Service Unavailable → Service down
timeout               → Network issues
```

## 🛠️ Recovery Procedures

### Complete System Reset

```bash
# 1. Stop application
pkill oye

# 2. Backup database (optional)
pg_dump $DATABASE_URL > backup.sql

# 3. Reset database
./oye --init-db

# 4. Re-sync all data
./oye full-sync

# 5. Verify functionality
./oye --build-test
curl https://your-domain.com/health
```

### Partial Data Recovery

```bash
# Re-sync specific data types
./oye sync-tasks      # Re-sync tasks only
./oye sync-time-entries  # Re-sync time entries only
./oye sync-users      # Re-sync users only
```

### Environment Reset

```bash
# 1. Verify all environment variables
cat .env

# 2. Test individual components
echo $DATABASE_URL | grep postgresql
echo $TIMECAMP_API_KEY | wc -c  # Should be >20 characters
echo $SLACK_BOT_TOKEN | grep xoxb

# 3. Test connectivity
psql $DATABASE_URL -c "SELECT 1;"
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  https://www.timecamp.com/third_party/api/users/format/json
```

## 📞 Getting Help

### Log Collection

Before seeking help, collect relevant logs:

```bash
# Application logs
./oye > app.log 2>&1

# System logs (if using systemd)
journalctl -u oye.service > system.log

# Database logs
# Check PostgreSQL log directory
tail -f /var/log/postgresql/postgresql-*.log
```

### Information to Provide

When reporting issues, include:
1. **Error message**: Complete error text
2. **Environment**: OS, Go version, database version
3. **Configuration**: Environment variables (redact secrets)
4. **Logs**: Relevant log excerpts
5. **Steps to reproduce**: What you did before the error
6. **Expected behavior**: What should have happened

### Self-Help Checklist

Before escalating issues:
- [ ] Checked this troubleshooting guide
- [ ] Verified environment variables are set correctly
- [ ] Tested database connectivity
- [ ] Verified API keys are valid
- [ ] Checked application logs for errors
- [ ] Tried restarting the application
- [ ] Tested with CLI commands before Slack commands

## 📖 Related Documentation

- [Installation Guide](INSTALLATION.md) - Initial setup procedures
- [Configuration Guide](CONFIGURATION.md) - Environment variable details
- [CLI Commands](CLI_COMMANDS.md) - Command-line debugging tools
- [Slack Integration](SLACK_INTEGRATION.md) - Slack-specific troubleshooting 