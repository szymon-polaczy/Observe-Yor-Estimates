# Deployment Guide

This guide covers deploying the Observe-Yor-Estimates (OYE) system to production environments with step-by-step workflows and best practices.

## 🎯 Deployment Overview

### Architecture Summary

**OYE is a self-contained Go server that:**
- 🌐 Handles all HTTP requests for Slack slash commands
- ⏰ Includes internal cron scheduler for periodic tasks
- 🔄 Manages data synchronization and report generation
- 📦 Deploys as a single Docker container

This monolithic architecture simplifies deployment and eliminates serverless complexities.

### 🚀 Deployment Workflow

```mermaid
graph TD
    Start([Start Deployment]) --> Fork[📂 Fork Repository]
    Fork --> Platform{Choose Platform}
    
    Platform -->|Railway| Railway[🚂 Railway Setup]
    Platform -->|Docker| Docker[🐳 Docker Setup]
    Platform -->|Binary| Binary[⚙️ Binary Setup]
    
    Railway --> R1[1. Create Railway Project]
    R1 --> R2[2. Connect GitHub Repo]
    R2 --> R3[3. Add PostgreSQL Service]
    R3 --> R4[4. Configure Environment]
    R4 --> R5[5. Generate Domain]
    R5 --> Slack[💬 Configure Slack App]
    
    Docker --> D1[1. Build Docker Image]
    D1 --> D2[2. Set up Database]
    D2 --> D3[3. Configure Environment]
    D3 --> D4[4. Deploy Container]
    D4 --> D5[5. Set up Reverse Proxy]
    D5 --> Slack
    
    Binary --> B1[1. Build Binary]
    B1 --> B2[2. Set up Server]
    B2 --> B3[3. Configure Database]
    B3 --> B4[4. Create System Service]
    B4 --> B5[5. Configure SSL/Domain]
    B5 --> Slack
    
    Slack --> Test[✅ Test Deployment]
    Test --> Monitor[📊 Monitor & Maintain]
    
    style Start fill:#e1f5fe
    style Test fill:#c8e6c9
    style Monitor fill:#fff3e0
```

## 🚂 Railway Deployment (Recommended)

Railway provides the simplest deployment experience with automatic Docker detection and managed PostgreSQL.

### Step 1: Fork Repository (2 minutes)

1. 🍴 Fork this repository to your GitHub account
2. 📝 Clone your fork locally (optional for customization)

### Step 2: Create Railway Project (3 minutes)

