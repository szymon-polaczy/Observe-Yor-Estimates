package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func FullSyncTasksToDatabase() error {
	logger := GetGlobalLogger()

	if err := godotenv.Load(); err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting FULL task synchronization with TimeCamp")

	if err := validateDatabaseWriteAccess(); err != nil {
		return fmt.Errorf("database write validation failed: %w", err)
	}

	if apiKey := os.Getenv("TIMECAMP_API_KEY"); apiKey == "" {
		return fmt.Errorf("TIMECAMP_API_KEY environment variable not set - cannot proceed with sync")
	}

	logger.Debug("TimeCamp API key is configured")
	return SyncTasksToDatabase(true)
}

func FullSyncTimeEntriesToDatabase() error {
	logger := GetGlobalLogger()
	logger.Debug("Starting FULL time entries synchronization with TimeCamp")

	fromDate := time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	toDate := time.Now().Format("2006-01-02")

	logger.Infof("Full sync: retrieving time entries from %s to %s", fromDate, toDate)
	return SyncTimeEntriesToDatabaseWithOptions(fromDate, toDate, true)
}

func FullSyncAll() error {
	logger := GetGlobalLogger()
	logger.Info("Starting optimized full synchronization of all data from TimeCamp")

	logger.Debug("Validating database write access...")
	if err := validateDatabaseWriteAccess(); err != nil {
		return fmt.Errorf("database write validation failed: %w", err)
	}
	logger.Debug("Database write access validated successfully")

	startTime := time.Now()

	logger.Info("Starting optimized full tasks sync...")
	if err := FullSyncTasksToDatabase(); err != nil {
		return fmt.Errorf("full tasks sync failed: %w", err)
	}
	logger.Info("Full tasks sync completed successfully")

	logger.Info("Starting optimized full time entries sync...")
	if err := FullSyncTimeEntriesToDatabase(); err != nil {
		return fmt.Errorf("full time entries sync failed: %w", err)
	}
	logger.Info("Full time entries sync completed successfully")

	logger.Info("Processing orphaned time entries...")
	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection for orphaned processing: %v", err)
	} else {
		if err := ProcessOrphanedTimeEntries(db); err != nil {
			logger.Errorf("Failed to process orphaned time entries: %v", err)
		} else {
			if count, err := GetOrphanedTimeEntriesCount(db); err != nil {
				logger.Warnf("Failed to count remaining orphaned entries: %v", err)
			} else if count > 0 {
				logger.Infof("Remaining orphaned time entries: %d (likely for deleted/archived tasks)", count)
			} else {
				logger.Info("All orphaned time entries successfully processed")
			}
		}
	}

	duration := time.Since(startTime)
	logger.Infof("Optimized full synchronization completed successfully in %v", duration.Round(time.Second))
	return nil
}

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
		sendSlackResponse(responseURL, errorMessage)
		return
	}

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

	if err := sendSlackResponse(responseURL, message); err != nil {
		logger.Errorf("Failed to send delayed response: %v", err)
	} else {
		logger.Info("Successfully sent full sync completion message via response URL")
	}
}
