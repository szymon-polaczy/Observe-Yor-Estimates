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

		// Get database connection
		db, err := GetDB()
		if err != nil {
			logger.Fatalf("Failed to get database connection: %v", err)
		}

		// Get task changes
		taskInfos, err := getTaskChanges(db, period)
		if err != nil {
			logger.Fatalf("Failed to get task changes: %v", err)
		}

		// Send update
		if err := SendSlackUpdate(taskInfos, period); err != nil {
			logger.Fatalf("Failed to send Slack update: %v", err)
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
	case "full-sync-tasks-only":
		logger.Info("Running full tasks synchronization only")
		if err := SyncTasksToDatabaseFull(); err != nil {
			logger.Errorf("Full tasks sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full tasks synchronization completed successfully")
	case "full-sync-entries-only":
		logger.Info("Running full time entries synchronization only")
		if err := FullSyncTimeEntriesToDatabase(); err != nil {
			logger.Errorf("Full time entries sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full time entries synchronization completed successfully")
	case "threshold-check":
		logger.Info("Running threshold monitoring check")
		if err := RunThresholdMonitoring(); err != nil {
			logger.Errorf("Threshold monitoring failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Threshold monitoring completed successfully")
	case "process-orphaned":
		logger.Info("Processing orphaned time entries")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}

		// Show current count before processing
		if count, err := GetOrphanedTimeEntriesCount(db); err != nil {
			logger.Errorf("Failed to get orphaned entries count: %v", err)
			os.Exit(1)
		} else {
			logger.Infof("Found %d orphaned time entries to process", count)
			if count == 0 {
				logger.Info("No orphaned time entries to process")
				return
			}
		}

		if err := ProcessOrphanedTimeEntries(db); err != nil {
			logger.Errorf("Failed to process orphaned time entries: %v", err)
			os.Exit(1)
		}

		// Show remaining count after processing
		if count, err := GetOrphanedTimeEntriesCount(db); err != nil {
			logger.Warnf("Failed to get remaining orphaned entries count: %v", err)
		} else {
			logger.Infof("Remaining orphaned time entries: %d", count)
		}
		logger.Info("Orphaned time entries processing completed successfully")
	case "cleanup-orphaned":
		if len(args) < 2 {
			logger.Error("Error: cleanup-orphaned command requires number of days (e.g., cleanup-orphaned 30)")
			return
		}

		var days int
		if _, err := fmt.Sscanf(args[1], "%d", &days); err != nil {
			logger.Errorf("Error: invalid number of days '%s'", args[1])
			return
		}

		if days < 1 {
			logger.Error("Error: number of days must be at least 1")
			return
		}

		logger.Infof("Cleaning up orphaned time entries older than %d days", days)
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}

		if err := CleanupOldOrphanedEntries(db, days); err != nil {
			logger.Errorf("Failed to cleanup orphaned entries: %v", err)
			os.Exit(1)
		}
		logger.Info("Orphaned entries cleanup completed successfully")
	case "sync-projects":
		logger.Info("Syncing projects table from task hierarchy")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}
		if err := SyncProjectsFromTasks(db); err != nil {
			logger.Errorf("Error syncing projects: %v", err)
			os.Exit(1)
		}
		logger.Info("Projects synced successfully")
	case "list-projects":
		logger.Info("Listing all projects from database")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}
		projects, err := GetAllProjects(db)
		if err != nil {
			logger.Errorf("Error listing projects: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d projects:\n", len(projects))
		for _, project := range projects {
			fmt.Printf("- %s (TimeCamp Task ID: %d)\n", project.Name, project.TimeCampTaskID)
		}
	default:
		logger.Warnf("Unknown command line argument: %s", command)
		showHelp()
	}
}

func setupCronJobs(logger *Logger) {
	cronScheduler := cron.New()

	addCronJob(cronScheduler, "TASK_SYNC_SCHEDULE", "0 */3 * * *", "task sync", logger, func() {
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
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection for daily update: %v", err)
			return
		}

		taskInfos, err := getTaskChanges(db, "daily")
		if err != nil {
			logger.Errorf("Failed to get task changes for daily update: %v", err)
			return
		}

		if err := SendSlackUpdate(taskInfos, "daily"); err != nil {
			logger.Errorf("Failed to send daily Slack update: %v", err)
		}
	})

	// Add threshold monitoring cron job (every minute)
	addCronJob(cronScheduler, "THRESHOLD_MONITORING_SCHEDULE", "*/15 * * * *", "threshold monitoring", logger, func() {
		if err := RunThresholdMonitoring(); err != nil {
			logger.Errorf("Threshold monitoring failed: %v", err)
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
	fmt.Println("  sync-time-entries        - Sync recent time entries (last day)")
	fmt.Println("  sync-tasks               - Full sync of all tasks (manual operation)")
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  full-sync-tasks-only     - Full sync of tasks only")
	fmt.Println("  full-sync-entries-only   - Full sync of time entries only")
	fmt.Println("  threshold-check          - Manual threshold monitoring check")
	fmt.Println("  process-orphaned         - Process orphaned time entries")
	fmt.Println("  cleanup-orphaned <days>   - Clean up orphaned time entries older than specified days")
	fmt.Println("")
	fmt.Println("User management commands:")
	fmt.Println("  sync-users               - Sync users from TimeCamp to the database")
	fmt.Println("  list-users               - Show all users in the database")
	fmt.Println("  active-users             - Show user IDs that have time entries")
	fmt.Println("  add-user <id> <user> <name> - Add a specific user to the database")
	fmt.Println("")
	fmt.Println("Project management commands:")
	fmt.Println("  sync-projects            - Sync projects table from task hierarchy")
	fmt.Println("  list-projects            - Show all projects in the database")
	fmt.Println("")
	fmt.Println("Testing commands:")
	fmt.Println("  test-command <command>   - Test OYE commands locally (e.g., test-command \"/oye Filestage.io this month\")")
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

func getResponseURL() string {
	// First check command line arguments
	for i, arg := range os.Args {
		if (arg == "--response-url" || arg == "-r") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func getOutputJSON() bool {
	// First check command line arguments
	for _, arg := range os.Args {
		if arg == "--json" {
			return true
		}
	}
	return false
}
