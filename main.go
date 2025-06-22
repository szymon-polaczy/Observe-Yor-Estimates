package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	// Check if we're in JSON output mode first and configure logger accordingly
	var logger *Logger
	if getOutputJSON() {
		// Reinitialize logger to send all output to stderr
		appLogger = NewLoggerForJSON()
		logger = appLogger
	} else {
		// Initialize logger normally
		logger = NewLogger()
	}

	// Check for help arguments first, before any environment validation
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			showHelp()
			return
		case "--version", "version":
			fmt.Println("Observe-Yor-Estimates v1.0.0")
			return
		case "--build-test", "build-test":
			// Simple build test that doesn't require environment variables
			fmt.Println("Build test successful - binary is working correctly")
			return
		case "--init-db", "init-db":
			// Initialize database without requiring API keys (for Netlify deployment)
			// This creates a proper SQLite database file with the correct schema
			_, err := GetDB()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Database initialized successfully")
			return
		}
	}

	// Load environment variables - try to load .env file but don't fail if it doesn't exist
	err := godotenv.Load()
	if err != nil {
		// For Netlify builds, .env files may not exist, so only warn
		if isNetlifyBuild() {
			logger.Info("No .env file found (normal for Netlify builds)")
		} else {
			logger.Warnf("Warning: Failed to load .env file: %v", err)
		}
	}

	// Validate required environment variables
	if err := validateRequiredEnvVars(); err != nil {
		logger.Fatalf("Critical error: Missing required environment variables: %v", err)
	}

	logger.Info("Starting Observe-Yor-Estimates application")

	// Initialize database connection once at startup
	_, err = GetDB()
	if err != nil {
		logger.Fatalf("Critical error: Failed to initialize database: %v", err)
	}
	logger.Info("Database connection initialized successfully")

	// Check for command line arguments
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "daily-update":
			logger.Info("Running daily update command")
			responseURL := getResponseURL()
			outputJSON := getOutputJSON()
			if outputJSON {
				SendDailySlackUpdateJSON()
			} else if responseURL != "" {
				SendDailySlackUpdateWithResponseURL(responseURL)
			} else {
				SendDailySlackUpdate()
			}
			return
		case "weekly-update":
			logger.Info("Running weekly update command")
			responseURL := getResponseURL()
			outputJSON := getOutputJSON()
			if outputJSON {
				SendWeeklySlackUpdateJSON()
			} else if responseURL != "" {
				SendWeeklySlackUpdateWithResponseURL(responseURL)
			} else {
				SendWeeklySlackUpdate()
			}
			return
		case "monthly-update":
			logger.Info("Running monthly update command")
			responseURL := getResponseURL()
			outputJSON := getOutputJSON()
			if outputJSON {
				SendMonthlySlackUpdateJSON()
			} else if responseURL != "" {
				SendMonthlySlackUpdateWithResponseURL(responseURL)
			} else {
				SendMonthlySlackUpdate()
			}
			return
		case "sync-time-entries":
			logger.Info("Running time entries sync command")
			if err := SyncTimeEntriesToDatabase(); err != nil {
				logger.Errorf("Time entries sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Time entries sync completed successfully")
			return
		case "sync-tasks":
			logger.Info("Running tasks sync command")
			if err := SyncTasksToDatabase(); err != nil {
				logger.Errorf("Tasks sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Tasks sync completed successfully")
			return
		case "full-sync":
			logger.Info("Running full synchronization command")
			if err := FullSyncAll(); err != nil {
				logger.Errorf("Full sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Full synchronization completed successfully")
			return
		case "full-sync-tasks":
			logger.Info("Running full tasks sync command")
			if err := FullSyncTasksToDatabase(); err != nil {
				logger.Errorf("Full tasks sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Full tasks sync completed successfully")
			return
		case "full-sync-time-entries":
			logger.Info("Running full time entries sync command")
			if err := FullSyncTimeEntriesToDatabase(); err != nil {
				logger.Errorf("Full time entries sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Full time entries sync completed successfully")
			return
		default:
			logger.Warnf("Unknown command line argument: %s", os.Args[1])
			logger.Info("Available commands:")
			logger.Info("  daily-update             - Send daily Slack update")
			logger.Info("  weekly-update            - Send weekly Slack update")
			logger.Info("  monthly-update           - Send monthly Slack update")
			logger.Info("  sync-time-entries        - Sync recent time entries (last day)")
			logger.Info("  sync-tasks               - Sync all tasks")
			logger.Info("  full-sync                - Full sync of all tasks and time entries")
			logger.Info("  full-sync-tasks          - Full sync of all tasks only")
			logger.Info("  full-sync-time-entries   - Full sync of all time entries only")
			return
		}
	}

	// Run initial sync - log errors but don't crash the app
	logger.Info("Running initial task sync")
	if err := SyncTasksToDatabase(); err != nil {
		logger.Errorf("Failed initial task sync: %v", err)
		// Continue running - we can retry later via cron
	}

	// Set up cron scheduler
	cronScheduler := cron.New()

	// Get cron schedules from environment variables or use defaults
	taskSyncSchedule := os.Getenv("TASK_SYNC_SCHEDULE")
	if taskSyncSchedule == "" {
		taskSyncSchedule = "*/5 * * * *" // default: every 5 minutes
	}

	timeEntriesSyncSchedule := os.Getenv("TIME_ENTRIES_SYNC_SCHEDULE")
	if timeEntriesSyncSchedule == "" {
		timeEntriesSyncSchedule = "*/10 * * * *" // default: every 10 minutes
	}

	dailyUpdateSchedule := os.Getenv("DAILY_UPDATE_SCHEDULE")
	if dailyUpdateSchedule == "" {
		dailyUpdateSchedule = "0 6 * * *" // default: 6 AM daily
	}

	weeklyUpdateSchedule := os.Getenv("WEEKLY_UPDATE_SCHEDULE")
	if weeklyUpdateSchedule == "" {
		weeklyUpdateSchedule = "0 8 * * 1" // default: 8 AM on Mondays
	}

	monthlyUpdateSchedule := os.Getenv("MONTHLY_UPDATE_SCHEDULE")
	if monthlyUpdateSchedule == "" {
		monthlyUpdateSchedule = "0 9 1 * *" // default: 9 AM on the 1st of each month
	}

	// Schedule SyncTasksToDatabase to run based on configured schedule
	_, err = cronScheduler.AddFunc(taskSyncSchedule, func() {
		logger.Debug("Running scheduled task sync")
		if err := SyncTasksToDatabase(); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule task sync cron job: %v", err)
	}

	// Schedule SyncTimeEntriesToDatabase to run based on configured schedule
	_, err = cronScheduler.AddFunc(timeEntriesSyncSchedule, func() {
		logger.Debug("Running scheduled time entries sync")
		if err := SyncTimeEntriesToDatabase(); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule time entries sync cron job: %v", err)
	}

	// Schedule daily Slack update to run based on configured schedule
	_, err = cronScheduler.AddFunc(dailyUpdateSchedule, func() {
		logger.Debug("Running scheduled daily Slack update")
		SendDailySlackUpdate()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule daily Slack update: %v", err)
	}

	// Schedule weekly Slack update to run based on configured schedule
	_, err = cronScheduler.AddFunc(weeklyUpdateSchedule, func() {
		logger.Debug("Running scheduled weekly Slack update")
		SendWeeklySlackUpdate()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule weekly Slack update: %v", err)
	}

	// Schedule monthly Slack update to run based on configured schedule
	_, err = cronScheduler.AddFunc(monthlyUpdateSchedule, func() {
		logger.Debug("Running scheduled monthly Slack update")
		SendMonthlySlackUpdate()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule monthly Slack update: %v", err)
	}

	// Start the cron scheduler
	cronScheduler.Start()
	defer cronScheduler.Stop()

	logger.Info("Cron scheduler started successfully")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Setup Slack REST API routes
	setupSlackRoutes()

	// Get port from environment variable or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start HTTP server for Slack commands
	server := &http.Server{
		Addr: ":" + port,
	}

	go func() {
		logger.Infof("Starting HTTP server on port %s for Slack commands", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("HTTP server error: %v", err)
		}
	}()

	logger.Info("Application is running. Press Ctrl+C to stop.")
	logger.Infof("Slack command endpoints available at http://localhost:%s/slack/*", port)

	for {
		select {
		case <-interrupt:
			logger.Info("Received interrupt signal, shutting down gracefully...")

			// Stop the cron scheduler first
			logger.Info("Stopping cron scheduler...")
			cronScheduler.Stop()

			// Close the database connection
			logger.Info("Closing database connection...")
			if err := CloseDB(); err != nil {
				logger.Errorf("Error closing database: %v", err)
			} else {
				logger.Info("Database connection closed successfully")
			}

			// Shutdown HTTP server
			logger.Info("Shutting down HTTP server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				logger.Errorf("Error shutting down HTTP server: %v", err)
			} else {
				logger.Info("HTTP server shut down successfully")
			}

			return
		}
	}

}

// showHelp displays usage information for the application
func showHelp() {
	fmt.Println("Observe-Yor-Estimates - Task time tracking and Slack notifications")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  ./observe-yor-estimates [command]")
	fmt.Println("")
	fmt.Println("Available commands:")
	fmt.Println("  daily-update             - Send daily Slack update")
	fmt.Println("  weekly-update            - Send weekly Slack update")
	fmt.Println("  monthly-update           - Send monthly Slack update")
	fmt.Println("  sync-time-entries        - Sync recent time entries (last day)")
	fmt.Println("  sync-tasks               - Sync all tasks")
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  full-sync-tasks          - Full sync of all tasks only")
	fmt.Println("  full-sync-time-entries   - Full sync of all time entries only")
	fmt.Println("  --version, version       - Show version information")
	fmt.Println("  --build-test, build-test - Test that the binary is working (for builds)")
	fmt.Println("  --init-db, init-db       - Initialize database without API keys (for deployments)")
	fmt.Println("  --help, -h, help         - Show this help message")
	fmt.Println("")
	fmt.Println("Command options:")
	fmt.Println("  --response-url=<url>     - Send response to Slack response URL (for slash commands)")
	fmt.Println("")
	fmt.Println("If no command is provided, the application will start in daemon mode")
	fmt.Println("with scheduled synchronization and Slack updates.")
	fmt.Println("")
	fmt.Println("Slack commands are available via REST API endpoints:")
	fmt.Println("  POST /slack/daily-update   - Trigger daily update")
	fmt.Println("  POST /slack/weekly-update  - Trigger weekly update")
	fmt.Println("  POST /slack/monthly-update - Trigger monthly update")
	fmt.Println("  GET  /health               - Health check endpoint")
	fmt.Println("")
	fmt.Println("Required environment variables:")
	fmt.Println("  SLACK_WEBHOOK_URL         - Slack webhook URL for notifications")
	fmt.Println("  TIMECAMP_API_KEY          - TimeCamp API key")
	fmt.Println("")
	fmt.Println("Optional environment variables:")
	fmt.Println("  SLACK_VERIFICATION_TOKEN  - Slack verification token for security")
	fmt.Println("  PORT                      - HTTP server port (default: 8080)")
	fmt.Println("")
	fmt.Println("For more information, see the README.md and Documentation/ folder.")
}

// getResponseURL extracts the response URL from command line arguments
// Looks for --response-url=<url> argument
func getResponseURL() string {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "--response-url=") {
			return strings.TrimPrefix(arg, "--response-url=")
		}
	}
	return ""
}

// getOutputJSON checks if the --output-json flag is set
// This flag indicates that the Go binary should output JSON to stdout instead of sending to Slack
func getOutputJSON() bool {
	for _, arg := range os.Args {
		if arg == "--output-json" {
			return true
		}
	}
	return false
}
