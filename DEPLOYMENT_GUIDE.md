# Deployment Guide - Unified OYE System

This guide covers deploying the **completely redesigned OYE system** with context-aware Slack responses and unified command interface.

**⚠️ Breaking Changes**: This version removes all legacy endpoints. Only the new `/oye` command is supported.

## 🎯 Architecture Summary

**Problem Solved**: The previous Go server architecture suffered from timeout issues on Netlify (10-15 second limits) when operations took 30-60 seconds.

**Solution**: Hybrid approach with:
- **Netlify JS Functions**: Handle HTTP requests instantly (< 1 second response)
- **Go CLI Tool**: Process data in background, send results via webhooks
- **Asynchronous Processing**: No more timeouts, better user experience

## 🚀 Deployment Steps

### 1. Repository Setup

Connect your repository to Netlify:

```bash
# Clone the repository
git clone <your-repo-url>
cd observe-yor-estimates

# Install Node.js dependencies
npm install
```

### 2. Environment Variables

Set these variables in **Netlify Dashboard → Site Settings → Environment Variables**:

#### Required Variables
```bash
# Database (PostgreSQL)
DATABASE_URL=postgresql://username:password@host:port/database

# TimeCamp API
TIMECAMP_API_KEY=your_timecamp_api_key

# Slack Integration  
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_slack_verification_token
```

#### Optional Variables
```bash
# API Endpoints (use defaults if not specified)
TIMECAMP_API_URL=https://app.timecamp.com/third_party/api

# Sync Schedules (cron format)
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"  
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"

# Development
NODE_ENV=production
```

### 3. Build Configuration

The `netlify.toml` file is already configured:

```toml
[build]
  command = "npm run build"
  publish = "."
  functions = "netlify/functions"

[build.environment]
  NODE_VERSION = "18"
  GO_VERSION = "1.22"

[functions]
  node_bundler = "esbuild"
```

### 4. Deploy to Netlify

#### Option A: Automatic Deployment (Recommended)

1. **Connect Repository**: Link your GitHub/GitLab repository to Netlify
2. **Configure Build**: Use the existing `netlify.toml` configuration
3. **Set Environment Variables**: Add all required variables in Netlify dashboard
4. **Deploy**: Push to main branch or deploy manually

#### Option B: Manual Deployment

```bash
# Install Netlify CLI
npm install -g netlify-cli

# Login to Netlify
netlify login

# Deploy
netlify deploy --prod
```

### 5. Database Setup

Initialize your PostgreSQL database:

```bash
# Test database connection locally first
export DATABASE_URL="your_postgresql_url"
./bin/observe-yor-estimates sync-tasks

# Run initial sync
./bin/observe-yor-estimates full-sync
```

## 🔧 Function Endpoints

After deployment, your functions will be available at:

### Slack Command Endpoint
```bash
# Unified OYE command (replaces all old commands)
/.netlify/functions/slack-oye
```

### Manual API Endpoints
```bash
# Health check
/.netlify/functions/health

# Manual sync operations
/.netlify/functions/sync-tasks
/.netlify/functions/sync-time-entries

# Scheduled operations
/.netlify/functions/scheduled-sync?type=task-sync
/.netlify/functions/scheduled-sync?type=daily-update
```

## 📱 Slack App Configuration

### Update Slack App Settings

**⚠️ Delete ALL old slash commands and create a single new one:**

```bash
# Replace YOUR_SITE with your Netlify site name
https://YOUR_SITE.netlify.app/.netlify/functions/slack-oye
```

### Slash Command Setup

| Command | URL | Description |
|---------|-----|-------------|
| `/oye` | `/.netlify/functions/slack-oye` | All updates, syncing, and configuration |

**Delete these old commands:** `/daily-update`, `/weekly-update`, `/monthly-update`, `/full-sync`

## ⏰ Scheduled Tasks Setup

To run automated syncs, use one of these methods:

### Option 1: External Cron Service (Recommended)

Use a service like **Uptime Robot**, **Cronitor**, or **cron-job.org**:

```bash
# Task sync every 5 minutes
GET https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=task-sync

# Time entries sync every 10 minutes
GET https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=time-entries-sync

# Daily update at 6 AM  
GET https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=daily-update

# Weekly update on Mondays at 8 AM
GET https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=weekly-update

# Monthly update on 1st at 9 AM
GET https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=monthly-update
```

