package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	var logger *Logger
	if getOutputJSON() {
		appLogger = NewLoggerForJSON()
		logger = appLogger
	} else {
		logger = NewLogger()
	}

	err := godotenv.Load()
	if err != nil {
		if isNetlifyBuild() {
			logger.Info("No .env file found (normal for Netlify builds)")
		} else {
			logger.Warnf("Warning: Failed to load .env file: %v", err)
		}
	}

	if err := validateRequiredEnvVars(); err != nil {
		logger.Fatalf("Critical error: Missing required environment variables: %v", err)
	}

	if len(os.Args) > 1 {
		handleCliCommands(os.Args[1:], logger)
		return
	}

	logger.Info("Starting Observe-Yor-Estimates application")

	// Initialize database connection for CLI operations
	logger.Info("Initializing database connection...")
	_, initErr := GetDB()
	if initErr != nil {
		logger.Errorf("Failed to initialize database: %v", initErr)
		return
	}
	logger.Info("Database connection initialized successfully")

	// Start as long-running server with background services
	logger.Info("Starting server mode with background services...")

	// Setup and start cron jobs for periodic syncing
	logger.Info("Setting up scheduled tasks...")
	setupCronJobs(logger)

	// Start HTTP server for Slack commands (this will block)
	logger.Info("Starting HTTP server for Slack integrations...")
	StartServer(logger)
}

