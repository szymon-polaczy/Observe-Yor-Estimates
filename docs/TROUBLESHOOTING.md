# Troubleshooting Guide

This guide helps diagnose and resolve common issues with the OYE (Observe-Yor-Estimates) application quickly and effectively.

## 🔍 Quick Diagnosis Decision Tree

```mermaid
graph TD
    Start([Issue Occurred]) --> Type{What type of issue?}
    
    Type -->|Database| DB_Issue[Database Connection Issue]
    Type -->|Slack| Slack_Issue[Slack Integration Issue]
    Type -->|Sync| Sync_Issue[Data Sync Issue]
    Type -->|General| Gen_Issue[General Application Issue]
    
    DB_Issue --> DB_Check{Can connect<br/>to database?}
    DB_Check -->|Yes| DB_Schema[Check schema:<br/>./oye --init-db]
    DB_Check -->|No| DB_Fix[Fix DATABASE_URL<br/>Check credentials]
    
    Slack_Issue --> Slack_Check{Slash command<br/>working?}
    Slack_Check -->|No| Slack_App[Check app installation<br/>& slash command config]
    Slack_Check -->|Yes| Slack_Response{Getting responses?}
    Slack_Response -->|No| Slack_Timeout[Check app URL<br/>& health endpoint]
    Slack_Response -->|Partial| Slack_Tokens[Verify tokens<br/>& permissions]
    
    Sync_Issue --> Sync_Check{Data appearing<br/>in database?}
    Sync_Check -->|No| API_Keys[Check API keys<br/>Test TimeCamp access]
    Sync_Check -->|Yes| Sync_Outdated[Run full sync<br/>./oye full-sync]
    
    Gen_Issue --> Health_Check[Check health endpoint<br/>curl /health]
    Health_Check --> Logs[Check application logs<br/>for specific errors]
    
    DB_Schema --> Success[✅ Resolved]
    DB_Fix --> Success
    Slack_App --> Success
    Slack_Timeout --> Success
    Slack_Tokens --> Success
    API_Keys --> Success
    Sync_Outdated --> Success
    Logs --> Success
```

## 🚨 Common Issues & Solutions

### 🗄️ Database Connection Problems

#### Issue: "Failed to initialize database"
**Symptoms**:
```bash
Failed to initialize database: connection refused
ERROR: database connection failed
pq: password authentication failed
```

**Diagnostic Steps**:
```bash
# 1. Check database URL format
echo $DATABASE_URL
# Should be: postgresql://user:password@host:port/database

# 2. Test direct connection
psql $DATABASE_URL -c "SELECT 1;"

# 3. Check if database server is running
pg_isready -h localhost -p 5432

# 4. Verify credentials
psql -h localhost -U your_user -d your_db
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **Invalid URL Format** | Fix `DATABASE_URL` format: `postgresql://user:pass@host:port/db` |
| **Server Not Running** | Start PostgreSQL: `sudo systemctl start postgresql` |
| **Wrong Credentials** | Update username/password in environment |
| **Database Doesn't Exist** | Create database: `createdb your_database_name` |
| **Network/Firewall Issues** | Check firewall rules and network connectivity |

#### Issue: "Database tables not found"
**Symptoms**:
```bash
ERROR: relation "tasks" does not exist
ERROR: table doesn't exist
pq: column "id" does not exist
```

**Solution**:
```bash
# Initialize database schema
./oye --init-db

# Verify tables were created
psql $DATABASE_URL -c "\dt"
```

**Expected output**: List of tables including `tasks`, `time_entries`, `users`, etc.

---

### 💬 Slack Integration Issues

#### Issue: "Slash command not working"
**Symptoms**:
- `/oye` command not recognized in Slack
- "Unknown command" error
- No response when typing `/oye`

**Diagnostic Steps**:
```bash
# 1. Check app installation status
# Go to Slack workspace → Apps → Manage → Installed Apps

# 2. Test request URL accessibility
curl -X POST https://your-domain.com/slack/oye \
  -d "token=test&text=help"

# 3. Verify slash command configuration
# Slack App Dashboard → Slash Commands → /oye
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **App Not Installed** | Install/reinstall Slack app in workspace |
| **Wrong Request URL** | Update URL in Slack app: `https://your-domain.com/slack/oye` |
| **Command Not Created** | Create `/oye` slash command in Slack app dashboard |
| **Permissions Missing** | Add `commands` scope to bot permissions |

#### Issue: "Application did not respond"
**Symptoms**:
```
This app took too long to respond
Timeout error in Slack
/oye command hangs without response
```

