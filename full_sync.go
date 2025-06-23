package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	timecampTasks, err := getTimecampTasksFull()
	if err != nil {
		return fmt.Errorf("failed to fetch tasks from TimeCamp: %w", err)
	}

	if len(timecampTasks) == 0 {
		logger.Warn("No tasks received from TimeCamp API")
		return nil // Not an error, just no data
	}

	logger.Infof("Retrieved %d tasks from TimeCamp for full sync", len(timecampTasks))

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Use INSERT ... ON CONFLICT to handle existing tasks during full sync (PostgreSQL equivalent of INSERT OR REPLACE)
	insertStatement, err := db.Prepare(`INSERT INTO tasks (task_id, parent_id, assigned_by, name, level, root_group_id) 
		VALUES ($1, $2, $3, $4, $5, $6) 
		ON CONFLICT (task_id) DO UPDATE SET 
		parent_id = EXCLUDED.parent_id,
		assigned_by = EXCLUDED.assigned_by,
		name = EXCLUDED.name,
		level = EXCLUDED.level,
		root_group_id = EXCLUDED.root_group_id`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer CloseWithErrorLog(insertStatement, "prepared statement")

	// Process all tasks
	errorCount := 0
	successCount := 0
	var firstError error // Capture the first error for better diagnostics

	for _, task := range timecampTasks {
		_, err = insertStatement.Exec(task.TaskID, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID)
		if err != nil {
			logger.Errorf("Failed to insert task %d (%s): %v", task.TaskID, task.Name, err)
			errorCount++
			// Capture the first error for detailed reporting
			if firstError == nil {
				firstError = err
			}
			continue
		}

		successCount++

		// Track task changes for existing tasks
		if trackErr := TrackTaskChange(db, task.TaskID, task.Name, "full_sync", "", ""); trackErr != nil {
			logger.Errorf("Failed to track task change for task %d during full sync: %v", task.TaskID, trackErr)
		}
	}

	logger.Infof("Full task sync completed: %d tasks processed successfully, %d errors encountered", successCount, errorCount)

	if errorCount > 0 && successCount == 0 {
		// Provide more detailed error information for diagnosis
		return fmt.Errorf("all task operations failed during full sync - first error: %v (total tasks attempted: %d, all failed)", firstError, len(timecampTasks))
	}

	return nil
}

// FullSyncTimeEntriesToDatabase fetches ALL time entries from TimeCamp and stores them in the database
// This is intended for initial setup or full re-sync operations
func FullSyncTimeEntriesToDatabase() error {
	logger := GetGlobalLogger()

	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting FULL time entries synchronization with TimeCamp")

	// For full sync, get entries from a much longer period (e.g., last 6 months)
	// You can adjust this timeframe based on your needs
	fromDate := time.Now().AddDate(0, -6, 0).Format("2006-01-02") // 6 months ago
	toDate := time.Now().Format("2006-01-02")

	logger.Infof("Full sync: retrieving time entries from %s to %s", fromDate, toDate)

	timeEntries, err := getTimeCampTimeEntriesFull(fromDate, toDate)
	if err != nil {
		return fmt.Errorf("failed to fetch time entries from TimeCamp: %w", err)
	}

	if len(timeEntries) == 0 {
		logger.Info("No time entries received from TimeCamp API")
		return nil // Not an error, just no data
	}

	logger.Infof("Retrieved %d time entries from TimeCamp for full sync", len(timeEntries))

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Ensure time_entries table exists
	if err := migrateTimeEntriesTable(db); err != nil {
		return fmt.Errorf("failed to migrate time_entries table: %w", err)
	}

	// Use INSERT ... ON CONFLICT to handle existing time entries during full sync (PostgreSQL equivalent of INSERT OR REPLACE)
	insertStatement, err := db.Prepare(`INSERT INTO time_entries 
		(id, task_id, user_id, date, start_time, end_time, duration, description, billable, locked, modify_time) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) 
		ON CONFLICT (id) DO UPDATE SET 
		task_id = EXCLUDED.task_id,
		user_id = EXCLUDED.user_id,
		date = EXCLUDED.date,
		start_time = EXCLUDED.start_time,
		end_time = EXCLUDED.end_time,
		duration = EXCLUDED.duration,
		description = EXCLUDED.description,
		billable = EXCLUDED.billable,
		locked = EXCLUDED.locked,
		modify_time = EXCLUDED.modify_time`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer CloseWithErrorLog(insertStatement, "prepared statement")

	// Pre-process entries to validate and convert data types
	validEntries := make([]ProcessedTimeEntry, 0, len(timeEntries))
	invalidCount := 0

	for _, entry := range timeEntries {
		processed, err := processTimeEntry(entry, logger)
		if err != nil {
			logger.Warnf("Failed to process time entry %s: %v, skipping", entry.ID, err)
			invalidCount++
			continue
		}
		validEntries = append(validEntries, processed)
	}

	if len(validEntries) == 0 {
		logger.Warnf("No valid time entries to process after validation")
		return nil
	}

	logger.Infof("Processing %d valid entries (%d invalid entries skipped) for full sync", len(validEntries), invalidCount)

	// Batch insert valid entries
	successCount := 0
	errorCount := 0

	for _, processed := range validEntries {
		_, err = insertStatement.Exec(
			processed.ID, processed.TaskID, processed.UserID, processed.Date,
			processed.Start, processed.End, processed.Duration, processed.Description,
			processed.Billable, processed.Locked, processed.ModifyTime,
		)
		if err != nil {
			logger.Errorf("Failed to insert time entry %d: %v", processed.ID, err)
			errorCount++
			continue
		}
		successCount++
	}

	logger.Infof("Full time entries sync completed: %d entries processed successfully, %d errors encountered", successCount, errorCount)

	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all time entry operations failed during full sync")
	}

	return nil
}

