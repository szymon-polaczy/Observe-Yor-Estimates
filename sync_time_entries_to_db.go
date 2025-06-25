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
	"github.com/lib/pq"
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

// SyncTimeEntriesToDatabase fetches time entries from TimeCamp and stores them in the database
// If fromDate and toDate are provided, uses those dates; otherwise defaults to last day (optimized for cron jobs)
func SyncTimeEntriesToDatabase(fromDate, toDate string) error {
	return SyncTimeEntriesToDatabaseWithOptions(fromDate, toDate, false)
}

// SyncTimeEntriesToDatabaseWithOptions provides more control over the sync behavior
// includeOrphaned: if true, stores orphaned time entries for later processing (useful during full sync)
func SyncTimeEntriesToDatabaseWithOptions(fromDate, toDate string, includeOrphaned bool) error {
	logger := GetGlobalLogger()

	// Load environment variables - but don't panic here since main already validated them
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting time entries synchronization with TimeCamp")

	// Use provided dates or default to last day only (optimized for cron jobs)
	if fromDate == "" || toDate == "" {
		fromDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		toDate = time.Now().Format("2006-01-02")
		logger.Debugf("Using default date range for cron sync: %s to %s", fromDate, toDate)
	} else {
		logger.Infof("Using custom date range: %s to %s", fromDate, toDate)
	}

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

	// Ensure orphaned_time_entries table exists if we're including orphaned entries
	if includeOrphaned {
		if err := ensureOrphanedTimeEntriesTable(db); err != nil {
			return fmt.Errorf("failed to ensure orphaned_time_entries table: %w", err)
		}
	}

	// Prepare insert statement (ON CONFLICT to upsert)
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

	// Prepare orphaned insert statement if needed
	var orphanedInsertStatement *sql.Stmt
	if includeOrphaned {
		orphanedInsertStatement, err = db.Prepare(`INSERT INTO orphaned_time_entries 
			(id, task_id, user_id, date, start_time, end_time, duration, description, billable, locked, modify_time, sync_date) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) 
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
			modify_time = EXCLUDED.modify_time,
			sync_date = EXCLUDED.sync_date`)
		if err != nil {
			return fmt.Errorf("failed to prepare orphaned insert statement: %w", err)
		}
		defer CloseWithErrorLog(orphanedInsertStatement, "orphaned prepared statement")
	}

	// Build an optimized in-memory set of existing task IDs to avoid per-entry queries
	existingTasks := make(map[int]struct{})
	rowsTasks, err := db.Query(`SELECT task_id FROM tasks`)
	if err != nil {
		return fmt.Errorf("failed to fetch existing tasks: %w", err)
	}
	defer rowsTasks.Close()

	for rowsTasks.Next() {
		var id int
		if err := rowsTasks.Scan(&id); err == nil {
			existingTasks[id] = struct{}{}
		}
	}

	logger.Debugf("Loaded %d existing task IDs for validation", len(existingTasks))

	// Pre-process entries to validate and convert data types
	validEntries := make([]ProcessedTimeEntry, 0, len(timeEntries))
	orphanedEntries := make([]ProcessedTimeEntry, 0)
	invalidCount := 0
	missingTaskCount := 0

	for _, entry := range timeEntries {
		processed, err := processTimeEntry(entry, logger)
		if err != nil {
			logger.Warnf("Failed to process time entry %s: %v, skipping", entry.ID, err)
			invalidCount++
			continue
		}

		// Check if task exists in the pre-fetched map
		if _, ok := existingTasks[processed.TaskID]; !ok {
			missingTaskCount++
			if includeOrphaned {
				// Store orphaned entry for later processing
				orphanedEntries = append(orphanedEntries, processed)
				continue
			}
			// Skip orphaned entries if not in full sync mode
			continue
		}

		validEntries = append(validEntries, processed)
	}

	if len(validEntries) == 0 && len(orphanedEntries) == 0 {
		logger.Warnf("No valid time entries to process after validation (total: %d, invalid: %d, missing tasks: %d)", len(timeEntries), invalidCount, missingTaskCount)
		return nil
	}

	logger.Infof("Processing %d valid entries (%d invalid entries skipped, %d entries with missing tasks)", len(validEntries), invalidCount, missingTaskCount)

	if includeOrphaned && len(orphanedEntries) > 0 {
		logger.Infof("Storing %d orphaned time entries for later processing", len(orphanedEntries))
	}

	// Use optimized batch processing for better performance
	err = processBatchTimeEntries(db, validEntries, logger)
	if err != nil {
		return err
	}

	// Process orphaned entries if in full sync mode
	if includeOrphaned && len(orphanedEntries) > 0 {
		err = processBatchOrphanedTimeEntries(db, orphanedEntries, orphanedInsertStatement, logger)
		if err != nil {
			logger.Errorf("Failed to store orphaned time entries: %v", err)
			// Don't fail the entire sync for orphaned entries
		}
	}

	// Check for missing task references and suggest remediation
	if missingTaskCount > 0 && !includeOrphaned {
		logger.Warnf("Warning: %d time entries were skipped due to missing task references. Consider running a full tasks sync if this number is high.", missingTaskCount)
		checkMissingTasksAndSuggestRemediation(db, missingTaskCount, logger)
	}

	return nil
}

