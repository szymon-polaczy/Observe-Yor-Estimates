# Quick Start Guide

Get OYE (Observe-Yor-Estimates) up and running in under 10 minutes with our streamlined setup process.

## 🗺️ Setup Workflow Overview

```mermaid
graph TD
    Start([Start Setup]) --> Choice{Development or<br/>Production?}
    
    Choice -->|Development| DevPath[Development Setup]
    Choice -->|Production| ProdPath[Production Setup]
    
    DevPath --> D1[1. Clone Repository<br/>git clone repo]
    D1 --> D2[2. Install Dependencies<br/>go mod download]
    D2 --> D3[3. Setup Database<br/>PostgreSQL local/Docker]
    D3 --> D4[4. Configure Environment<br/>Create .env file]
    D4 --> D5[5. Initialize Database<br/>./oye --init-db]
    D5 --> D6[6. Test Installation<br/>./oye --build-test]
    D6 --> DevReady[✅ Ready for Development]
    
    ProdPath --> P1[1. Choose Platform<br/>Railway/Docker/Binary]
    P1 --> P2[2. Configure Database<br/>PostgreSQL hosted]
    P2 --> P3[3. Set Environment<br/>Production variables]
    P3 --> P4[4. Deploy Application<br/>Build & deploy]
    P4 --> P5[5. Initialize Database<br/>./oye --init-db]
    P5 --> P6[6. Configure Slack<br/>Create app & commands]
    P6 --> ProdReady[✅ Ready for Production]
    
    DevReady --> API[Get API Keys<br/>TimeCamp & Slack]
    ProdReady --> API
    API --> Final[🎉 Complete Setup]
```

## ⚡ Prerequisites

Before starting, ensure you have:

| Component | Version | Purpose |
|-----------|---------|---------|
| **Go** | 1.18+ | Application runtime |
| **PostgreSQL** | 12+ | Database storage |
| **TimeCamp Account** | Active | API access for time tracking |
| **Slack Workspace** | Admin access | Bot installation permissions |

## 🚀 5-Minute Local Setup

### Step 1: Clone and Setup (2 minutes)

```bash
# Clone the repository
git clone <your-repo-url>
cd observe-yor-estimates

# Install dependencies
go mod download

# Verify installation
go run . --version
```

**✅ Success indicator**: Version number displays without errors

### Step 2: Database Setup (1 minute)

Choose your preferred database setup:

**Option A: Docker (Recommended)**
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
# Ubuntu/Debian
sudo apt install postgresql postgresql-contrib
createdb oye_db
```

**Option C: Use existing PostgreSQL**
```bash
# Just ensure you have connection details ready
psql -h your-host -U your-user -d your-db -c "SELECT 1;"
```

### Step 3: Environment Configuration (1 minute)

Create `.env` file with your configuration:

```bash
cat > .env << 'EOF'
# Database Configuration
DATABASE_URL=postgresql://oye_user:oye_pass@localhost:5432/oye_db

# TimeCamp API (get from TimeCamp dashboard)
TIMECAMP_API_KEY=your_timecamp_api_key_here

# Slack Configuration (get from Slack app dashboard)
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here

# Optional: Custom port
PORT=8080
EOF
```

### Step 4: Initialize and Test (1 minute)

```bash
# Initialize database tables
go run . --init-db

# Test the application
go run . --build-test

# Start the server
go run .
```

**✅ Success indicators**:
- Database initialized: `Database initialized successfully`
- Build test passes: `Build test successful`
- Server starts: `Server listening on :8080`
- Health check works: `curl localhost:8080/health` returns `{"status":"healthy"}`

## 🌐 10-Minute Production Setup

### Option 1: Railway Deployment (Recommended)

#### Step 1: Prepare Repository (1 minute)
```bash
# Ensure your code is ready
git add .
git commit -m "Prepare for Railway deployment"
git push origin main
```

#### Step 2: Deploy to Railway (2 minutes)
1. 🌐 Visit [railway.app](https://railway.app)
2. 📂 Click **"New Project"** → **"Deploy from GitHub"**
3. 🔗 Select your forked repository
4. 🚀 Railway auto-detects Dockerfile and deploys

#### Step 3: Add PostgreSQL Database (1 minute)
1. 🗄️ In Railway project: **"New Service"** → **"PostgreSQL"**
2. 📋 Copy the connection string from the database service
3. 🔧 Add as `DATABASE_URL` environment variable

#### Step 4: Configure Environment Variables (2 minutes)
Add these in Railway dashboard under **"Variables"**:
```bash
DATABASE_URL=postgresql://postgres:password@host:5432/railway
TIMECAMP_API_KEY=your_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
SLACK_VERIFICATION_TOKEN=your_slack_verification_token
```

#### Step 5: Get Public URL (1 minute)
1. 🌐 Railway project → **"Settings"** → **"Generate Domain"**
2. 📝 Note your URL: `https://yourapp.up.railway.app`