**Diagnostic Steps**:
```bash
# 1. Check application health
curl https://your-domain.com/health

# 2. Test response time
time curl -X POST https://your-domain.com/slack/oye \
  -d "token=test&text=help"

# 3. Check application logs
./oye | grep -E "(ERROR|WARN|TIMEOUT)"
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **App Not Running** | Start application: `./oye` or restart service |
| **URL Not Accessible** | Check DNS, firewall, SSL certificate |
| **Database Slowness** | Optimize database queries, check indexes |
| **Network Issues** | Test network connectivity to external APIs |

#### Issue: "No response after acknowledgment"
**Symptoms**:
- Get "⏳ Processing..." message
- No final response received
- Commands seem to hang

**Diagnostic Steps**:
```bash
# 1. Test CLI command directly
./oye update daily

# 2. Check database connection
./oye --init-db

# 3. Verify environment variables
env | grep -E "(SLACK_|TIMECAMP_|DATABASE_)" | head -5

# 4. Check for async job processing errors
./oye | grep -E "(job|queue|async)"
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **Database Connection** | Fix `DATABASE_URL` and test connection |
| **Missing API Keys** | Set `TIMECAMP_API_KEY` and `SLACK_BOT_TOKEN` |
| **Job Processing Failure** | Check logs for specific error messages |
| **TimeCamp API Issues** | Verify API key and test TimeCamp connectivity |

---

### 🔄 Data Synchronization Issues

#### Issue: "No data syncing from TimeCamp"
**Symptoms**:
```bash
Sync completed but no data updated
Empty time entries after sync
0 tasks found during sync
```

**Diagnostic Steps**:
```bash
# 1. Test TimeCamp API directly
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/entries/format/json/from/2024-11-01/to/2024-11-30"

# 2. Check database contents
psql $DATABASE_URL -c "SELECT COUNT(*) FROM time_entries;"
psql $DATABASE_URL -c "SELECT COUNT(*) FROM tasks;"

# 3. Test sync with verbose output
./oye sync-tasks
./oye sync-time-entries
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **Invalid API Key** | Generate new API key in TimeCamp dashboard |
| **No TimeCamp Data** | Add time entries in TimeCamp first |
| **Date Range Issues** | Check sync date range parameters |
| **API Rate Limiting** | Wait 1 hour and retry, or contact TimeCamp support |

#### Issue: "Orphaned time entries"
**Symptoms**:
```bash
Found 150 orphaned time entries
Time entries without valid tasks
WARNING: Orphaned entries detected
```

**Solution**:
```bash
# Process orphaned entries
./oye process-orphaned

# Clean old orphaned entries (older than 30 days)
./oye cleanup-orphaned 30

# Re-sync tasks first, then time entries
./oye sync-tasks
./oye sync-time-entries
```

---

### 🔧 General Application Issues

#### Issue: "Application won't start"
**Symptoms**:
```bash
panic: runtime error
exit status 1
Error loading configuration
```

**Diagnostic Steps**:
```bash
# 1. Check binary integrity
./oye --version

# 2. Test minimal configuration
./oye --build-test

# 3. Check environment variables
env | grep -E "(DATABASE_|TIMECAMP_|SLACK_)"

# 4. Verify file permissions
ls -la oye-time-tracker
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **Binary Corruption** | Rebuild: `go build -o oye-time-tracker` |
| **Missing Environment** | Create `.env` file with required variables |
| **Permission Issues** | Fix permissions: `chmod +x oye-time-tracker` |
| **Port Already In Use** | Change `PORT` environment variable |

#### Issue: "Memory or performance issues"
**Symptoms**:
```bash
out of memory
slow response times
high CPU usage
```

**Diagnostic Steps**:
```bash
# Monitor resource usage
top -p $(pgrep oye)

# Check database connections
psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"

# Profile memory usage
go tool pprof http://localhost:8080/debug/pprof/heap
```

**Solutions**:
| Problem | Solution |
|---------|----------|
| **Memory Leaks** | Restart application regularly |
| **Database Connections** | Check connection pool settings |
| **Large Datasets** | Implement pagination for large queries |
| **Inefficient Queries** | Add database indexes, optimize SQL |

## 🔧 Advanced Troubleshooting

### Environment Variables Validation

```bash
# Check all required environment variables
cat > check_env.sh << 'EOF'
#!/bin/bash
echo "=== Environment Check ==="
echo "DATABASE_URL: ${DATABASE_URL:0:30}..."
echo "TIMECAMP_API_KEY: ${TIMECAMP_API_KEY:0:10}..."
echo "SLACK_BOT_TOKEN: ${SLACK_BOT_TOKEN:0:15}..."
echo "SLACK_VERIFICATION_TOKEN: ${SLACK_VERIFICATION_TOKEN:0:10}..."
echo "PORT: ${PORT:-8080}"
EOF

chmod +x check_env.sh
./check_env.sh
```

### Database Connection Testing