// processBatchTimeEntries performs optimized batch insert of time entries
func processBatchTimeEntries(db *sql.DB, entries []ProcessedTimeEntry, logger *Logger) error {
	if len(entries) == 0 {
		return nil
	}

	logger.Infof("Starting optimized batch processing for %d time entries", len(entries))

	// Start a transaction for better performance
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Will be ignored if tx.Commit() succeeds

	// Prepare the UPSERT statement
	stmt, err := tx.Prepare(`INSERT INTO time_entries 
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
		return fmt.Errorf("failed to prepare batch statement: %w", err)
	}
	defer stmt.Close()

	// Process entries in batches
	const batchSize = 500 // Larger batch size for time entries since they're simpler
	successCount := 0
	errorCount := 0

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		logger.Debugf("Processing time entries batch %d-%d of %d", i+1, end, len(entries))

		for _, entry := range batch {
			_, err = stmt.Exec(
				entry.ID, entry.TaskID, entry.UserID, entry.Date,
				entry.Start, entry.End, entry.Duration, entry.Description,
				entry.Billable, entry.Locked, entry.ModifyTime,
			)
			if err != nil {
				logger.Errorf("Failed to upsert time entry %d: %v", entry.ID, err)
				errorCount++
				continue
			}
			successCount++
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Infof("Optimized time entries sync completed: %d entries processed successfully, %d errors encountered", successCount, errorCount)

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

	// Use optimized HTTP client for better performance
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Use retry mechanism for API calls
	retryConfig := DefaultRetryConfig()
	response, err := DoHTTPWithRetry(client, request, retryConfig)
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

// checkMissingTasksAndSuggestRemediation analyzes missing task references and suggests remediation steps
func checkMissingTasksAndSuggestRemediation(db *sql.DB, missingTaskCount int, logger interface{ Warnf(string, ...interface{}) }) {
	if missingTaskCount == 0 {
		return
	}

	// Check if we have any tasks in the database at all
	var taskCount int
	err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&taskCount)
	if err != nil {
		logger.Warnf("Could not check task count in database: %v", err)
		return
	}

	if taskCount == 0 {
		logger.Warnf("REMEDIATION SUGGESTION: No tasks found in database. Run a full tasks sync first: `./bin/observe-yor-estimates sync-tasks` or `./bin/observe-yor-estimates full-sync`")
	} else if missingTaskCount > 10 {
		logger.Warnf("REMEDIATION SUGGESTION: High number of missing task references (%d). Consider running a full tasks sync to ensure all tasks are up to date: `./bin/observe-yor-estimates sync-tasks`", missingTaskCount)
	} else {
		logger.Warnf("REMEDIATION SUGGESTION: Some time entries reference missing tasks (%d). This may be normal if tasks were deleted/archived in TimeCamp but still have historical time entries.", missingTaskCount)
	}
}

// GetTaskTimeEntries retrieves time entry information for tasks, aggregated by date
func GetTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for daily task time entries")

	// First get user breakdown data
	userBreakdownQuery := `
WITH yesterday AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date = CURRENT_DATE - INTERVAL '1 day'
    GROUP BY task_id, user_id
),
day_before AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date = CURRENT_DATE - INTERVAL '2 days'
    GROUP BY task_id, user_id
)
SELECT 
    COALESCE(y.task_id, db.task_id) AS task_id,
    COALESCE(y.user_id, db.user_id) AS user_id,
    COALESCE(y.total_duration, 0) AS yesterday_duration, 
    COALESCE(db.total_duration, 0) AS day_before_duration
