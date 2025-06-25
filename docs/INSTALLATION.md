# Installation & Setup Guide

This guide covers setting up the Observe-Yor-Estimates project for development and production environments.

## 📋 Prerequisites

### System Requirements
- **Go**: Version 1.18 or higher
- **Database**: PostgreSQL 12+
- **Memory**: Minimum 512MB RAM
- **Storage**: 1GB free space

### External Services
- **TimeCamp Account**: With API access
- **Slack Workspace**: With admin permissions to create apps
- **PostgreSQL Database**: Local or hosted (e.g., Railway, AWS RDS)

## 🔧 Local Development Setup

### 1. Clone the Repository

```bash
git clone <repository-url>
cd observe-yor-estimates
```

### 2. Install Dependencies

```bash
# Download Go modules
go mod download

# Verify installation
go run . --version
```

### 3. Database Setup

#### Option A: Local PostgreSQL
```bash
# Install PostgreSQL (Ubuntu/Debian)
sudo apt update
sudo apt install postgresql postgresql-contrib

# Create database and user
sudo -u postgres psql
CREATE DATABASE your_database_name;
CREATE USER your_database_user WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE your_database_name TO your_database_user;
\q
```

#### Option B: Docker PostgreSQL
```bash
# Run PostgreSQL in Docker
docker run --name oye-postgres \
  -e POSTGRES_DB=your_database_name \
  -e POSTGRES_USER=your_database_user \
  -e POSTGRES_PASSWORD=your_secure_password \
  -p 5432:5432 \
  -d postgres:14
```

### 4. Environment Configuration

Create a `.env` file in the project root:

```bash
# Database Configuration
DATABASE_URL=postgresql://your_database_user:your_secure_password@localhost:5432/your_database_name

# TimeCamp API
TIMECAMP_API_KEY=your_timecamp_api_key_here

# Slack Configuration
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here

# Optional: Custom Schedules (cron format)
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"

# Development Settings
LOG_LEVEL=info
PORT=8080
```

### 5. Initialize Database

```bash
# Initialize database tables
go run . --init-db

# Verify setup
go run . --build-test
```

### 6. Test Installation

```bash
# Start the application
go run .

# In another terminal, test health endpoint
curl http://localhost:8080/health
```

## 🌐 Production Setup

### Option 1: Railway Deployment

#### 1. Prepare Repository
```bash
# Ensure Dockerfile exists (it should)
ls Dockerfile

# Commit any changes
git add .
git commit -m "Prepare for deployment"
git push origin main
```

#### 2. Deploy to Railway
1. Visit [Railway.app](https://railway.app)
2. Click "New Project"
3. Select "Deploy from GitHub repo"
4. Choose your repository
5. Railway will auto-detect the Dockerfile

#### 3. Configure Environment Variables
In Railway dashboard, add environment variables:
- All variables from your `.env` file
- `DATABASE_URL` (from Railway PostgreSQL service)

#### 4. Add PostgreSQL Service
1. In Railway project, click "New Service"
2. Select "PostgreSQL"
3. Copy the connection URL to `DATABASE_URL`

### Option 2: Docker Deployment

#### 1. Build Image
```bash
# Build the Docker image
docker build -t observe-yor-estimates .

# Test locally
docker run -p 8080:8080 --env-file .env observe-yor-estimates
```

#### 2. Deploy to Server
```bash
# Save image
docker save observe-yor-estimates | gzip > oye-app.tar.gz

# Transfer to server and load
scp oye-app.tar.gz user@server:/tmp/
ssh user@server
docker load < /tmp/oye-app.tar.gz

# Run with environment file
docker run -d \
  --name oye-app \
  -p 8080:8080 \
  --env-file .env \
  --restart unless-stopped \
  observe-yor-estimates
```

### Option 3: Binary Deployment

#### 1. Build Binary
```bash
# Build for Linux (if cross-compiling)
GOOS=linux GOARCH=amd64 go build -o oye-time-tracker

# For local system
go build -o oye-time-tracker
```

#### 2. Create Service File
Create `/etc/systemd/system/oye.service`:

```ini
[Unit]
Description=Observe-Yor-Estimates Service
After=network.target

[Service]
Type=simple
User=oye
WorkingDirectory=/opt/oye
ExecStart=/opt/oye/oye-time-tracker
Restart=always
RestartSec=5
EnvironmentFile=/opt/oye/.env

[Install]
WantedBy=multi-user.target
```

#### 3. Deploy and Start
```bash
# Create user and directories
sudo useradd -r -s /bin/false oye
sudo mkdir -p /opt/oye
sudo chown oye:oye /opt/oye

# Copy files
sudo cp oye-time-tracker /opt/oye/
sudo cp .env /opt/oye/
sudo chown oye:oye /opt/oye/*

# Enable and start service
sudo systemctl enable oye
sudo systemctl start oye
sudo systemctl status oye
```

## 🔑 API Keys Setup

### TimeCamp API Key
1. Log in to TimeCamp
2. Go to Account Settings > Add-ons
3. Find "API" section
4. Generate or copy your API key

### Slack Bot Setup
1. Visit [Slack API](https://api.slack.com/apps)
2. Click "Create New App"
3. Choose "From scratch"
4. Enter app name and workspace
5. Go to "OAuth & Permissions"
6. Add bot token scopes:
   - `chat:write`
   - `commands`
7. Install app to workspace
8. Copy Bot User OAuth Token (starts with `xoxb-`)

### Slack Slash Command
1. In your Slack app, go to "Slash Commands"
2. Click "Create New Command"
3. Set command: `/oye`
4. Set request URL: `https://your-domain.com/slack/oye`
5. Set description: "OYE time tracking commands"
6. Save and reinstall app

## ✅ Verification

### 1. Health Check
```bash
curl https://your-domain.com/health
# Expected: {"status":"healthy"}
```

### 2. Database Connection
```bash
./oye-time-tracker --init-db
# Should complete without errors
```

### 3. Slack Integration
In Slack, try:
```
/oye help
```
Should show help message.

### 4. Data Sync
```bash
./oye-time-tracker sync-tasks
./oye-time-tracker sync-time-entries
```

## 🔧 Troubleshooting

### Common Issues

#### Database Connection Failed
```bash
# Check database URL format
echo $DATABASE_URL
# Should be: postgresql://your_user:your_password@your_host:port/your_database

# Test connection directly
psql $DATABASE_URL -c "SELECT 1;"
```

#### Slack Verification Failed
- Check `SLACK_VERIFICATION_TOKEN` matches Slack app
- Ensure request URL is accessible from internet
- Verify Slack app is installed to workspace

#### TimeCamp API Errors
```bash
# Test API key
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  https://www.timecamp.com/third_party/api/users/format/json
```

#### Permission Denied
```bash
# Check file permissions
ls -la oye-time-tracker
chmod +x oye-time-tracker
```

### Debug Mode
```bash
# Enable detailed logging
export LOG_LEVEL=debug
go run .
```

## 📖 Next Steps

- [Configuration Guide](CONFIGURATION.md) - Detailed configuration options
- [Slack Integration](SLACK_INTEGRATION.md) - Setting up Slack features
- [User Management](USER_MANAGEMENT.md) - Adding and managing users
- [Development Guide](DEVELOPMENT.md) - Contributing to the project 