#### Step 6: Configure Slack Integration (3 minutes)
1. 🔗 Visit [Slack API](https://api.slack.com/apps) → **"Create New App"**
2. ⚙️ Add slash command `/oye` → `https://yourapp.up.railway.app/slack/oye`
3. 🔑 Install app to workspace and copy tokens
4. 🔄 Update Railway environment variables with actual tokens

### Option 2: Docker Deployment

#### Build and Deploy
```bash
# Build the Docker image
docker build -t observe-yor-estimates .

# Run with environment file
docker run -d \
  --name oye-app \
  -p 8080:8080 \
  --env-file .env \
  --restart unless-stopped \
  observe-yor-estimates
```

### Option 3: Binary Deployment

#### Build and Deploy
```bash
# Build for your target platform
GOOS=linux GOARCH=amd64 go build -o oye-time-tracker

# Deploy to server
scp oye-time-tracker user@server:/opt/oye/
scp .env user@server:/opt/oye/

# Set up as system service (systemd example)
sudo systemctl enable oye
sudo systemctl start oye
```

## 🔑 API Keys - Quick Setup Guide

### TimeCamp API Key (2 minutes)

1. 🔐 Login to [TimeCamp](https://www.timecamp.com)
2. ⚙️ Go to **Account Settings** → **Add-ons** → **API**
3. 🗝️ Generate or copy your API key
4. 📋 Add to `.env` file as `TIMECAMP_API_KEY`

**Test your key:**
```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  "https://www.timecamp.com/third_party/api/users/format/json"
```

### Slack App Setup (4 minutes)

#### Create Slack App (2 minutes)
1. 🔗 Visit [Slack API](https://api.slack.com/apps) → **"Create New App"**
2. 📝 Choose **"From scratch"** → Enter app name and workspace
3. 🔧 Configure in **"OAuth & Permissions"**:
   - Add scopes: `chat:write`, `commands`
   - Install to workspace
   - Copy **Bot User OAuth Token** (starts with `xoxb-`)

#### Create Slash Command (2 minutes)
1. 💬 Go to **"Slash Commands"** → **"Create New Command"**
2. ⚙️ Configure:
   - **Command**: `/oye`
   - **Request URL**: `https://your-domain.com/slack/oye`
   - **Description**: "OYE time tracking commands"
   - **Usage Hint**: `[daily|weekly|monthly|sync|help] [public]`
3. 🔄 **Save** and **reinstall app** to activate

## ✅ Quick Verification Checklist

### Test Database Connection
```bash
./oye --init-db
# ✅ Should complete without errors
```

### Test TimeCamp Integration
```bash
./oye sync-tasks
# ✅ Should fetch and display task information
```

### Test Slack Integration
In your Slack workspace:
```
/oye help
# ✅ Should show help message
```

### Test Application Health
```bash
curl https://your-domain.com/health
# ✅ Should return: {"status":"healthy"}
```

## 📊 Your First Commands

### Get Time Reports
```bash
/oye daily          # Private daily summary
/oye weekly public  # Public weekly report to channel
/oye monthly        # Monthly overview
```

### Sync Your Data
```bash
/oye sync          # Quick data refresh
/oye full-sync     # Complete synchronization
```

### Monitor Project Health
```bash
/oye over 80 weekly   # Tasks over 80% of estimate
/oye over 100 daily   # Tasks over budget
```

## 🔧 Quick Troubleshooting

### Common Issues & Solutions

| Issue | Quick Fix |
|-------|-----------|
| **Database connection failed** | Check `DATABASE_URL` format |
| **Slack command not found** | Verify app installation & slash command |
| **No response from /oye** | Check app URL and health endpoint |
| **Empty reports** | Run `./oye full-sync` to populate data |
| **API errors** | Verify TimeCamp API key validity |

### Debug Commands
```bash
# Check environment
env | grep -E "(DATABASE_|TIMECAMP_|SLACK_)" | head -3

# Test components
./oye --build-test      # Application
./oye list-users        # Database
./oye sync-tasks        # TimeCamp API

# Monitor logs
./oye | grep -E "(ERROR|WARN|INFO)"
```

## 🎯 Next Steps

Once your setup is complete:

1. **👥 Add Team Members**: [User Management Guide](USER_MANAGEMENT.md)
2. **⏰ Configure Schedules**: [Scheduled Tasks Documentation](SCHEDULED_TASKS.md)
3. **📊 Customize Reports**: [Configuration Guide](CONFIGURATION.md)
4. **🔍 Set Up Monitoring**: [Troubleshooting Guide](TROUBLESHOOTING.md)

## 📱 Mobile-Friendly Usage

OYE works great from Slack mobile app:

```bash
/oye                # Quick daily update
/oye weekly         # Weekly team summary
/oye sync           # Refresh data
/oye over 100 daily # Check budget overruns
```

Perfect for checking project status on the go! 📲

## 🆘 Getting Help

**Quick Resources:**
- 📚 [Installation Guide](INSTALLATION.md) - Detailed setup instructions
- 🔧 [Troubleshooting](TROUBLESHOOTING.md) - Common issues & solutions
- 💻 [CLI Commands](CLI_COMMANDS.md) - All available commands
- 🏗️ [Architecture](ARCHITECTURE.md) - System design overview

**Need immediate help?** Check the [Troubleshooting Decision Tree](TROUBLESHOOTING.md#quick-diagnosis) for step-by-step issue resolution.

---

*This quick start guide is designed to get you productive immediately. For detailed configuration and advanced features, explore the full documentation.* 