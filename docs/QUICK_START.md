# Quick Start Guide

Get OYE (Observe-Yor-Estimates) up and running in under 10 minutes.

## ⚡ Prerequisites

Before starting, ensure you have:
- [x] Go 1.18+ installed
- [x] PostgreSQL database access (local or hosted)
- [x] TimeCamp account with API access
- [x] Slack workspace admin access

## 🚀 5-Minute Local Setup

### 1. Clone and Setup (2 minutes)

```bash
# Clone the repository
git clone <your-repo-url>
cd observe-yor-estimates

# Install dependencies
go mod download

# Verify installation
go run . --version
```

### 2. Database Setup (1 minute)

**Option A: Docker (Fastest)**
```bash
docker run --name oye-postgres \
  -e POSTGRES_DB=oye_db \
  -e POSTGRES_USER=oye_user \
  -e POSTGRES_PASSWORD=oye_pass \
  -p 5432:5432 \
  -d postgres:14
```

**Option B: Local PostgreSQL**
```bash
createdb oye_db
```

### 3. Environment Configuration (1 minute)

Create `.env` file:
```bash
cat > .env << EOF
DATABASE_URL=postgresql://your_user:your_password@localhost:5432/your_database
TIMECAMP_API_KEY=your_timecamp_api_key_here
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here
EOF
```

### 4. Initialize and Test (1 minute)

```bash
# Initialize database
go run . --init-db

# Test the application
go run . --build-test

# Start the server
go run .
```

**✅ Success Indicators:**
- Database initialized without errors
- Build test passes
- Server starts on port 8080
- Health check responds: `curl localhost:8080/health`

## 🌐 10-Minute Production Setup

### Quick Railway Deployment

1. **Fork Repository** (30 seconds)
   - Fork this repository to your GitHub account

2. **Deploy to Railway** (2 minutes)
   - Visit [railway.app](https://railway.app)
   - Click "New Project" → "Deploy from GitHub"
   - Select your forked repository
   - Railway auto-detects Dockerfile

3. **Add Database** (1 minute)
   - In Railway project: "New Service" → "PostgreSQL"
   - Copy the connection string

4. **Configure Environment** (2 minutes)
   ```
   DATABASE_URL=<your_railway_postgres_url>
   TIMECAMP_API_KEY=<your_timecamp_api_key>
   SLACK_BOT_TOKEN=<your_slack_bot_token>
   SLACK_VERIFICATION_TOKEN=<your_slack_verification_token>
   ```

5. **Get Public URL** (30 seconds)
   - Railway project → Settings → Generate Domain
   - Note your URL: `https://yourapp.up.railway.app`

6. **Configure Slack** (4 minutes)
   - Create Slack App at [api.slack.com](https://api.slack.com/apps)
   - Add slash command `/oye` → `https://yourapp.up.railway.app/slack/oye`
   - Install app to workspace
   - Copy tokens to Railway environment

## 🔑 API Keys - Quick Setup

### TimeCamp API Key (1 minute)
1. Login to [TimeCamp](https://www.timecamp.com)
2. Account Settings → Add-ons → API
3. Copy the API key

### Slack App Setup (3 minutes)
1. Visit [Slack API](https://api.slack.com/apps) → "Create New App"
2. Choose "From scratch" → Enter name and workspace
3. **OAuth & Permissions**:
   - Add scopes: `chat:write`, `commands`
   - Install to workspace → Copy Bot Token (`xoxb-...`)
4. **Slash Commands**:
   - Create command `/oye`
   - Request URL: `https://your-domain.com/slack/oye`
5. **Basic Information** → Copy Verification Token

## ✅ Quick Verification

### Test Database Connection
```bash
./oye --init-db
# Should complete without errors
```

### Test TimeCamp Sync
```bash
./oye sync-tasks
# Should fetch and store tasks
```

### Test Slack Integration
In Slack:
```
/oye help
# Should show help message
```

## 📊 First Commands to Try

### Get Your First Report
```
/oye daily          # Private daily summary
/oye weekly public  # Public weekly report
```

### Sync Your Data
```
/oye sync          # Full data synchronization
```

### Monitor Estimates
```
/oye over 80 weekly  # Tasks over 80% of estimate
```

## 🔧 Quick Troubleshooting

### Command Not Working?
```bash
# Check health
curl https://your-domain.com/health

# Verify environment
echo $SLACK_BOT_TOKEN | cut -c1-10  # Should show: xoxb-12345
echo $DATABASE_URL | cut -d@ -f2     # Should show: host:port/db
```

### No Data in Reports?
```bash
# Force sync
./oye full-sync

# Check data
./oye list-users
./oye active-users
```

### Slack App Issues?
1. Ensure app is installed to workspace
2. Verify request URL is accessible
3. Check slash command configuration
4. Reinstall app after changes

## 🎯 Next Steps

Once everything is working:

1. **Add Team Members**: [User Management Guide](USER_MANAGEMENT.md)
2. **Configure Schedules**: [Scheduled Tasks](SCHEDULED_TASKS.md)  
3. **Customize Reports**: [Configuration Guide](CONFIGURATION.md)
4. **Set Up Monitoring**: [Troubleshooting Guide](TROUBLESHOOTING.md)

## 🆘 Need Help?

**Quick Fixes:**
- Database issues → Check `DATABASE_URL` format
- Slack issues → Verify tokens and app installation
- API issues → Test keys with curl commands

**Detailed Help:**
- [Installation Guide](INSTALLATION.md) - Complete setup instructions
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
- [CLI Commands](CLI_COMMANDS.md) - All available commands

## 📱 Mobile-Friendly Commands

Once set up, you can use OYE from Slack mobile:

```
/oye                # Quick daily update
/oye weekly         # Weekly summary
/oye sync           # Refresh data
/oye over 100 daily # Check overruns
```

Perfect for checking project status on the go! 📲 