```bash
# Comprehensive database test
cat > test_db.sh << 'EOF'
#!/bin/bash
echo "=== Database Connection Test ==="
echo "Testing connection..."
psql $DATABASE_URL -c "SELECT version();" || exit 1

echo "Testing tables..."
psql $DATABASE_URL -c "\dt" || exit 1

echo "Testing data..."
psql $DATABASE_URL -c "SELECT COUNT(*) as total_tasks FROM tasks;"
psql $DATABASE_URL -c "SELECT COUNT(*) as total_entries FROM time_entries;"
psql $DATABASE_URL -c "SELECT COUNT(*) as total_users FROM users;"
EOF

chmod +x test_db.sh
./test_db.sh
```

### TimeCamp API Testing

```bash
# Test TimeCamp API connectivity
cat > test_timecamp.sh << 'EOF'
#!/bin/bash
echo "=== TimeCamp API Test ==="
echo "Testing users endpoint..."
curl -s -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/users/format/json" | jq '.[] | .display_name' | head -5

echo "Testing tasks endpoint..."
curl -s -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/tasks/format/json" | jq 'length'

echo "Testing entries endpoint..."
curl -s -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/entries/format/json/from/2024-11-01/to/2024-11-30" | jq 'length'
EOF

chmod +x test_timecamp.sh
./test_timecamp.sh
```

## 🔍 Debugging Tools

### Application Logs

```bash
# View live logs with filtering
./oye 2>&1 | grep -E "(ERROR|WARN|INFO)" --color=always

# Save logs to file
./oye > app.log 2>&1 &

# Monitor specific components
./oye 2>&1 | grep -E "(sync|slack|database)" --color=always
```

### Health Check Script

```bash
# Comprehensive health check
cat > health_check.sh << 'EOF'
#!/bin/bash
echo "=== OYE Health Check ==="

# Application health
echo "1. Application Health:"
curl -s http://localhost:8080/health || echo "❌ App not responding"

# Database health
echo "2. Database Health:"
./oye --init-db > /dev/null 2>&1 && echo "✅ Database OK" || echo "❌ Database issues"

# TimeCamp API health
echo "3. TimeCamp API Health:"
curl -s -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/users/format/json" > /dev/null \
  && echo "✅ TimeCamp OK" || echo "❌ TimeCamp issues"

# Slack integration health
echo "4. Slack Integration Health:"
curl -s -X POST http://localhost:8080/slack/oye \
  -d "token=test&text=help" > /dev/null \
  && echo "✅ Slack endpoint OK" || echo "❌ Slack endpoint issues"

echo "=== Health Check Complete ==="
EOF

chmod +x health_check.sh
./health_check.sh
```

## 🎯 Prevention & Best Practices

### Regular Maintenance

```bash
# Weekly maintenance script
cat > maintenance.sh << 'EOF'
#!/bin/bash
echo "=== Weekly Maintenance ==="

# Clean old orphaned entries
./oye cleanup-orphaned 30

# Full sync to ensure data consistency
./oye full-sync

# Check database size
psql $DATABASE_URL -c "SELECT pg_size_pretty(pg_database_size(current_database()));"

# Update user information
./oye sync-users

echo "=== Maintenance Complete ==="
EOF

chmod +x maintenance.sh
# Run weekly via cron: 0 6 * * 0 /path/to/maintenance.sh
```

### Monitoring Setup

```bash
# Create monitoring script
cat > monitor.sh << 'EOF'
#!/bin/bash
while true; do
  echo "$(date): Checking OYE health..."
  curl -s http://localhost:8080/health > /dev/null || {
    echo "$(date): OYE is down, attempting restart..."
    pkill -f oye-time-tracker
    ./oye-time-tracker &
  }
  sleep 300  # Check every 5 minutes
done
EOF

chmod +x monitor.sh
# Run in background: ./monitor.sh &
```

## 📞 Getting Additional Help

### Creating Support Tickets

When reporting issues, include:

1. **Environment Information**:
   ```bash
   # System info
   uname -a
   go version
   psql --version
   
   # Application info
   ./oye --version
   ```

2. **Error Messages**:
   ```bash
   # Full error output
   ./oye 2>&1 | head -50
   ```

3. **Configuration** (redacted):
   ```bash
   # Environment variables (remove sensitive values)
   env | grep -E "(DATABASE_|TIMECAMP_|SLACK_)" | sed 's/=.*/=***/'
   ```

### Useful Resources

| Resource | Purpose |
|----------|---------|
| [Installation Guide](INSTALLATION.md) | Complete setup instructions |
| [Architecture Documentation](ARCHITECTURE.md) | System design and components |
| [API Reference](API_REFERENCE.md) | Endpoint documentation |
| [CLI Commands](CLI_COMMANDS.md) | Command-line interface |

---

*This troubleshooting guide covers the most common issues. For complex problems, consider reviewing the [Architecture Documentation](ARCHITECTURE.md) to understand system components better.* 