FROM yesterday y
FULL OUTER JOIN day_before db ON y.task_id = db.task_id AND y.user_id = db.user_id
WHERE COALESCE(y.total_duration, 0) > 0;
`

	userRows, err := db.Query(userBreakdownQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query user breakdown: %w", err)
	}
	defer userRows.Close()

	// Build user breakdown map: taskID -> userID -> contribution
	userBreakdowns := make(map[int]map[int]UserTimeContribution)
	for userRows.Next() {
		var taskID, userID, yesterdayDuration, dayBeforeDuration int
		err := userRows.Scan(&taskID, &userID, &yesterdayDuration, &dayBeforeDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user breakdown row: %w", err)
		}

		if _, exists := userBreakdowns[taskID]; !exists {
			userBreakdowns[taskID] = make(map[int]UserTimeContribution)
		}

		userBreakdowns[taskID][userID] = UserTimeContribution{
			UserID:       userID,
			CurrentTime:  formatDuration(yesterdayDuration),
			PreviousTime: formatDuration(dayBeforeDuration),
		}
	}

	// Now get aggregated task data
	query := `
WITH yesterday AS (
    SELECT task_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date = CURRENT_DATE - INTERVAL '1 day'
    GROUP BY task_id
),
day_before AS (
    SELECT task_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date = CURRENT_DATE - INTERVAL '2 days'
    GROUP BY task_id
)
SELECT 
    t.name, 
    COALESCE(y.total_duration, 0) AS yesterday_duration, 
    COALESCE(db.total_duration, 0) AS day_before_duration,
    (SELECT string_agg(description, ' | ') FROM time_entries te WHERE te.task_id = t.task_id AND te.date::date = CURRENT_DATE - INTERVAL '1 day'),
    t.task_id,
    t.parent_id
FROM tasks t
LEFT JOIN yesterday y ON t.task_id = y.task_id
LEFT JOIN day_before db ON t.task_id = db.task_id
WHERE COALESCE(y.total_duration, 0) > 0;
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily task time entries: %w", err)
	}
	defer rows.Close()

	var taskInfos []TaskUpdateInfo
	for rows.Next() {
		var info TaskUpdateInfo
		var yesterdayDuration, dayBeforeDuration int
		var comments sql.NullString
		err := rows.Scan(&info.Name, &yesterdayDuration, &dayBeforeDuration, &comments, &info.TaskID, &info.ParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task time entry row: %w", err)
		}
		info.CurrentPeriod = "Yesterday"
		info.CurrentTime = formatDuration(yesterdayDuration)
		info.PreviousPeriod = "Day Before"
		info.PreviousTime = formatDuration(dayBeforeDuration)
		info.EstimationInfo, info.EstimationStatus = parseEstimationWithUsage(info.Name, info.CurrentTime, info.PreviousTime)
		if comments.Valid {
			info.Comments = strings.Split(comments.String, " | ")
		}
		
		// Add user breakdown
		if breakdown, exists := userBreakdowns[info.TaskID]; exists {
			info.UserBreakdown = breakdown
		}
		
		taskInfos = append(taskInfos, info)
	}

	// Extract task IDs for bulk comment fetching
	var taskIDs []int
	for _, info := range taskInfos {
		taskIDs = append(taskIDs, info.TaskID)
	}

	// Bulk fetch comments for all tasks found
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	commentsMap, err := getTaskCommentsBulk(db, taskIDs, yesterday, today)
	if err != nil {
		logger.Warnf("failed to bulk fetch comments: %v", err)
	}

	// Attach comments
	for i, info := range taskInfos {
		if comments, ok := commentsMap[info.TaskID]; ok {
			taskInfos[i].Comments = comments
		}
	}

	return taskInfos, nil
}

