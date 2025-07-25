package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	logger := NewLogger()

	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Warning: Failed to load .env file: %v", err)
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
	case "full-sync":
		logger.Info("Running full synchronization command")
		if err := FullSyncAll(); err != nil {
			logger.Errorf("Full sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full synchronization completed successfully")
	default:
		logger.Warnf("Unknown command line argument: %s", command)
		showHelp()
	}
}

func setupCronJobs(logger *Logger) {
	cronScheduler := cron.New()

	addCronJob(cronScheduler, "TASK_SYNC_SCHEDULE", "0 */3 * * *", "task sync", logger, func() {
		if err := SyncTasksToDatabase(false); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "TIME_ENTRIES_SYNC_SCHEDULE", "*/10 * * * *", "time entries sync", logger, func() {
		if err := SyncTimeEntriesToDatabaseWithOptions("", "", false); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "DAILY_UPDATE_SCHEDULE", "0 6 * * *", "daily Slack update", logger, func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection for daily update: %v", err)
			return
		}

		taskInfos, err := getTaskChanges(db, "daily", 1)
		if err != nil {
			logger.Errorf("Failed to get task changes for daily update: %v", err)
			return
		}

		if err := SendSlackUpdate(taskInfos, "daily"); err != nil {
			logger.Errorf("Failed to send daily Slack update: %v", err)
		}
	})

	// Add orphaned time entries processing cron job (every hour)
	addCronJob(cronScheduler, "ORPHANED_PROCESSING_SCHEDULE", "0 * * * *", "orphaned time entries processing", logger, func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection for orphaned processing: %v", err)
			return
		}

		// Check if there are any orphaned entries to process
		count, err := GetOrphanedTimeEntriesCount(db)
		if err != nil {
			logger.Errorf("Failed to count orphaned entries: %v", err)
			return
		}

		if count > 0 {
			logger.Infof("Found %d orphaned time entries, processing...", count)
			if err := ProcessOrphanedTimeEntries(db); err != nil {
				logger.Errorf("Orphaned time entries processing failed: %v", err)
			} else {
				logger.Debug("Orphaned time entries processing completed successfully")
			}
		}
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
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  threshold-check          - Manual threshold monitoring check")
	fmt.Println("")
	fmt.Println("  --version, version         - Show application version")
	fmt.Println("  --help, -h, help         - Show help message")
	fmt.Println("\nSync Behavior:")
	fmt.Println("  • Cron jobs use incremental sync every 3 hours (only process changed tasks)")
	fmt.Println("  • Manual commands use full sync (process all tasks)")
	fmt.Println("  • Uses TimeCamp API minimal option for optimized performance")
	fmt.Println("\nThreshold Monitoring:")
	fmt.Println("  • Automatic alerts for tasks crossing 50%, 80%, 90%, and 100% thresholds")
	fmt.Println("  • Runs every 15 minutes via cron job")
	fmt.Println("  • Manual Slack commands: /oye over <percentage> <period>")
	fmt.Println("\nSlack Integration:")
	fmt.Println("  Set up /oye command in Slack to point to /slack/oye endpoint")
	fmt.Println("  Requires SLACK_BOT_TOKEN environment variable for direct responses")
}
