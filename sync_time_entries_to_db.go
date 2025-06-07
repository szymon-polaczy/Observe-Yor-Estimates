package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// JsonTimeEntry represents a time entry from TimeCamp API
// Some fields come as strings from the API even though they represent numbers
type JsonTimeEntry struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	UserID      string `json:"user_id"`
	Date        string `json:"date"`
	Start       string `json:"start_time"`
	End         string `json:"end_time"`
	Duration    string `json:"duration"`
	Description string `json:"description"`
	Billable    string `json:"billable"`
	Locked      string `json:"locked"`
	ModifyTime  string `json:"modify_time"`
}

// ProcessedTimeEntry represents a time entry with validated and converted data types
type ProcessedTimeEntry struct {
	ID          int
	TaskID      int
	UserID      int
	Date        string
	Start       string
	End         string
	Duration    int
	Description string
	Billable    int
	Locked      int
	ModifyTime  string
}

// TimeEntryInfo represents aggregated time information for a task
type TimeEntryInfo struct {
	TaskID           int
	TaskName         string
	YesterdaySeconds int
	TodaySeconds     int
	FirstEntryTime   *time.Time
}

// processTimeEntry validates and converts a JsonTimeEntry to ProcessedTimeEntry
func processTimeEntry(entry JsonTimeEntry, logger interface{ Warnf(string, ...interface{}) }) (ProcessedTimeEntry, error) {
	var processed ProcessedTimeEntry

	// Parse ID with scientific notation support
	idFloat, err := strconv.ParseFloat(entry.ID, 64)
	if err != nil {
		return processed, fmt.Errorf("invalid ID '%s': %w", entry.ID, err)
	}
	processed.ID = int(idFloat)

	// Parse required integer fields - these should fail if invalid
	processed.TaskID, err = strconv.Atoi(entry.TaskID)
	if err != nil {
		return processed, fmt.Errorf("invalid TaskID '%s': %w", entry.TaskID, err)
	}

	processed.UserID, err = strconv.Atoi(entry.UserID)
	if err != nil {
		return processed, fmt.Errorf("invalid UserID '%s': %w", entry.UserID, err)
	}

	// Parse optional integer fields - default to 0 if invalid
	if processed.Duration, err = strconv.Atoi(entry.Duration); err != nil {
		logger.Warnf("Invalid duration '%s' for entry %s, defaulting to 0", entry.Duration, entry.ID)
		processed.Duration = 0
	}

	if processed.Billable, err = strconv.Atoi(entry.Billable); err != nil {
		logger.Warnf("Invalid billable '%s' for entry %s, defaulting to 0", entry.Billable, entry.ID)
		processed.Billable = 0
	}

	if processed.Locked, err = strconv.Atoi(entry.Locked); err != nil {
		logger.Warnf("Invalid locked '%s' for entry %s, defaulting to 0", entry.Locked, entry.ID)
		processed.Locked = 0
	}

	// Copy string fields directly
	processed.Date = entry.Date
	processed.Start = entry.Start
	processed.End = entry.End
	processed.Description = entry.Description
	processed.ModifyTime = entry.ModifyTime

	return processed, nil
}

