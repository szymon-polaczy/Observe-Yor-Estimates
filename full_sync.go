package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// FullSyncTasksToDatabase fetches ALL tasks from TimeCamp and stores them in the database
// This is intended for initial setup or full re-sync operations
func FullSyncTasksToDatabase() error {
	logger := GetGlobalLogger()

	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting FULL task synchronization with TimeCamp")

	// Validate database write access before proceeding
	if err := validateDatabaseWriteAccess(); err != nil {
		return fmt.Errorf("database write validation failed: %w", err)
	}

	// Validate required environment variables before proceeding
	apiKey := os.Getenv("TIMECAMP_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("TIMECAMP_API_KEY environment variable not set - cannot proceed with sync")
	}
	logger.Debug("TimeCamp API key is configured")

	// Use the updated SyncTasksToDatabase with fullSync=true
	return SyncTasksToDatabase(true)
}

// FullSyncTimeEntriesToDatabase fetches ALL time entries from TimeCamp and stores them in the database
// This is intended for initial setup or full re-sync operations
func FullSyncTimeEntriesToDatabase() error {
	logger := GetGlobalLogger()

	logger.Debug("Starting FULL time entries synchronization with TimeCamp")

	// For full sync, get entries from a much longer period (e.g., last 6 months)
	// You can adjust this timeframe based on your needs
	fromDate := time.Now().AddDate(0, -6, 0).Format("2006-01-02") // 6 months ago
	toDate := time.Now().Format("2006-01-02")

	logger.Infof("Full sync: retrieving time entries from %s to %s", fromDate, toDate)

	// Use the updated SyncTimeEntriesToDatabase function with custom date range
	return SyncTimeEntriesToDatabase(fromDate, toDate)
}

// FullSyncAll performs both full tasks sync and full time entries sync with optimizations
func FullSyncAll() error {
	logger := GetGlobalLogger()

	logger.Info("Starting optimized full synchronization of all data from TimeCamp")

	// Validate database write access before attempting sync operations
	logger.Debug("Validating database write access...")
	if err := validateDatabaseWriteAccess(); err != nil {
		return fmt.Errorf("database write validation failed: %w", err)
	}
	logger.Debug("Database write access validated successfully")

	startTime := time.Now()

	// Sync tasks first (time entries depend on tasks)
	logger.Info("Starting optimized full tasks sync...")
	if err := FullSyncTasksToDatabase(); err != nil {
		return fmt.Errorf("full tasks sync failed: %w", err)
	}
	logger.Info("Full tasks sync completed successfully")

	// Then sync time entries
	logger.Info("Starting optimized full time entries sync...")
	if err := FullSyncTimeEntriesToDatabase(); err != nil {
		return fmt.Errorf("full time entries sync failed: %w", err)
	}
	logger.Info("Full time entries sync completed successfully")

	duration := time.Since(startTime)
	logger.Infof("Optimized full synchronization completed successfully in %v", duration.Round(time.Second))
	return nil
}

// SendFullSyncJSON performs a full sync and outputs the result as JSON to stdout
func SendFullSyncJSON() {
	logger := GetGlobalLogger()
	logger.Info("Starting full sync JSON output")

	if err := FullSyncAll(); err != nil {
		logger.Errorf("Full sync failed: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Full synchronization failed",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: fmt.Sprintf("❌ *Full Sync Failed*\n\nError: `%v`\n*Time:* %s", err, time.Now().Format("2006-01-02 15:04:05")),
					},
				},
			},
		}
		outputJSON([]SlackMessage{errorMessage})
		return
	}

	// Send success message
	message := SlackMessage{
		Text: "✅ Full synchronization completed successfully",
		Blocks: []Block{
			{
				Type: "header",
				Text: &Text{
					Type: "plain_text",
					Text: "✅ Full Sync Complete",
				},
			},
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Full synchronization completed successfully*\n\n• All tasks synced from TimeCamp\n• Time entries synced (last 6 months)\n• Database is now up to date\n\n*Completed at:* %s", time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	outputJSON([]SlackMessage{message})
	logger.Info("Successfully generated full sync JSON")
}

// SendFullSyncWithResponseURL performs a full sync and sends the result to a Slack response URL
func SendFullSyncWithResponseURL(responseURL string) {
	logger := GetGlobalLogger()
	logger.Info("Starting full sync with response URL")

	if err := FullSyncAll(); err != nil {
		logger.Errorf("Full sync failed: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Full synchronization failed",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: fmt.Sprintf("❌ *Full Sync Failed*\n\nError: `%v`\n*Time:* %s", err, time.Now().Format("2006-01-02 15:04:05")),
					},
				},
			},
		}
		sendDelayedResponseShared(responseURL, errorMessage)
		return
	}

	// Send success message
	message := SlackMessage{
		Text: "✅ Full synchronization completed successfully",
		Blocks: []Block{
			{
				Type: "header",
				Text: &Text{
					Type: "plain_text",
					Text: "✅ Full Sync Complete",
				},
			},
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Full synchronization completed successfully*\n\n• All tasks synced from TimeCamp\n• Time entries synced (last 6 months)\n• Database is now up to date\n\n*Completed at:* %s", time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	if err := sendDelayedResponseShared(responseURL, message); err != nil {
		logger.Errorf("Failed to send delayed response: %v", err)
	} else {
		logger.Info("Successfully sent full sync completion message via response URL")
	}
}