1. 🌐 Go to [Railway Dashboard](https://railway.app)
2. 📂 Click **"New Project"**
3. 🔗 Select **"Deploy from GitHub repo"**
4. 🎯 Choose your forked repository
5. 🔧 Railway auto-detects `Dockerfile` and starts building

### Step 3: Add PostgreSQL Database (2 minutes)

1. 🗄️ In Railway project: **"New Service"** → **"PostgreSQL"**
2. 📋 Copy the **PostgreSQL connection URL**
3. 💾 Note it for environment configuration

### Step 4: Configure Environment Variables (5 minutes)

Navigate to **"Variables"** tab and add:

#### 🔑 Required Variables
```bash
# Database Configuration
DATABASE_URL=postgresql://user:pass@host:port/db  # From Railway PostgreSQL

# TimeCamp Integration
TIMECAMP_API_KEY=your_timecamp_api_key_here

# Slack Integration  
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here
```

#### ⏰ Optional Schedule Overrides
```bash
# Sync Schedules (cron format)
TASK_SYNC_SCHEDULE="*/5 * * * *"           # Every 5 minutes
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"  # Every 10 minutes
DAILY_UPDATE_SCHEDULE="0 6 * * *"          # 6 AM daily
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"         # 8 AM Monday
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"        # 9 AM 1st of month
```

### Step 5: Generate Public Domain (1 minute)

1. 🌐 Go to **"Settings"** → **"Networking"**
2. 📝 Click **"Generate Domain"**
3. 📋 Copy the URL: `https://your-project.up.railway.app`

### Step 6: Configure Slack App (5 minutes)

1. 🔗 Visit [Slack API Dashboard](https://api.slack.com/apps)
2. 📝 Find your existing Slack app (or create new)
3. 💬 Go to **"Slash Commands"** → Edit `/oye`
4. 🔧 Update **Request URL**: `https://your-project.up.railway.app/slack/oye`
5. 💾 Save and reinstall app

## 🐳 Docker Deployment

For custom server environments or more control over the deployment.

### Prerequisites
- Docker installed on target server
- PostgreSQL database (local or hosted)
- Domain name with SSL certificate

### Step 1: Build Image (5 minutes)

```bash
# Clone repository
git clone <your-fork-url>
cd observe-yor-estimates

# Build Docker image
docker build -t oye-app:latest .

# Test locally (optional)
docker run -p 8080:8080 --env-file .env oye-app:latest
```

### Step 2: Set Up Database (10 minutes)

**Option A: Docker PostgreSQL**
```bash
# Run PostgreSQL container
docker run -d \
  --name oye-postgres \
  -e POSTGRES_DB=oye_db \
  -e POSTGRES_USER=oye_user \
  -e POSTGRES_PASSWORD=secure_password \
  -v oye_data:/var/lib/postgresql/data \
  -p 5432:5432 \
  postgres:14
```

**Option B: Managed Database**
```bash
# Use hosted PostgreSQL (AWS RDS, Google Cloud SQL, etc.)
# Copy connection string for environment configuration
```

### Step 3: Configure Environment (5 minutes)

Create production `.env` file:
```bash
cat > .env.production << 'EOF'
DATABASE_URL=postgresql://oye_user:secure_password@localhost:5432/oye_db
TIMECAMP_API_KEY=your_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_verification_token
PORT=8080
EOF
```

### Step 4: Deploy Container (5 minutes)

```bash
# Run OYE application
docker run -d \
  --name oye-app \
  --env-file .env.production \
  -p 8080:8080 \
  --restart unless-stopped \
  --link oye-postgres:postgres \
  oye-app:latest
```

### Step 5: Set Up Reverse Proxy (10 minutes)

**Nginx Configuration**:
```nginx
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl;
    server_name your-domain.com;
    
    ssl_certificate /path/to/certificate.crt;
    ssl_certificate_key /path/to/private.key;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## ⚙️ Binary Deployment

For direct server deployment without Docker.

### Prerequisites
- Linux server with Go 1.18+
- PostgreSQL database
- Systemd for service management

### Step 1: Build Binary (5 minutes)

```bash
# On development machine
git clone <your-fork-url>
cd observe-yor-estimates

# Build for Linux (if cross-compiling)
GOOS=linux GOARCH=amd64 go build -o oye-time-tracker

# Or build on target server
go build -o oye-time-tracker
```

### Step 2: Set Up Server (10 minutes)

```bash
# Create user and directories
sudo useradd -r -s /bin/false oye
sudo mkdir -p /opt/oye
sudo chown oye:oye /opt/oye

# Copy files
sudo cp oye-time-tracker /opt/oye/
sudo cp .env /opt/oye/
sudo chown oye:oye /opt/oye/*
sudo chmod +x /opt/oye/oye-time-tracker
```

### Step 3: Create System Service (10 minutes)

Create `/etc/systemd/system/oye.service`:
```ini
[Unit]
Description=Observe-Yor-Estimates Service
After=network.target postgresql.service

[Service]
Type=simple
User=oye
WorkingDirectory=/opt/oye
ExecStart=/opt/oye/oye-time-tracker
Restart=always
RestartSec=5
EnvironmentFile=/opt/oye/.env

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/oye

[Install]
WantedBy=multi-user.target
```

### Step 4: Start Service (5 minutes)

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable oye
sudo systemctl start oye

# Check status
sudo systemctl status oye
sudo journalctl -u oye -f
```

## ✅ Testing Your Deployment

### 🩺 Health Check
```bash
# Test application health
curl https://your-domain.com/health

# Expected response:
{"status":"healthy"}
```

### 📊 Database Verification
```bash
# Initialize database (if needed)
./oye-time-tracker --init-db

# Test data sync
./oye-time-tracker sync-tasks
./oye-time-tracker sync-time-entries
```

### 💬 Slack Integration Test
In your Slack workspace:
```bash
/oye help           # Should show command help
/oye daily          # Should generate daily report
/oye sync           # Should trigger data sync
```

## 📊 Monitoring & Maintenance

### 🔍 Application Monitoring

**Health Monitoring Script**:
```bash
#!/bin/bash
# monitor.sh - Simple health check script

while true; do
  if ! curl -s https://your-domain.com/health > /dev/null; then
    echo "$(date): OYE health check failed, sending alert..."
    # Add your alert mechanism here (email, Slack, etc.)
  fi
  sleep 300  # Check every 5 minutes
done
```

### 📋 Log Management

**View Logs**:
```bash
# Railway: Check Railway dashboard logs
# Docker: docker logs oye-app
# Systemd: journalctl -u oye -f
```

**Log Rotation** (for binary deployment):
```bash
# Add to /etc/logrotate.d/oye
/opt/oye/logs/*.log {
    daily
    missingok
    rotate 7
    compress
    notifempty
    create 644 oye oye
    postrotate
        systemctl reload oye
    endscript
}
```

### 🔧 Maintenance Tasks

**Weekly Maintenance Script**:
```bash
#!/bin/bash
# maintenance.sh

echo "=== OYE Weekly Maintenance ==="

# Health check
curl -s https://your-domain.com/health || exit 1

# Full data sync
./oye-time-tracker full-sync

# Clean orphaned entries
./oye-time-tracker cleanup-orphaned 30

# Database size check
psql $DATABASE_URL -c "SELECT pg_size_pretty(pg_database_size(current_database()));"

echo "=== Maintenance Complete ==="
```

## 🆘 Troubleshooting

### Common Deployment Issues

| Issue | Symptoms | Solution |
|-------|----------|----------|
| **App won't start** | Build fails or crashes | Check environment variables and logs |
| **Database connection** | Connection refused errors | Verify `DATABASE_URL` format and accessibility |
| **Slack not responding** | Timeout errors | Check domain accessibility and SSL |
| **High memory usage** | App crashes or slow response | Add resource limits, optimize queries |

### 🔧 Debug Commands

```bash
# Check environment
env | grep -E "(DATABASE_|TIMECAMP_|SLACK_)"

# Test database connection
psql $DATABASE_URL -c "SELECT 1;"

# Test TimeCamp API
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  "https://www.timecamp.com/third_party/api/users/format/json"

# Monitor resource usage
top -p $(pgrep oye)
```

## 🔒 Security Best Practices

### Environment Security
- ✅ Use environment variables for all secrets
- ✅ Enable SSL/TLS for all connections
- ✅ Regular security updates for dependencies
- ✅ Implement proper firewall rules
- ✅ Use non-root user for application

### Access Control
- ✅ Limit database permissions to required operations
- ✅ Use strong passwords for all accounts
- ✅ Enable two-factor authentication where possible
- ✅ Regular audit of access logs

## 📚 Related Documentation

- [Quick Start Guide](QUICK_START.md) - Local development setup
- [Installation Guide](INSTALLATION.md) - Detailed installation instructions
- [Slack Integration](SLACK_INTEGRATION.md) - Slack app configuration
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions

---

*This deployment guide covers production deployment scenarios. For development setup, see the [Quick Start Guide](QUICK_START.md).* 