// GetWeeklyTaskTimeEntries retrieves weekly time entry information for tasks
func GetWeeklyTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for weekly task time entries")

	// First get user breakdown data
	userBreakdownQuery := `
WITH current_week AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '7 days' AND date::date < CURRENT_DATE
    GROUP BY task_id, user_id
),
previous_week AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '14 days' AND date::date < CURRENT_DATE - INTERVAL '7 days'
    GROUP BY task_id, user_id
)
SELECT 
    COALESCE(cw.task_id, pw.task_id) AS task_id,
    COALESCE(cw.user_id, pw.user_id) AS user_id,
    COALESCE(cw.total_duration, 0) AS current_week_duration, 
    COALESCE(pw.total_duration, 0) AS previous_week_duration
FROM current_week cw
FULL OUTER JOIN previous_week pw ON cw.task_id = pw.task_id AND cw.user_id = pw.user_id
WHERE COALESCE(cw.total_duration, 0) > 0;
`

	userRows, err := db.Query(userBreakdownQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query weekly user breakdown: %w", err)
	}
	defer userRows.Close()

	// Build user breakdown map: taskID -> userID -> contribution
	userBreakdowns := make(map[int]map[int]UserTimeContribution)
	for userRows.Next() {
		var taskID, userID, currentWeekDuration, previousWeekDuration int
		err := userRows.Scan(&taskID, &userID, &currentWeekDuration, &previousWeekDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan weekly user breakdown row: %w", err)
		}

		if _, exists := userBreakdowns[taskID]; !exists {
			userBreakdowns[taskID] = make(map[int]UserTimeContribution)
		}

		userBreakdowns[taskID][userID] = UserTimeContribution{
			UserID:       userID,
			CurrentTime:  formatDuration(currentWeekDuration),
			PreviousTime: formatDuration(previousWeekDuration),
		}
	}

	// Weekly changes: compare last 7 days with the 7 days before that
	query := `
WITH current_week AS (
    SELECT task_id, SUM(duration) AS total_duration, COUNT(DISTINCT date) as days_worked
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '7 days' AND date::date < CURRENT_DATE
    GROUP BY task_id
),
previous_week AS (
    SELECT task_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '14 days' AND date::date < CURRENT_DATE - INTERVAL '7 days'
    GROUP BY task_id
)
SELECT 
    t.name, 
    COALESCE(cw.total_duration, 0) AS current_week_duration, 
    COALESCE(pw.total_duration, 0) AS previous_week_duration,
    COALESCE(cw.days_worked, 0) as days_worked,
    (SELECT string_agg(description, ' | ') FROM time_entries te WHERE te.task_id = t.task_id AND te.date::date >= CURRENT_DATE - INTERVAL '7 days' AND te.date::date < CURRENT_DATE),
    t.task_id,
    t.parent_id
FROM tasks t
LEFT JOIN current_week cw ON t.task_id = cw.task_id
LEFT JOIN previous_week pw ON t.task_id = pw.task_id
WHERE COALESCE(cw.total_duration, 0) > 0;
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query weekly task time entries: %w", err)
	}
	defer rows.Close()

	var taskInfos []TaskUpdateInfo
	for rows.Next() {
		var info TaskUpdateInfo
		var currentWeekDuration, previousWeekDuration int
		var comments sql.NullString
		err := rows.Scan(
			&info.Name,
			&currentWeekDuration,
			&previousWeekDuration,
			&info.DaysWorked,
			&comments,
			&info.TaskID,
			&info.ParentID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan weekly task time entry row: %w", err)
		}
		info.CurrentPeriod = "This Week"
		info.CurrentTime = formatDuration(currentWeekDuration)
		info.PreviousPeriod = "Last Week"
		info.PreviousTime = formatDuration(previousWeekDuration)
		info.EstimationInfo, info.EstimationStatus = parseEstimationWithUsage(info.Name, info.CurrentTime, info.PreviousTime)
		if comments.Valid {
			info.Comments = strings.Split(comments.String, " | ")
		}
		
		// Add user breakdown
		if breakdown, exists := userBreakdowns[info.TaskID]; exists {
			info.UserBreakdown = breakdown
		}
		
		taskInfos = append(taskInfos, info)
	}

	// Extract task IDs for bulk comment fetching
	var taskIDs []int
	for _, info := range taskInfos {
		taskIDs = append(taskIDs, info.TaskID)
	}

	// Bulk fetch comments
	lastWeekStart := time.Now().AddDate(0, 0, -14)
	commentsMap, err := getTaskCommentsBulk(db, taskIDs, lastWeekStart.Format("2006-01-02"), time.Now().Format("2006-01-02"))
	if err != nil {
		logger.Warnf("failed to bulk fetch comments: %v", err)
	}

	// Attach comments
	for i, info := range taskInfos {
		if comments, ok := commentsMap[info.TaskID]; ok {
			taskInfos[i].Comments = comments
		}
	}

	return taskInfos, nil
}

// GetMonthlyTaskTimeEntries retrieves monthly time entry information for tasks
func GetMonthlyTaskTimeEntries(db *sql.DB) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debug("Querying database for monthly task time entries")

	// First get user breakdown data
	userBreakdownQuery := `