// getTimecampTasksFull fetches ALL tasks from TimeCamp API (same as regular getTimecampTasks but with clearer naming)
func getTimecampTasksFull() ([]JsonTask, error) {
	logger := GetGlobalLogger()

	// Get TimeCamp API URL from environment variable or use default
	timecampAPIURL := os.Getenv("TIMECAMP_API_URL")
	if timecampAPIURL == "" {
		timecampAPIURL = "https://app.timecamp.com/third_party/api"
	}
	getAllTasksURL := timecampAPIURL + "/tasks"

	// Validate API key exists
	apiKey := os.Getenv("TIMECAMP_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TIMECAMP_API_KEY environment variable not set")
	}

	authBearer := "Bearer " + apiKey

	request, err := http.NewRequest("GET", getAllTasksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Add("Authorization", authBearer)
	request.Header.Add("Accept", "application/json")

	logger.Debugf("Fetching ALL tasks from TimeCamp API: %s", getAllTasksURL)

	// Use retry mechanism for API calls
	retryConfig := DefaultRetryConfig()
	response, err := DoHTTPWithRetry(http.DefaultClient, request, retryConfig)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to TimeCamp API failed after retries (URL: %s): %w", getAllTasksURL, err)
	}
	defer CloseWithErrorLog(response.Body, "HTTP response body")

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		logger.Errorf("TimeCamp API returned status %d for tasks endpoint. Response body: %s", response.StatusCode, string(body))

		// Provide more specific error messages based on status code
		switch response.StatusCode {
		case 401:
			return nil, fmt.Errorf("TimeCamp API authentication failed (status 401) - check if TIMECAMP_API_KEY is valid")
		case 403:
			return nil, fmt.Errorf("TimeCamp API access forbidden (status 403) - check API key permissions")
		case 429:
			return nil, fmt.Errorf("TimeCamp API rate limit exceeded (status 429) - try again later")
		case 500, 502, 503, 504:
			return nil, fmt.Errorf("TimeCamp API server error (status %d) - service may be temporarily unavailable", response.StatusCode)
		default:
			return nil, fmt.Errorf("TimeCamp API returned status %d: %s", response.StatusCode, string(body))
		}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		logger.Warn("Empty response from TimeCamp API")
		return []JsonTask{}, nil
	}

	// Unmarshal into a map first
	taskMap := make(map[string]JsonTask)
	if err := json.Unmarshal(body, &taskMap); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response from TimeCamp: %w", err)
	}

	// Convert the map to a slice
	tasks := make([]JsonTask, 0, len(taskMap))
	for _, task := range taskMap {
		tasks = append(tasks, task)
	}

	logger.Debugf("Successfully fetched %d tasks from TimeCamp for full sync", len(tasks))

	return tasks, nil
}

