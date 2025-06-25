# Observe-Yor-Estimates Documentation

Welcome to the comprehensive documentation for the OYE (Observe-Yor-Estimates) project - a TimeCamp to Slack integration system.

## 📚 Documentation Structure

### Getting Started
- [Installation & Setup](INSTALLATION.md) - Complete setup guide for development and production
- [Quick Start Guide](QUICK_START.md) - Get up and running in minutes
- [Configuration](CONFIGURATION.md) - Environment variables and configuration options

### Development
- [Architecture Overview](ARCHITECTURE.md) - System design and component overview
- [API Reference](API_REFERENCE.md) - Complete API endpoints and usage
- [Database Schema](DATABASE.md) - Database structure and relationships
- [Development Guide](DEVELOPMENT.md) - Development workflow and guidelines

### Operations
- [Deployment Guide](DEPLOYMENT_GUIDE.md) - Production deployment instructions
- [User Management](USER_MANAGEMENT.md) - Managing users in the system
- [Monitoring & Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
- [CLI Commands](CLI_COMMANDS.md) - Complete command-line interface reference

### Features
- [Slack Integration](SLACK_INTEGRATION.md) - Slack slash commands and responses
- [Time Tracking](TIME_TRACKING.md) - TimeCamp integration and sync processes
- [Scheduled Tasks](SCHEDULED_TASKS.md) - Automated sync and reporting jobs
- [Threshold Monitoring](THRESHOLD_MONITORING.md) - Task estimation monitoring

### Advanced Topics
- [Security](SECURITY.md) - Security considerations and best practices
- [Performance](PERFORMANCE.md) - Performance optimization and scaling
- [Contributing](CONTRIBUTING.md) - How to contribute to the project
- [Changelog](CHANGELOG.md) - Version history and changes

## 🔍 Quick Navigation

| Need to... | Go to |
|------------|-------|
| Set up the project locally | [Installation & Setup](INSTALLATION.md) |
| Deploy to production | [Deployment Guide](DEPLOYMENT_GUIDE.md) |
| Add/manage users | [User Management](USER_MANAGEMENT.md) |
| Understand Slack commands | [Slack Integration](SLACK_INTEGRATION.md) |
| Troubleshoot issues | [Monitoring & Troubleshooting](TROUBLESHOOTING.md) |
| Use CLI commands | [CLI Commands](CLI_COMMANDS.md) |

## 🎯 Project Overview

OYE is a monolithic Go application that bridges TimeCamp time tracking with Slack notifications. It provides:

- **Real-time sync** between TimeCamp and PostgreSQL database
- **Slack slash commands** for on-demand reporting
- **Automated scheduling** for periodic updates and threshold monitoring
- **User management** for proper name display in reports
- **Threshold alerts** for tasks exceeding estimated time

## 🏗️ System Architecture

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

## 📋 Prerequisites

- Go 1.18+
- PostgreSQL database
- TimeCamp API access
- Slack workspace and bot permissions

## 🚀 Getting Started

1. **First Time Setup**: Start with [Installation & Setup](INSTALLATION.md)
2. **Quick Demo**: Follow the [Quick Start Guide](QUICK_START.md)
3. **Production Deployment**: See [Deployment Guide](DEPLOYMENT_GUIDE.md)

## 📖 Additional Resources

- [Project Repository](../) - Main codebase
- [TimeCamp API Documentation](https://developer.timecamp.com/)
- [Slack API Documentation](https://api.slack.com/)

---

*This documentation is automatically maintained. Last updated: $(date)* 