### Option 2: GitHub Actions (Alternative)

Create `.github/workflows/scheduled-sync.yml`:

```yaml
name: Scheduled Sync
on:
  schedule:
    - cron: '*/5 * * * *'  # Every 5 minutes - task sync
    - cron: '*/10 * * * *' # Every 10 minutes - time entries
    - cron: '0 6 * * *'    # Daily at 6 AM
    - cron: '0 8 * * 1'    # Weekly on Monday at 8 AM
    - cron: '0 9 1 * *'    # Monthly on 1st at 9 AM

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - name: Trigger Sync
        run: |
          curl "https://YOUR_SITE.netlify.app/.netlify/functions/scheduled-sync?type=task-sync"
```

## 🧪 Testing Your Deployment

### 1. Health Check
```bash
curl https://YOUR_SITE.netlify.app/.netlify/functions/health
```

Expected response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-20T10:30:00Z",
  "version": "2.0.0",
  "architecture": "netlify-functions-go-cli"
}
```

### 2. Test Sync Operations
```bash
# Test task sync
curl -X POST https://YOUR_SITE.netlify.app/.netlify/functions/sync-tasks

# Test time entries sync  
curl -X POST https://YOUR_SITE.netlify.app/.netlify/functions/sync-time-entries
```

### 3. Test Slack Commands

Use your Slack workspace:
```
/daily-update daily
/full-sync
```

Expected immediate response:
```
⏳ Preparing your daily update... I'll send the results shortly!
```

## 📊 Monitoring & Debugging

### Function Logs

View logs in **Netlify Dashboard → Functions → Function name → Logs**

### Success Patterns
```bash
"Successfully completed daily update"
"Task synchronization completed successfully"
"Scheduled task-sync completed successfully"
```

### Error Patterns
```bash
"Failed to queue job"
"Database connection failed"
"Go CLI execution failed"
```

### Common Issues & Solutions

**"Go binary not found"**
```bash
# Check if build completed successfully
# Look for bin/observe-yor-estimates in deploy logs
```

**"Database connection timeout"**
```bash
# Verify DATABASE_URL format
# Check PostgreSQL server accessibility
```

**"Slack webhook failed"**
```bash
# Verify SLACK_WEBHOOK_URL is correct
# Check webhook permissions in Slack
```

## 🔄 Rollback Strategy

If deployment fails:

### Quick Rollback
1. **Netlify Dashboard → Deploys → Previous Deploy → Publish**
2. Or use CLI: `netlify rollback`

### Environment Issues
1. Check **Site Settings → Environment Variables**
2. Compare with working environment
3. Redeploy after fixing variables

## 📈 Performance Expectations

### Response Times
- **Slack Commands**: < 1 second (immediate response)
- **Background Processing**: 10-60 seconds (depending on data volume)
- **Sync Operations**: 5-30 seconds (via manual endpoints)

### Function Execution Limits
- **Netlify Free**: 125,000 function invocations/month
- **Netlify Pro**: 2,000,000 function invocations/month
- **Individual Function**: 10 second timeout (sufficient for instant responses)

## 🚀 Production Optimization

### Database Connection Pooling
Consider using connection pooling for high-traffic scenarios:

```bash
# Add to environment variables
DATABASE_MAX_CONNECTIONS=20
DATABASE_CONNECTION_LIFETIME=300
```

### Caching Strategy
Implement caching for frequently accessed data:

```bash
# Add Redis URL for caching (optional)
REDIS_URL=redis://your-redis-instance
```

### Error Alerting
Set up alerts for critical errors:

1. **Slack webhook failures** → Send admin notifications
2. **Database connection issues** → Email alerts  
3. **Sync operation failures** → Dashboard monitoring

## 🎉 Migration Complete!

Your application is now running with the new **timeout-resistant architecture**:

✅ **No more Slack command timeouts**  
✅ **Instant user feedback**  
✅ **Reliable background processing**  
✅ **Better error handling**  
✅ **Easier monitoring and debugging**  

Users will immediately notice the improved experience:
- Commands respond in < 1 second
- Clear progress indicators  
- Reliable delivery of final results

---

**Need help?** Check the function logs in Netlify dashboard for detailed error information. 