# Observe-Yor-Estimates

A powerful TimeCamp to Slack integration that provides real-time time tracking insights and automated reporting for your team.

## 🎯 What is OYE?

OYE (Observe-Yor-Estimates) is a monolithic Go application that bridges TimeCamp time tracking with Slack notifications. It provides:

- **📊 Real-time reporting** via Slack slash commands
- **⚡ Automated sync** between TimeCamp and PostgreSQL database  
- **🚨 Threshold monitoring** for budget overruns
- **👥 User management** for proper name display in reports
- **⏰ Scheduled updates** for daily, weekly, and monthly summaries

## 🚀 Quick Start

**Get up and running in under 10 minutes:**

```bash
# 1. Clone and setup
git clone <your-repo>
cd observe-yor-estimates
go mod download

# 2. Configure environment
cat > .env << EOF
DATABASE_URL=postgresql://user:pass@host:port/dbname
TIMECAMP_API_KEY=your_timecamp_api_key
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_slack_verification_token
EOF

# 3. Initialize and run
go run . --init-db
go run .
```

**Production deployment:** Deploy to Railway in minutes with our [Quick Start Guide](docs/QUICK_START.md).

## 📝 Core Features

### Slack Commands
```bash
/oye daily          # Daily time summary
/oye weekly public  # Public weekly report  
/oye sync           # Sync with TimeCamp
/oye over 80 weekly # Tasks over 80% estimate
```

### Automated Monitoring
- **Data Sync**: Automatic TimeCamp synchronization every 5-10 minutes
- **Threshold Alerts**: Proactive notifications for budget overruns
- **Scheduled Reports**: Daily, weekly, and monthly team updates

### User-Friendly Reports
```
📊 Daily Time Summary - November 15, 2024

• Today: 7h 30m [John Smith: 4h 15m, Mary Johnson: 3h 15m]
• This Week: 32h 45m [John Smith: 18h 30m, Mary Johnson: 14h 15m]

🎯 Top Tasks Today:
• Website Redesign: 3h 45m (75% of estimate)
• API Development: 2h 30m (50% of estimate) 
• Bug Fixes: 1h 15m (125% of estimate) ⚠️
```

## 📚 Documentation

For comprehensive setup, configuration, and usage information, visit our complete documentation:

### 🚀 Getting Started
- **[Quick Start Guide](docs/QUICK_START.md)** - Get running in 10 minutes
- **[Installation & Setup](docs/INSTALLATION.md)** - Complete setup for all environments
- **[Configuration](docs/CONFIGURATION.md)** - Environment variables and options

### 📖 User Guides
- **[Slack Integration](docs/SLACK_INTEGRATION.md)** - Setting up and using Slack features
- **[CLI Commands](docs/CLI_COMMANDS.md)** - Complete command-line reference
- **[User Management](docs/USER_MANAGEMENT.md)** - Managing team members

### 🔧 Technical Documentation
- **[Architecture Overview](docs/ARCHITECTURE.md)** - System design and components
- **[API Reference](docs/API_REFERENCE.md)** - Complete API documentation
- **[Database Schema](docs/DATABASE.md)** - Database structure and relationships

### 🛠️ Operations
- **[Deployment Guide](docs/DEPLOYMENT_GUIDE.md)** - Production deployment
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and solutions
- **[Performance](docs/PERFORMANCE.md)** - Optimization and scaling

**📋 [Full Documentation Index](docs/README.md)**

## 🏗️ Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   TimeCamp  │────│     OYE     │────│    Slack    │
│     API     │    │ Application │    │     API     │
└─────────────┘    └─────────────┘    └─────────────┘
                           │
                    ┌─────────────┐
                    │ PostgreSQL  │
                    │  Database   │
                    └─────────────┘
```

**Why Monolithic?** Simplified deployment, better performance, easier debugging, and lower resource overhead.

## 📋 Prerequisites

- **Go 1.18+** for development
- **PostgreSQL** database (local or hosted)
- **TimeCamp account** with API access
- **Slack workspace** with admin permissions

## 🔧 Development Commands

```bash
# Help and version
go run . --help
go run . --version

# Database management
go run . --init-db

# Data synchronization
go run . sync-tasks
go run . sync-time-entries
go run . full-sync

# Generate reports
go run . update daily
go run . update weekly
go run . update monthly

# User management
go run . list-users
go run . add-user 123 "john" "John Doe"
```

## 🌐 Deployment Options

- **[Railway](docs/DEPLOYMENT_GUIDE.md#railway-deployment)** - Recommended for quick deployment
- **[Docker](docs/DEPLOYMENT_GUIDE.md#docker-deployment)** - Container-based deployment
- **[Binary](docs/DEPLOYMENT_GUIDE.md#binary-deployment)** - Direct server deployment

## 🆘 Need Help?

- **Quick issues:** Check our [Troubleshooting Guide](docs/TROUBLESHOOTING.md)
- **Setup problems:** Follow the [Installation Guide](docs/INSTALLATION.md)  
- **Slack integration:** See [Slack Integration Guide](docs/SLACK_INTEGRATION.md)
- **CLI usage:** Reference [CLI Commands](docs/CLI_COMMANDS.md)

## 📖 Learn More

- [TimeCamp API Documentation](https://developer.timecamp.com/)
- [Slack API Documentation](https://api.slack.com/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)

---

**Ready to get started?** Jump to our [Quick Start Guide](docs/QUICK_START.md) and be up and running in 10 minutes! 🚀
