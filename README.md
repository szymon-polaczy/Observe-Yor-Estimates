# Observe-Yor-Estimates

A TimeCamp to Slack integration that runs as a self-contained Go application.

## 🏗️ Architecture Overview

This project is a monolithic Go application that includes:
- An HTTP server to handle Slack slash commands.
- An integrated cron scheduler for periodic data syncing and reporting.

It is designed to be deployed as a Docker container. This architecture simplifies deployment and maintenance.

## 🚀 Quick Start

### Prerequisites

- Go 1.18+
- Docker
- PostgreSQL database

### Local Development

1.  **Clone the repository:**
    ```bash
    git clone <your-repo>
    cd observe-yor-estimates
    ```

2.  **Set up environment variables:**
    Create a `.env` file (you can copy `.env.example` if it exists) with the necessary variables:
    ```bash
    # Database Configuration
    DATABASE_URL=postgresql://user:pass@host:port/dbname

    # TimeCamp API
    TIMECAMP_API_KEY=your_timecamp_api_key

    # Slack Configuration
    SLACK_BOT_TOKEN=xoxb-your-bot-token
    SLACK_VERIFICATION_TOKEN=your_slack_verification_token
    ```

3.  **Run the application:**
    ```bash
    go run .
    ```
    The server will start on port 8080.

### Building with Docker

You can build the Docker image locally:
```bash
docker build -t observe-yor-estimates .
```

And run it:
```bash
docker run -p 8080:8080 --env-file .env observe-yor-estimates
```

## 📁 Project Structure

```
observe-yor-estimates/
├── *.go                       # Go source code
├── go.mod, go.sum             # Go module files
├── Dockerfile                 # Docker configuration for deployment
├── DEPLOYMENT_GUIDE.md        # Detailed deployment instructions
└── README.md                  # This file
```

## 🚀 Deployment

This application is designed for deployment on platforms like Railway that support Docker-based deployments.

For detailed deployment instructions, please see the [Deployment Guide](DEPLOYMENT_GUIDE.md).

## 🔧 Go CLI Commands

The Go binary supports several commands for manual operations:

```bash
# Get help
go run . --help

# Send updates to Slack
go run . update daily
go run . update weekly
go run . update monthly

# Manual data synchronization
go run . sync-tasks
go run . sync-time-entries
go run . full-sync
```

## 🆘 Troubleshooting

Check the application logs for any error messages. When running locally, logs are printed to the console. In a deployed environment like Railway, you can view the logs through the platform's dashboard.
