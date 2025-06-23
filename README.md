# Observe-Yor-Estimates v2.0

A modern, serverless TimeCamp to Slack integration using **Netlify Functions + Go CLI** architecture.

## 🏗️ Architecture Overview

This project has been reorganized to use a **hybrid approach** that eliminates timeout issues:

- **Netlify JavaScript Functions**: Handle HTTP requests and provide instant responses
- **Go CLI Tool**: Performs data processing, API calls, and database operations
- **Background Processing**: Long operations run asynchronously with status updates

### Why This Architecture?

✅ **No Timeouts**: Netlify functions respond instantly  
✅ **Reliable Processing**: Go CLI handles heavy lifting  
✅ **Better UX**: Users get immediate feedback  
✅ **Scalable**: Each function is independent  
✅ **Maintainable**: Clear separation of concerns  

## 🚀 Quick Start

### Prerequisites

- Node.js 18+
- Go 1.22+
- PostgreSQL database
- Netlify account

### Installation

```bash
# Clone the repository
git clone <your-repo>
cd observe-yor-estimates

# Install dependencies
npm install

# Build the Go CLI tool
npm run build

# Set up environment variables (see below)
cp .env.example .env
```

### Environment Variables

Create a `.env` file with:

```bash
# Database Configuration
DATABASE_URL=postgresql://user:pass@host:port/dbname

# TimeCamp API
TIMECAMP_API_KEY=your_timecamp_api_key
TIMECAMP_API_URL=https://app.timecamp.com/third_party/api

# Slack Configuration
SLACK_WEBHOOK_URL=https://hooks.slack.com/your/webhook/url
SLACK_VERIFICATION_TOKEN=your_slack_verification_token

# Optional: Custom scheduling
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
```

## 📁 Project Structure

```
observe-yor-estimates/
├── netlify/functions/          # Netlify JavaScript functions
│   ├── slack-update.js        # Handle /daily-update commands
│   ├── slack-full-sync.js     # Handle /full-sync commands
│   ├── sync-tasks.js          # Manual task sync
│   ├── sync-time-entries.js   # Manual time entries sync
│   ├── scheduled-sync.js      # Automated syncing
│   └── health.js              # Health check
├── bin/                       # Built Go binaries
├── *.go                       # Go source code (CLI tool)
├── package.json               # Node.js dependencies
├── netlify.toml               # Netlify configuration
└── README.md                  # This file
```

## 🔧 Available Functions

### Slack Commands

| Endpoint | Description | Usage |
|----------|-------------|-------|
| `/slack/slack-update` | Daily/weekly/monthly updates | `/daily-update`, `/weekly-update` |
| `/slack/slack-full-sync` | Complete data synchronization | `/full-sync` |

### Manual Operations

| Endpoint | Description |
|----------|-------------|
| `/api/sync-tasks` | Sync tasks from TimeCamp |
| `/api/sync-time-entries` | Sync time entries |
| `/api/health` | Health check |

### Scheduled Operations

The `/api/scheduled-sync` function can be triggered with different types:

```bash
# Task sync (every 5 minutes)
curl "https://your-app.netlify.app/.netlify/functions/scheduled-sync?type=task-sync"

# Time entries sync (every 10 minutes)  
curl "https://your-app.netlify.app/.netlify/functions/scheduled-sync?type=time-entries-sync"

# Daily update (6 AM)
curl "https://your-app.netlify.app/.netlify/functions/scheduled-sync?type=daily-update"
```

## 🔄 How It Works

### Before (Problematic)
```
Slack → /daily-update → [30 seconds of work] → TIMEOUT ❌
```

### After (Fixed)
```
Slack → /daily-update → Immediate Response (⏳ Working...)
                            ↓
        Background → Go CLI → Final Response (✅ Done!)
```

### Example Flow

1. User runs `/daily-update` in Slack
2. `slack-update.js` function responds instantly: *"⏳ Preparing your daily update..."*
3. Function spawns Go CLI: `./bin/observe-yor-estimates update daily`
4. Go CLI processes data and sends final result to Slack
5. User receives: *"✅ Daily update completed!"*

## 🛠️ Development

### Local Development

```bash
# Build Go binary
npm run build

# Test Go CLI directly
./bin/observe-yor-estimates update daily
./bin/observe-yor-estimates sync-tasks
./bin/observe-yor-estimates full-sync

# Run Netlify dev server
npm run dev
```

### Testing Functions

```bash
# Test health check
curl http://localhost:8888/.netlify/functions/health

# Test sync operations
curl -X POST http://localhost:8888/.netlify/functions/sync-tasks

# Test Slack commands
curl -X POST http://localhost:8888/.netlify/functions/slack-update \
  -d "command=/daily-update&text=daily&response_url=https://hooks.slack.com/test"
```

## 🚀 Deployment

### Automatic Deployment

1. Connect your repository to Netlify
2. Set environment variables in Netlify dashboard
3. Deploy automatically on git push

### Manual Deployment

```bash
# Deploy to Netlify
npm run deploy
```

### Environment Variables in Netlify

Go to your Netlify dashboard → Site settings → Environment variables and add all the variables from your `.env` file.

## 📊 Monitoring

### Health Check

Monitor your deployment:
```bash
curl https://your-app.netlify.app/.netlify/functions/health
```

### Logs

- **Netlify Functions**: Check Netlify dashboard → Functions → Logs
- **Go CLI**: Logs are captured by the JavaScript functions
- **Background Jobs**: Monitor using webhook responses

### Success Indicators

Look for these patterns in logs:
```
✅ "Successfully completed daily update"
✅ "Task synchronization completed successfully"  
✅ "Scheduled task-sync completed successfully"
```

## 🔧 Go CLI Commands

The Go binary supports these commands:

```bash
# Updates (with Slack integration)
./observe-yor-estimates update daily
./observe-yor-estimates update weekly  
./observe-yor-estimates update monthly

# Sync operations
./observe-yor-estimates sync-tasks
./observe-yor-estimates sync-time-entries
./observe-yor-estimates full-sync

# Utilities
./observe-yor-estimates --help
./observe-yor-estimates --version
```

## 🔀 Migration from v1.0

If you're migrating from the old server-based architecture:

1. **No Slack app changes needed** - endpoints remain the same
2. **Update deployment** - Use new `netlify.toml` configuration  
3. **Test functions** - Verify all commands work with new architecture
4. **Monitor performance** - Should see dramatic improvement in response times

## 🆘 Troubleshooting

### Common Issues

**Function timeouts**: 
- Check if Go binary exists in `bin/` directory
- Verify environment variables are set correctly

**Database connection errors**:
- Ensure `DATABASE_URL` is correct
- Check PostgreSQL connectivity

**Slack integration issues**:
- Verify `SLACK_VERIFICATION_TOKEN` 
- Check webhook URL format

### Getting Help

Check function logs in Netlify dashboard for detailed error information.

## 📈 Benefits vs Previous Architecture

| Aspect | Old (Go Server) | New (JS Functions + Go CLI) |
|--------|----------------|------------------------------|
| **Response Time** | 30+ seconds | < 1 second |
| **Timeout Issues** | Frequent ❌ | None ✅ |
| **User Experience** | Poor | Excellent |
| **Scalability** | Limited | High |
| **Debugging** | Difficult | Easy |
| **Maintenance** | Complex | Simple |

---

This architecture provides the best of both worlds: **instant responses** from JavaScript functions and **reliable data processing** from Go CLI tools! 🎉