// getTimeCampTimeEntriesFull fetches time entries from TimeCamp API for the specified date range (same as regular function but with clearer naming)
func getTimeCampTimeEntriesFull(fromDate, toDate string) ([]JsonTimeEntry, error) {
	logger := GetGlobalLogger()

	// Get TimeCamp API URL from environment variable or use default
	timecampAPIURL := os.Getenv("TIMECAMP_API_URL")
	if timecampAPIURL == "" {
		timecampAPIURL = "https://app.timecamp.com/third_party/api"
	}
	getTimeEntriesURL := fmt.Sprintf("%s/entries", timecampAPIURL)

	// Validate API key exists
	apiKey := os.Getenv("TIMECAMP_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TIMECAMP_API_KEY environment variable not set")
	}

	authBearer := "Bearer " + apiKey

	request, err := http.NewRequest("GET", getTimeEntriesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add query parameters
	q := request.URL.Query()
	q.Add("from", fromDate)
	q.Add("to", toDate)
	q.Add("format", "json")
	request.URL.RawQuery = q.Encode()

	request.Header.Add("Authorization", authBearer)
	request.Header.Add("Accept", "application/json")

	logger.Debugf("Fetching ALL time entries from TimeCamp API: %s", request.URL.String())

	// Use retry mechanism for API calls
	retryConfig := DefaultRetryConfig()
	response, err := DoHTTPWithRetry(http.DefaultClient, request, retryConfig)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to TimeCamp API failed after retries: %w", err)
	}
	defer CloseWithErrorLog(response.Body, "HTTP response body")

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("TimeCamp API returned status %d: %s", response.StatusCode, string(body))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		logger.Warn("Empty response from TimeCamp API")
		return []JsonTimeEntry{}, nil
	}

	// Try to unmarshal directly to JsonTimeEntry first
	var timeEntries []JsonTimeEntry
	if err := json.Unmarshal(body, &timeEntries); err == nil {
		logger.Debugf("Successfully parsed %d time entries directly for full sync", len(timeEntries))
		return timeEntries, nil
	}

	// Fallback to the flexible parsing if direct unmarshaling fails
	logger.Debug("Direct unmarshaling failed, using flexible parsing for full sync")
	var timeEntriesRaw []map[string]interface{}
	if err := json.Unmarshal(body, &timeEntriesRaw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response from TimeCamp: %w", err)
	}

	// Convert the raw data to our structured format
	timeEntries = make([]JsonTimeEntry, 0, len(timeEntriesRaw))
	for _, rawEntry := range timeEntriesRaw {
		entry := JsonTimeEntry{
			ID:          safeStringConvert(rawEntry["id"]),
			TaskID:      safeStringConvert(rawEntry["task_id"]),
			UserID:      safeStringConvert(rawEntry["user_id"]),
			Date:        safeStringConvert(rawEntry["date"]),
			Start:       safeStringConvert(rawEntry["start_time"]),
			End:         safeStringConvert(rawEntry["end_time"]),
			Duration:    safeStringConvert(rawEntry["duration"]),
			Description: safeStringConvert(rawEntry["description"]),
			Billable:    safeStringConvert(rawEntry["billable"]),
			Locked:      safeStringConvert(rawEntry["locked"]),
			ModifyTime:  safeStringConvert(rawEntry["modify_time"]),
		}
		timeEntries = append(timeEntries, entry)
	}

	logger.Debugf("Successfully fetched %d time entries from TimeCamp for full sync", len(timeEntries))

	return timeEntries, nil
}

// FullSyncAll performs both full tasks sync and full time entries sync
func FullSyncAll() error {
	logger := GetGlobalLogger()

	logger.Info("Starting full synchronization of all data from TimeCamp")

	// Validate database write access before attempting sync operations
	logger.Debug("Validating database write access...")
	if err := validateDatabaseWriteAccess(); err != nil {
		return fmt.Errorf("database write validation failed: %w", err)
	}
	logger.Debug("Database write access validated successfully")

	// Sync tasks first
	logger.Info("Starting full tasks sync...")
	if err := FullSyncTasksToDatabase(); err != nil {
		return fmt.Errorf("full tasks sync failed: %w", err)
	}
	logger.Info("Full tasks sync completed successfully")

	// Then sync time entries
	logger.Info("Starting full time entries sync...")
	if err := FullSyncTimeEntriesToDatabase(); err != nil {
		return fmt.Errorf("full time entries sync failed: %w", err)
	}
	logger.Info("Full time entries sync completed successfully")

	logger.Info("Full synchronization completed successfully")
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
		outputJSON(errorMessage)
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

	outputJSON(message)
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
