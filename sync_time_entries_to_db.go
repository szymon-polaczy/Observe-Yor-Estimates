package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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
// This function is optimized for regular cron jobs and only syncs the last day's entries
func SyncTimeEntriesToDatabase() error {
	logger := GetGlobalLogger()

	// Load environment variables - but don't panic here since main already validated them
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting time entries synchronization with TimeCamp")

	// Get time entries from the last day only (optimized for cron jobs)
	fromDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
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
	// Note: Using shared database connection, no need to close here

	// Ensure time_entries table exists
	if err := migrateTimeEntriesTable(db); err != nil {
		return fmt.Errorf("failed to migrate time_entries table: %w", err)
	}

	// Use INSERT ... ON CONFLICT to handle existing time entries (PostgreSQL equivalent of INSERT OR REPLACE)
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

	logger.Debugf("Fetching time entries from TimeCamp API: %s", request.URL.String())

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
func GetTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for daily task time entries")

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	rows, err := db.Query(`
		SELECT
			t.task_id,
			t.name,
			SUM(CASE WHEN te.date = $1 THEN te.duration ELSE 0 END) as today_seconds,
			SUM(CASE WHEN te.date = $2 THEN te.duration ELSE 0 END) as yesterday_seconds
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id AND te.date IN ($1, $2)
		GROUP BY t.task_id, t.name
		HAVING SUM(CASE WHEN te.date = $1 THEN te.duration ELSE 0 END) > 0 OR SUM(CASE WHEN te.date = $2 THEN te.duration ELSE 0 END) > 0
		ORDER BY t.name;
	`, today, yesterday)
	if err != nil {
		return nil, fmt.Errorf("failed to query task time entries: %w", err)
	}
	defer rows.Close()

	var tasks []TaskUpdateInfo
	for rows.Next() {
		var taskID int
		var name string
		var todaySeconds, yesterdaySeconds int
		if err := rows.Scan(&taskID, &name, &todaySeconds, &yesterdaySeconds); err != nil {
			return nil, fmt.Errorf("failed to scan task time entry: %w", err)
		}

		comments, err := getTaskComments(db, taskID, yesterday, today)
		if err != nil {
			logger.Warnf("failed to get comments for task %d: %v", taskID, err)
		}

		estimation, status := parseEstimation(name)

		tasks = append(tasks, TaskUpdateInfo{
			Name:             name,
			CurrentPeriod:    "Today",
			CurrentTime:      formatDuration(todaySeconds),
			PreviousPeriod:   "Yesterday",
			PreviousTime:     formatDuration(yesterdaySeconds),
			Comments:         comments,
			EstimationInfo:   estimation,
			EstimationStatus: status,
		})
	}

	return tasks, nil
}

// GetWeeklyTaskTimeEntries retrieves weekly time entry information for tasks
func GetWeeklyTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for weekly task time entries")

	thisWeekStart := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)

	rows, err := db.Query(`
		SELECT
			t.task_id,
			t.name,
			SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) as this_week_seconds,
			SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END) as last_week_seconds,
			COUNT(DISTINCT te.date) as days_worked
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		GROUP BY t.task_id, t.name
		HAVING SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) > 0 OR SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END) > 0
		ORDER BY (SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) + SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END)) DESC;
	`, thisWeekStart.Format("2006-01-02"), lastWeekStart.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query weekly task time entries: %w", err)
	}
	defer rows.Close()

	var tasks []TaskUpdateInfo
	for rows.Next() {
		var taskID int
		var name string
		var thisWeekSeconds, lastWeekSeconds, daysWorked int
		if err := rows.Scan(&taskID, &name, &thisWeekSeconds, &lastWeekSeconds, &daysWorked); err != nil {
			return nil, fmt.Errorf("failed to scan weekly task time entry: %w", err)
		}

		comments, err := getTaskComments(db, taskID, lastWeekStart.Format("2006-01-02"), time.Now().Format("2006-01-02"))
		if err != nil {
			logger.Warnf("failed to get comments for task %d: %v", taskID, err)
		}

		estimation, status := parseEstimation(name)

		tasks = append(tasks, TaskUpdateInfo{
			Name:             name,
			CurrentPeriod:    "This Week",
			CurrentTime:      formatDuration(thisWeekSeconds),
			PreviousPeriod:   "Last Week",
			PreviousTime:     formatDuration(lastWeekSeconds),
			DaysWorked:       daysWorked,
			Comments:         comments,
			EstimationInfo:   estimation,
			EstimationStatus: status,
		})
	}
	return tasks, nil
}