WITH current_month AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '30 days' AND date::date < CURRENT_DATE
    GROUP BY task_id, user_id
),
previous_month AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '60 days' AND date::date < CURRENT_DATE - INTERVAL '30 days'
    GROUP BY task_id, user_id
)
SELECT 
    COALESCE(cm.task_id, pm.task_id) AS task_id,
    COALESCE(cm.user_id, pm.user_id) AS user_id,
    COALESCE(cm.total_duration, 0) AS current_month_duration, 
    COALESCE(pm.total_duration, 0) AS previous_month_duration
FROM current_month cm
FULL OUTER JOIN previous_month pm ON cm.task_id = pm.task_id AND cm.user_id = pm.user_id
WHERE COALESCE(cm.total_duration, 0) > 0;
`

	userRows, err := db.Query(userBreakdownQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query monthly user breakdown: %w", err)
	}
	defer userRows.Close()

	// Build user breakdown map: taskID -> userID -> contribution
	userBreakdowns := make(map[int]map[int]UserTimeContribution)
	for userRows.Next() {
		var taskID, userID, currentMonthDuration, previousMonthDuration int
		err := userRows.Scan(&taskID, &userID, &currentMonthDuration, &previousMonthDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monthly user breakdown row: %w", err)
		}

		if _, exists := userBreakdowns[taskID]; !exists {
			userBreakdowns[taskID] = make(map[int]UserTimeContribution)
		}

		userBreakdowns[taskID][userID] = UserTimeContribution{
			UserID:       userID,
			CurrentTime:  formatDuration(currentMonthDuration),
			PreviousTime: formatDuration(previousMonthDuration),
		}
	}

	// Monthly changes: compare last 30 days with the 30 days before that
	query := `
WITH current_month AS (
    SELECT task_id, SUM(duration) AS total_duration, COUNT(DISTINCT date) as days_worked
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '30 days' AND date::date < CURRENT_DATE
    GROUP BY task_id
),
previous_month AS (
    SELECT task_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= CURRENT_DATE - INTERVAL '60 days' AND date::date < CURRENT_DATE - INTERVAL '30 days'
    GROUP BY task_id
)
SELECT 
    t.name, 
    COALESCE(cm.total_duration, 0) AS current_month_duration, 
    COALESCE(pm.total_duration, 0) AS previous_month_duration,
    COALESCE(cm.days_worked, 0) as days_worked,
    (SELECT string_agg(description, ' | ') FROM time_entries te WHERE te.task_id = t.task_id AND te.date::date >= CURRENT_DATE - INTERVAL '30 days' AND te.date::date < CURRENT_DATE),
    t.task_id,
    t.parent_id
FROM tasks t
LEFT JOIN current_month cm ON t.task_id = cm.task_id
LEFT JOIN previous_month pm ON t.task_id = pm.task_id
WHERE COALESCE(cm.total_duration, 0) > 0;
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query monthly task time entries: %w", err)
	}
	defer rows.Close()

	var taskInfos []TaskUpdateInfo
	for rows.Next() {
		var info TaskUpdateInfo
		var currentMonthDuration, previousMonthDuration int
		var comments sql.NullString
		err := rows.Scan(
			&info.Name,
			&currentMonthDuration,
			&previousMonthDuration,
			&info.DaysWorked,
			&comments,
			&info.TaskID,
			&info.ParentID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monthly task time entry row: %w", err)
		}
		info.CurrentPeriod = "This Month"
		info.CurrentTime = formatDuration(currentMonthDuration)
		info.PreviousPeriod = "Last Month"
		info.PreviousTime = formatDuration(previousMonthDuration)
		info.EstimationInfo, info.EstimationStatus = parseEstimationWithUsage(info.Name, info.CurrentTime, info.PreviousTime)
		if comments.Valid {
			info.Comments = strings.Split(comments.String, " | ")
		}
		
		// Add user breakdown
		if breakdown, exists := userBreakdowns[info.TaskID]; exists {
			info.UserBreakdown = breakdown
		}
		
		taskInfos = append(taskInfos, info)
	}

	// Extract task IDs for bulk comment fetching
	var taskIDs []int
	for _, info := range taskInfos {
		taskIDs = append(taskIDs, info.TaskID)
	}

	// Bulk fetch comments
	lastMonthStart := time.Now().AddDate(0, -1, 0)
	commentsMap, err := getTaskCommentsBulk(db, taskIDs, lastMonthStart.Format("2006-01-02"), time.Now().Format("2006-01-02"))
	if err != nil {
		logger.Warnf("failed to bulk fetch comments: %v", err)
	}

	// Attach comments
	for i, info := range taskInfos {
		if comments, ok := commentsMap[info.TaskID]; ok {
			taskInfos[i].Comments = comments
		}
	}

	return taskInfos, nil
}