func handleCliCommands(args []string, logger *Logger) {
	if len(args) == 0 {
		return
	}
	command := args[0]
	switch command {
	case "--help", "-h", "help":
		showHelp()
	case "--version", "version":
		fmt.Println("Observe-Yor-Estimates v1.0.0")
	case "--build-test", "build-test":
		fmt.Println("Build test successful - binary is working correctly")
	case "--init-db", "init-db":
		_, err := GetDB()
		if err != nil {
			logger.Fatalf("Failed to initialize database: %v", err)
		}
		logger.Info("Database initialized successfully")
	case "update":
		if len(args) < 2 {
			logger.Error("Error: update command requires a period (daily, weekly, or monthly)")
			return
		}
		period := args[1]
		if period != "daily" && period != "weekly" && period != "monthly" {
			logger.Errorf("Error: invalid period '%s'. Must be one of daily, weekly, or monthly.", period)
			return
		}
		logger.Infof("Running %s update command", period)

		// Check if we have Slack API context (from Netlify function)
		if os.Getenv("SLACK_BOT_TOKEN") != "" && os.Getenv("CHANNEL_ID") != "" {
			logger.Info("Using direct Slack API for context-aware response")
			handleDirectSlackUpdate(period)
		} else {
			// Fallback to original behavior
			responseURL := getResponseURL()
			outputJSON := getOutputJSON()
			SendSlackUpdate(period, responseURL, outputJSON)
		}
	case "sync-time-entries":
		logger.Info("Running time entries sync command")
		if err := SyncTimeEntriesToDatabase("", ""); err != nil {
			logger.Errorf("Time entries sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Time entries sync completed successfully")
	case "sync-tasks":
		logger.Info("Running tasks sync command")
		if err := SyncTasksToDatabaseFull(); err != nil {
			logger.Errorf("Tasks sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Tasks sync completed successfully")
	case "full-sync":
		logger.Info("Running full synchronization command")
		responseURL := getResponseURL()
		outputJSON := getOutputJSON()
		if outputJSON {
			SendFullSyncJSON()
		} else if responseURL != "" {
			SendFullSyncWithResponseURL(responseURL)
		} else {
			if err := FullSyncAll(); err != nil {
				logger.Errorf("Full sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Full synchronization completed successfully")
		}
	default:
		logger.Warnf("Unknown command line argument: %s", command)
		showHelp()
	}
}

func setupCronJobs(logger *Logger) {
	cronScheduler := cron.New()

	addCronJob(cronScheduler, "TASK_SYNC_SCHEDULE", "*/5 * * * *", "task sync", logger, func() {
		if err := SyncTasksToDatabaseIncremental(); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "TIME_ENTRIES_SYNC_SCHEDULE", "*/10 * * * *", "time entries sync", logger, func() {
		if err := SyncTimeEntriesToDatabase("", ""); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "DAILY_UPDATE_SCHEDULE", "0 6 * * *", "daily Slack update", logger, func() {
		SendSlackUpdate("daily", "", false)
	})

	addCronJob(cronScheduler, "WEEKLY_UPDATE_SCHEDULE", "0 8 * * 1", "weekly Slack update", logger, func() {
		SendSlackUpdate("weekly", "", false)
	})

	addCronJob(cronScheduler, "MONTHLY_UPDATE_SCHEDULE", "0 9 1 * *", "monthly Slack update", logger, func() {
		SendSlackUpdate("monthly", "", false)
	})

	cronScheduler.Start()
	logger.Info("Cron scheduler started successfully")
}

func addCronJob(scheduler *cron.Cron, envVar, defaultSchedule, jobName string, logger *Logger, cmd func()) {
	schedule := os.Getenv(envVar)
	if schedule == "" {
		schedule = defaultSchedule
	}
	_, err := scheduler.AddFunc(schedule, func() {
		logger.Debugf("Running scheduled %s", jobName)
		cmd()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule %s cron job: %v", jobName, err)
	}
}

func showHelp() {
	fmt.Println("Usage: observe-yor-estimates [command]")
	fmt.Println("\nAvailable commands:")
	fmt.Println("  update <period>          - Send Slack update for a period (daily, weekly, monthly)")
	fmt.Println("  sync-time-entries        - Sync recent time entries (last day)")
	fmt.Println("  sync-tasks               - Full sync of all tasks (manual operation)")
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  job-processor            - Run as standalone job processor server")
	fmt.Println("  --version, version         - Show application version")
	fmt.Println("  --help, -h, help         - Show help message")
	fmt.Println("\nSync Behavior:")
	fmt.Println("  • Cron jobs use incremental sync (only process changed tasks)")
	fmt.Println("  • Manual commands use full sync (process all tasks)")
	fmt.Println("  • This optimizes performance for regular automated updates")
	fmt.Println("\nSlack Integration:")
	fmt.Println("  Set up /oye command in Slack to point to /slack/oye endpoint")
	fmt.Println("  Requires SLACK_BOT_TOKEN environment variable for direct responses")
}

func getResponseURL() string {
	// First check command line arguments
	for i, arg := range os.Args {
		if (arg == "--response-url" || arg == "-r") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	// If not found in args, check environment variable (set by Netlify functions)
	return os.Getenv("RESPONSE_URL")
}

func getOutputJSON() bool {
	// First check command line arguments
	for _, arg := range os.Args {
		if arg == "--json" {
			return true
		}
	}
	// If not found in args, check environment variable (set by Netlify functions)
	return os.Getenv("OUTPUT_JSON") == "true"
}

func handleDirectSlackUpdate(period string) {
	logger := GetGlobalLogger()
	logger.Infof("Handling direct Slack update for period: %s", period)

	// Create Slack API client and context from environment
	slackClient := NewSlackAPIClientFromEnv()
	ctx := GetContextFromEnv()

	// Get database connection
	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	// Get task data
	taskInfos, err := getTaskChanges(db, period)
	if err != nil {
		logger.Errorf("Failed to get %s task changes: %v", period, err)
		slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to get %s changes", period))
		return
	}

	// Send direct response to Slack
	if len(taskInfos) == 0 {
		slackClient.SendNoChangesMessage(ctx, period)
	} else {
		// Default to personal update for now - can be enhanced with user preferences
		slackClient.SendPersonalUpdate(ctx, taskInfos, period)
	}

	logger.Infof("Direct Slack update completed for period: %s", period)
}
