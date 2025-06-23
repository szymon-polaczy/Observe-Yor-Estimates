# Deployment Guide - Observe-Yor-Estimates on Railway

This guide covers deploying the Observe-Yor-Estimates (OYE) system on Railway. The application runs as a single, long-running Go server.

## 🎯 Architecture Summary

**The application is a self-contained Go server that:**
- Handles all HTTP requests for Slack slash commands.
- Includes an internal cron scheduler for periodic tasks like data synchronization and sending reports.
- Is deployed as a Docker container on Railway.

This architecture simplifies deployment and removes the complexities and limitations of a serverless environment.

## 🚀 Deployment Steps

### 1. Fork the Repository

First, fork this repository to your own GitHub account.

### 2. Create a Railway Project

1.  Go to your Railway dashboard.
2.  Click "New Project".
3.  Select "Deploy from GitHub repo" and choose your forked repository.
4.  Railway will automatically detect the `Dockerfile` and start building your application.

### 3. Environment Variables

In your Railway project, go to the "Variables" tab and add the following environment variables:

#### Required Variables
```
# Database (PostgreSQL) - You can create a PostgreSQL database on Railway
DATABASE_URL=postgresql://username:password@host:port/database

# TimeCamp API
TIMECAMP_API_KEY=your_timecamp_api_key

# Slack Integration
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_VERIFICATION_TOKEN=your_slack_verification_token
```

#### Optional Cron Schedule Overrides
The server has default schedules for all cron jobs. You can override them by setting these variables. The format is a standard cron expression.

```
# Sync Schedules (cron format)
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"
```

### 4. Expose the Application

1.  In your Railway project, go to the "Settings" tab.
2.  Under "Networking", click "Generate Domain" to get a public URL for your application. This will be in the format `https://<project-name>.up.railway.app`.

## 📱 Slack App Configuration

### Update Slack App Settings

You need to update your Slack slash command to point to your new Railway service URL.

1.  Go to your Slack App configuration page.
2.  Navigate to "Slash Commands".
3.  Edit your `/oye` command.
4.  Set the **Request URL** to your Railway application's public URL, followed by the `/slack/oye` path.

    ```
    https://<your-project-name>.up.railway.app/slack/oye
    ```

## ⏰ Scheduled Tasks

The Go application runs its own cron scheduler. There is no need to configure external cron job services. The schedules are defined in the code and can be overridden with environment variables as described above.

## 🧪 Testing Your Deployment

### 1. Health Check
You can test if your service is running by accessing the `/health` endpoint.

```bash
curl https://<your-project-name>.up.railway.app/health
```

Expected response:
```json
{
  "status": "healthy"
}
```

### 2. Test Slack Commands

Go to your Slack workspace and try out the commands:
```
/oye help
/oye daily
/oye sync
```

You should see an immediate response in Slack confirming that the command was received.

## 📊 Monitoring & Debugging

You can view the application logs in the Railway dashboard for your project. This will show you the output from the Go server, including any errors or debug messages. 