// safeStringConvert safely converts an interface{} to string, handling nil values
func safeStringConvert(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// SyncTimeEntriesToDatabase fetches recent time entries from TimeCamp and stores them in the database
func SyncTimeEntriesToDatabase() error {
	logger := NewLogger()

	// Load environment variables - but don't panic here since main already validated them
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting time entries synchronization with TimeCamp")

	// Get time entries from the last 7 days to ensure we capture recent changes
	fromDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	toDate := time.Now().Format("2006-01-02")

	timeEntries, err := getTimeCampTimeEntries(fromDate, toDate)
	if err != nil {
		return fmt.Errorf("failed to fetch time entries from TimeCamp: %w", err)
	}

	if len(timeEntries) == 0 {
		logger.Info("No time entries received from TimeCamp API")
		return nil // Not an error, just no data
	}

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer CloseWithErrorLog(db, "database connection")

	// Ensure time_entries table exists
	if err := migrateTimeEntriesTable(db); err != nil {
		return fmt.Errorf("failed to migrate time_entries table: %w", err)
	}

	// Use INSERT OR REPLACE to handle existing time entries
	insertStatement, err := db.Prepare(`INSERT OR REPLACE INTO time_entries 
		(id, task_id, user_id, date, start_time, end_time, duration, description, billable, locked, modify_time) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
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

	logger.Infof("Processing %d valid entries (%d invalid entries skipped)", len(validEntries), invalidCount)

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

	logger.Infof("Time entries sync completed: %d entries processed successfully, %d errors encountered", successCount, errorCount)

	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all time entry operations failed during sync")
	}

	return nil
}

// getTimeCampTimeEntries fetches time entries from TimeCamp API
func getTimeCampTimeEntries(fromDate, toDate string) ([]JsonTimeEntry, error) {
	logger := NewLogger()

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

	logger.Debugf("Fetching time entries from TimeCamp API: %s", request.URL.String())

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to TimeCamp API failed: %w", err)
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
		logger.Debugf("Successfully parsed %d time entries directly", len(timeEntries))
		return timeEntries, nil
	}

	// Fallback to the flexible parsing if direct unmarshaling fails
	logger.Debug("Direct unmarshaling failed, using flexible parsing")
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

	logger.Debugf("Successfully fetched %d time entries from TimeCamp", len(timeEntries))

	return timeEntries, nil
}

// GetTaskTimeEntries retrieves time entry information for tasks, aggregated by date
func GetTaskTimeEntries(db *sql.DB) ([]TaskTimeInfo, error) {
	logger := NewLogger()

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	query := `
		WITH task_time_data AS (
			SELECT 
				t.task_id,
				t.name,
				COALESCE(SUM(CASE WHEN te.date = ? THEN te.duration ELSE 0 END), 0) as yesterday_seconds,
				COALESCE(SUM(CASE WHEN te.date = ? THEN te.duration ELSE 0 END), 0) as today_seconds,
				MIN(te.start_time) as first_entry_time
			FROM tasks t
			LEFT JOIN time_entries te ON t.task_id = te.task_id 
				AND te.date IN (?, ?)
			WHERE te.task_id IS NOT NULL
			GROUP BY t.task_id, t.name
			HAVING yesterday_seconds > 0 OR today_seconds > 0
		)
		SELECT 
			task_id,
			name,
			yesterday_seconds,
			today_seconds,
			first_entry_time
		FROM task_time_data
		ORDER BY (yesterday_seconds + today_seconds) DESC
		LIMIT 20
	`

	logger.Debugf("Querying time entries for yesterday (%s) and today (%s)", yesterday, today)

	rows, err := db.Query(query, yesterday, today, yesterday, today)
	if err != nil {
		return nil, fmt.Errorf("error querying time entries: %w", err)
	}
	defer CloseWithErrorLog(rows, "database rows")

	var taskInfos []TaskTimeInfo
	errorCount := 0

	for rows.Next() {
		var taskInfo TaskTimeInfo
		var yesterdaySeconds, todaySeconds int
		var firstEntryTimeStr sql.NullString

		err := rows.Scan(
			&taskInfo.TaskID,
			&taskInfo.Name,
			&yesterdaySeconds,
			&todaySeconds,
			&firstEntryTimeStr,
		)
		if err != nil {
			logger.Errorf("Error scanning time entry row: %v", err)
			errorCount++
			continue
		}

		// Convert seconds to duration strings
		taskInfo.YesterdayTime = formatDuration(yesterdaySeconds)
		taskInfo.TodayTime = formatDuration(todaySeconds)

		// Handle start time
		if firstEntryTimeStr.Valid {
			if parsedTime, parseErr := time.Parse("15:04:05", firstEntryTimeStr.String); parseErr == nil {
				taskInfo.StartTime = parsedTime.Format("15:04")
			} else {
				taskInfo.StartTime = firstEntryTimeStr.String
			}
		} else {
			taskInfo.StartTime = "N/A"
		}

		// Extract estimation information from task name
		taskInfo.EstimationInfo, taskInfo.EstimationStatus = parseEstimation(taskInfo.Name)

		taskInfos = append(taskInfos, taskInfo)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	if errorCount > 0 {
		logger.Warnf("Encountered %d errors while processing time entry rows", errorCount)
	}

	logger.Debugf("Successfully retrieved %d task time entries", len(taskInfos))
	return taskInfos, nil
}

// formatDuration converts seconds to a human-readable duration string
func formatDuration(seconds int) string {
	if seconds == 0 {
		return "0h 0m"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