// formatDuration formats seconds into a human-readable string like "1h 30m"
func formatDuration(seconds int) string {
	if seconds == 0 {
		return "0h 0m"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60

	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// getTaskComments retrieves unique comments for a task within a specific date range
func getTaskComments(db *sql.DB, taskID int, fromDate, toDate string) ([]string, error) {
	logger := GetGlobalLogger()

	query := `
		SELECT description
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

func getTaskCommentsBulk(db *sql.DB, taskIDs []int, fromDate, toDate string) (map[int][]string, error) {
	logger := GetGlobalLogger()

	if len(taskIDs) == 0 {
		return map[int][]string{}, nil
	}

	// Build query using ANY($1) to leverage Postgres array parameter
	query := `SELECT task_id, ARRAY_AGG(DISTINCT TRIM(description) ORDER BY TRIM(description))
	          FROM time_entries
	          WHERE task_id = ANY($1) AND date BETWEEN $2 AND $3 AND description IS NOT NULL AND TRIM(description) != ''
	          GROUP BY task_id`

	rows, err := db.Query(query, pq.Array(taskIDs), fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("error querying bulk task comments: %w", err)
	}
	defer CloseWithErrorLog(rows, "bulk comments rows")

	commentsMap := make(map[int][]string)
	for rows.Next() {
		var taskID int
		var comments pq.StringArray
		if err := rows.Scan(&taskID, &comments); err != nil {
			logger.Errorf("error scanning bulk comments row: %v", err)
			continue
		}
		// Convert pq.StringArray to []string
		strComments := make([]string, len(comments))
		for i, c := range comments {
			strComments[i] = strings.TrimSpace(c)
		}
		commentsMap[taskID] = strComments
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating bulk comments rows: %w", err)
	}

	return commentsMap, nil
}

func parseEstimationStatus(currentSeconds, previousSeconds int) string {
	// Simple status based on whether time was logged
	if currentSeconds > 0 && previousSeconds == 0 {
		return "new"
	}
	if currentSeconds > 0 {
		return "active"
	}
	if currentSeconds == 0 && previousSeconds > 0 {
		return "stalled"
	}
	return "idle"
}

// ensureOrphanedTimeEntriesTable creates the orphaned_time_entries table if it doesn't exist
func ensureOrphanedTimeEntriesTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE IF NOT EXISTS orphaned_time_entries (
		id INTEGER PRIMARY KEY,
		task_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		date TEXT NOT NULL,
		start_time TEXT,
		end_time TEXT,
		duration INTEGER NOT NULL,
		description TEXT,
		billable INTEGER DEFAULT 0,
		locked INTEGER DEFAULT 0,
		modify_time TEXT,
		sync_date TEXT NOT NULL
	);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create orphaned_time_entries table: %w", err)
	}

	// Create index for efficient lookups
	indexSQL := `CREATE INDEX IF NOT EXISTS idx_orphaned_time_entries_task_id ON orphaned_time_entries(task_id);`
	_, err = db.Exec(indexSQL)
	if err != nil {
		logger.Warnf("Failed to create index on orphaned_time_entries.task_id: %v", err)
	}

	logger.Debug("Orphaned time entries table ensured")
	return nil
}

// processBatchOrphanedTimeEntries performs optimized batch insert of orphaned time entries
func processBatchOrphanedTimeEntries(db *sql.DB, entries []ProcessedTimeEntry, stmt *sql.Stmt, logger *Logger) error {
	if len(entries) == 0 {
		return nil
	}

	logger.Infof("Starting batch processing for %d orphaned time entries", len(entries))

	syncDate := time.Now().Format("2006-01-02 15:04:05")
	successCount := 0
	errorCount := 0

	for _, entry := range entries {
		_, err := stmt.Exec(
			entry.ID, entry.TaskID, entry.UserID, entry.Date,
			entry.Start, entry.End, entry.Duration, entry.Description,
			entry.Billable, entry.Locked, entry.ModifyTime, syncDate,
		)
		if err != nil {
			logger.Errorf("Failed to store orphaned time entry %d: %v", entry.ID, err)
			errorCount++
			continue
		}
		successCount++
	}

	logger.Infof("Orphaned time entries batch complete: %d successful, %d errors", successCount, errorCount)
	return nil
}

// ProcessOrphanedTimeEntries attempts to move orphaned time entries to the main table
// when their tasks become available (useful after task sync or when tasks are reopened)
func ProcessOrphanedTimeEntries(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Find orphaned entries whose tasks now exist
	query := `
		SELECT ote.id, ote.task_id, ote.user_id, ote.date, ote.start_time, ote.end_time, 
		       ote.duration, ote.description, ote.billable, ote.locked, ote.modify_time
		FROM orphaned_time_entries ote
		INNER JOIN tasks t ON ote.task_id = t.task_id
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query processable orphaned entries: %w", err)
	}
	defer rows.Close()

	var processableEntries []ProcessedTimeEntry
	var processableIDs []int

	for rows.Next() {
		var entry ProcessedTimeEntry
		err := rows.Scan(
			&entry.ID, &entry.TaskID, &entry.UserID, &entry.Date,
			&entry.Start, &entry.End, &entry.Duration, &entry.Description,
			&entry.Billable, &entry.Locked, &entry.ModifyTime,
		)
		if err != nil {
			logger.Errorf("Failed to scan orphaned entry: %v", err)
			continue
		}
		processableEntries = append(processableEntries, entry)
		processableIDs = append(processableIDs, entry.ID)
	}

	if len(processableEntries) == 0 {
		logger.Debug("No orphaned time entries to process")
		return nil
	}

	logger.Infof("Found %d orphaned time entries that can now be processed", len(processableEntries))

	// Start transaction for atomic operation
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into main time_entries table
	insertStmt, err := tx.Prepare(`INSERT INTO time_entries 
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
	defer insertStmt.Close()

	successCount := 0
	for _, entry := range processableEntries {
		_, err := insertStmt.Exec(
			entry.ID, entry.TaskID, entry.UserID, entry.Date,
			entry.Start, entry.End, entry.Duration, entry.Description,
			entry.Billable, entry.Locked, entry.ModifyTime,
		)
		if err != nil {
			logger.Errorf("Failed to insert orphaned entry %d into main table: %v", entry.ID, err)
			continue
		}
		successCount++
	}

	// Remove processed entries from orphaned table
	if successCount > 0 {
		deleteStmt, err := tx.Prepare(`DELETE FROM orphaned_time_entries WHERE id = ANY($1)`)
		if err != nil {
			return fmt.Errorf("failed to prepare delete statement: %w", err)
		}
		defer deleteStmt.Close()

		// Only delete IDs that were successfully processed
		var successfulIDs []int
		for i, entry := range processableEntries {
			if i < successCount { // Assumes processing was sequential
				successfulIDs = append(successfulIDs, entry.ID)
			}
		}

		_, err = deleteStmt.Exec(pq.Array(successfulIDs))
		if err != nil {
			logger.Errorf("Failed to delete processed orphaned entries: %v", err)
			// Don't fail the transaction, just log the error
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Infof("Successfully processed %d orphaned time entries", successCount)
	return nil
}

// GetOrphanedTimeEntriesCount returns the number of orphaned time entries in the database
func GetOrphanedTimeEntriesCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM orphaned_time_entries").Scan(&count)
	if err != nil {
		// If table doesn't exist, return 0
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "does not exist") {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to count orphaned time entries: %w", err)
	}
	return count, nil
}

// CleanupOldOrphanedEntries removes orphaned time entries older than the specified days
func CleanupOldOrphanedEntries(db *sql.DB, olderThanDays int) error {
	logger := GetGlobalLogger()

	query := `DELETE FROM orphaned_time_entries WHERE sync_date < $1`
	cutoffDate := time.Now().AddDate(0, 0, -olderThanDays).Format("2006-01-02")

	result, err := db.Exec(query, cutoffDate)
	if err != nil {
		// If table doesn't exist, that's fine
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "does not exist") {
			return nil
		}
		return fmt.Errorf("failed to cleanup old orphaned entries: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		logger.Infof("Cleaned up %d orphaned time entries older than %d days", rowsAffected, olderThanDays)
	}

	return nil
}