// GetMonthlyTaskTimeEntries retrieves monthly time entry information for tasks
func GetMonthlyTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for monthly task time entries")

	today := time.Now()
	thisMonthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	lastMonthStart := thisMonthStart.AddDate(0, -1, 0)

	rows, err := db.Query(`
		SELECT
			t.task_id,
			t.name,
			SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) as this_month_seconds,
			SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END) as last_month_seconds,
			COUNT(DISTINCT te.date) as days_worked
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		GROUP BY t.task_id, t.name
		HAVING SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) > 0 OR SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END) > 0
		ORDER BY (SUM(CASE WHEN te.date >= $1 THEN te.duration ELSE 0 END) + SUM(CASE WHEN te.date >= $2 AND te.date < $1 THEN te.duration ELSE 0 END)) DESC;
	`, thisMonthStart.Format("2006-01-02"), lastMonthStart.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query monthly task time entries: %w", err)
	}
	defer rows.Close()

	var tasks []TaskUpdateInfo
	for rows.Next() {
		var taskID int
		var name string
		var thisMonthSeconds, lastMonthSeconds, daysWorked int
		if err := rows.Scan(&taskID, &name, &thisMonthSeconds, &lastMonthSeconds, &daysWorked); err != nil {
			return nil, fmt.Errorf("failed to scan monthly task time entry: %w", err)
		}

		comments, err := getTaskComments(db, taskID, lastMonthStart.Format("2006-01-02"), time.Now().Format("2006-01-02"))
		if err != nil {
			logger.Warnf("failed to get comments for task %d: %v", taskID, err)
		}

		estimation, status := parseEstimation(name)

		tasks = append(tasks, TaskUpdateInfo{
			Name:             name,
			CurrentPeriod:    "This Month",
			CurrentTime:      formatDuration(thisMonthSeconds),
			PreviousPeriod:   "Last Month",
			PreviousTime:     formatDuration(lastMonthSeconds),
			DaysWorked:       daysWorked,
			Comments:         comments,
			EstimationInfo:   estimation,
			EstimationStatus: status,
		})
	}
	return tasks, nil
}

// formatDuration formats seconds into a human-readable string like "1h 30m"
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

// getTaskComments retrieves unique comments for a task within a specific date range
func getTaskComments(db *sql.DB, taskID int, fromDate, toDate string) ([]string, error) {
	logger := GetGlobalLogger()

	query := `
		SELECT DISTINCT description 
		FROM time_entries 
		WHERE task_id = $1 
		AND date BETWEEN $2 AND $3 
		AND description IS NOT NULL 
		AND TRIM(description) != ''
		ORDER BY description
	`

	rows, err := db.Query(query, taskID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("error querying task comments: %w", err)
	}
	defer CloseWithErrorLog(rows, "database rows")

	var comments []string
	for rows.Next() {
		var comment string
		err := rows.Scan(&comment)
		if err != nil {
			logger.Errorf("Error scanning comment row: %v", err)
			continue
		}

		// Only add non-empty comments
		if strings.TrimSpace(comment) != "" {
			comments = append(comments, strings.TrimSpace(comment))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during comment rows iteration: %w", err)
	}

	logger.Debugf("Retrieved %d comments for task %d between %s and %s", len(comments), taskID, fromDate, toDate)
	return comments, nil
}
