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
		logger.Warnf("REMEDIATION SUGGESTION: No tasks found in database. Run a full tasks sync first: `./observe-yor-estimates sync-tasks` or `./observe-yor-estimates full-sync`")
	} else if missingTaskCount > 10 {
		logger.Warnf("REMEDIATION SUGGESTION: High number of missing task references (%d). Consider running a full tasks sync to ensure all tasks are up to date: `./observe-yor-estimates sync-tasks`", missingTaskCount)
	} else {
		logger.Warnf("REMEDIATION SUGGESTION: Some time entries reference missing tasks (%d). This may be normal if tasks were deleted/archived in TimeCamp but still have historical time entries.", missingTaskCount)
	}
}

// calculateDateRanges calculates date ranges for different period types
func calculateDateRanges(periodType string, days int) PeriodDateRanges {
	now := time.Now()

	switch periodType {
	case "today":
		today := now.Format("2006-01-02")
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		return PeriodDateRanges{
			Current:  DateRange{Start: today, End: today, Label: "Today"},
			Previous: DateRange{Start: yesterday, End: yesterday, Label: "Yesterday"},
		}

	case "yesterday", "daily":
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		dayBefore := now.AddDate(0, 0, -2).Format("2006-01-02")
		return PeriodDateRanges{
			Current:  DateRange{Start: yesterday, End: yesterday, Label: "Yesterday"},
			Previous: DateRange{Start: dayBefore, End: dayBefore, Label: "Day Before"},
		}

	case "this_week":
		// Current week: Monday to today
		weekStart := now.AddDate(0, 0, -int(now.Weekday()-time.Monday))
		if now.Weekday() == time.Sunday {
			weekStart = weekStart.AddDate(0, 0, -6) // Go back to Monday
		}
		currentWeekEnd := now.Format("2006-01-02")

		// Last week: Monday to Sunday of previous week
		lastWeekStart := weekStart.AddDate(0, 0, -7).Format("2006-01-02")
		lastWeekEnd := weekStart.AddDate(0, 0, -1).Format("2006-01-02")

		return PeriodDateRanges{
			Current:  DateRange{Start: weekStart.Format("2006-01-02"), End: currentWeekEnd, Label: "This Week"},
			Previous: DateRange{Start: lastWeekStart, End: lastWeekEnd, Label: "Last Week"},
		}

	case "last_week", "weekly":
		// Last week: Monday to Sunday of previous week
		weekStart := now.AddDate(0, 0, -int(now.Weekday()-time.Monday))
		if now.Weekday() == time.Sunday {
			weekStart = weekStart.AddDate(0, 0, -6)
		}
		lastWeekStart := weekStart.AddDate(0, 0, -7).Format("2006-01-02")
		lastWeekEnd := weekStart.AddDate(0, 0, -1).Format("2006-01-02")

		// Previous week: Monday to Sunday before last week
		prevWeekStart := weekStart.AddDate(0, 0, -14).Format("2006-01-02")
		prevWeekEnd := weekStart.AddDate(0, 0, -8).Format("2006-01-02")

		return PeriodDateRanges{
			Current:  DateRange{Start: lastWeekStart, End: lastWeekEnd, Label: "Last Week"},
			Previous: DateRange{Start: prevWeekStart, End: prevWeekEnd, Label: "Previous Week"},
		}

	case "this_month":
		// Current month: 1st to today
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		currentMonthEnd := now.Format("2006-01-02")

		// Last month: 1st to last day of previous month
		lastMonthStart := monthStart.AddDate(0, -1, 0).Format("2006-01-02")
		lastMonthEnd := monthStart.AddDate(0, 0, -1).Format("2006-01-02")

		return PeriodDateRanges{
			Current:  DateRange{Start: monthStart.Format("2006-01-02"), End: currentMonthEnd, Label: "This Month"},
			Previous: DateRange{Start: lastMonthStart, End: lastMonthEnd, Label: "Last Month"},
		}

	case "last_month":
		// Last month: 1st to last day of previous month
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		lastMonthStart := monthStart.AddDate(0, -1, 0).Format("2006-01-02")
		lastMonthEnd := monthStart.AddDate(0, 0, -1).Format("2006-01-02")

		// Previous month: 1st to last day before last month
		prevMonthStart := monthStart.AddDate(0, -2, 0).Format("2006-01-02")
		prevMonthEnd := monthStart.AddDate(-1, 0, 0).Format("2006-01-02")

		return PeriodDateRanges{
			Current:  DateRange{Start: lastMonthStart, End: lastMonthEnd, Label: "Last Month"},
			Previous: DateRange{Start: prevMonthStart, End: prevMonthEnd, Label: "Previous Month"},
		}

	case "last_x_days":
		// Last X days: X days ago to today
		currentStart := now.AddDate(0, 0, -days).Format("2006-01-02")
		currentEnd := now.Format("2006-01-02")

		// Previous X days: 2X days ago to X days ago
		previousStart := now.AddDate(0, 0, -days*2).Format("2006-01-02")
		previousEnd := now.AddDate(0, 0, -days).Format("2006-01-02")

		currentLabel := fmt.Sprintf("Last %d Days", days)
		previousLabel := fmt.Sprintf("Previous %d Days", days)

		return PeriodDateRanges{
			Current:  DateRange{Start: currentStart, End: currentEnd, Label: currentLabel},
			Previous: DateRange{Start: previousStart, End: previousEnd, Label: previousLabel},
		}

	default:
		// Fallback to yesterday
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		dayBefore := now.AddDate(0, 0, -2).Format("2006-01-02")
		return PeriodDateRanges{
			Current:  DateRange{Start: yesterday, End: yesterday, Label: "Yesterday"},
			Previous: DateRange{Start: dayBefore, End: dayBefore, Label: "Day Before"},
		}
	}
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

// GetDynamicTaskTimeEntriesWithProject fetches task time entries for any dynamic time period
func GetDynamicTaskTimeEntriesWithProject(db *sql.DB, periodType string, days int, projectTaskID *int) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()
	logger.Debugf("Querying database for dynamic task time entries with period: %s, days: %d, project filtering", periodType, days)

	// Calculate date ranges for the period
	dateRanges := calculateDateRanges(periodType, days)

	var projectTaskIDs []int
	if projectTaskID != nil {
		var err error
		projectTaskIDs, err = GetProjectTaskIDs(db, *projectTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project task IDs: %w", err)
		}
		if len(projectTaskIDs) == 0 {
			return []TaskUpdateInfo{}, nil
		}
	}

	// Build project filtering clause for time_entries using project_id
	var projectCTE string
	var timeEntriesFilter string
	var args []interface{}

	if projectTaskID != nil {
		// Create a CTE that gets all task IDs for the project once
		projectCTE = `
project_tasks AS (
    SELECT task_id FROM tasks WHERE project_id = (
        SELECT id FROM projects WHERE timecamp_task_id = $1
    )
),`
		timeEntriesFilter = `AND task_id IN (SELECT task_id FROM project_tasks)`
		args = append(args, *projectTaskID)
	}

	// User breakdown query
	userBreakdownQuery := fmt.Sprintf(`
WITH %scurrent_period AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= '%s'::date AND date::date <= '%s'::date %s
    GROUP BY task_id, user_id
),
previous_period AS (
    SELECT task_id, user_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= '%s'::date AND date::date <= '%s'::date %s
    GROUP BY task_id, user_id
)
SELECT 
    COALESCE(tc.task_id, tp.task_id) AS task_id,
    COALESCE(tc.user_id, tp.user_id) AS user_id,
    COALESCE(tc.total_duration, 0) AS current_duration, 
    COALESCE(tp.total_duration, 0) AS previous_duration
FROM current_period tc
FULL OUTER JOIN previous_period tp ON tc.task_id = tp.task_id AND tc.user_id = tp.user_id
WHERE COALESCE(tc.total_duration, 0) > 0;`,
		projectCTE,
		dateRanges.Current.Start, dateRanges.Current.End, timeEntriesFilter,
		dateRanges.Previous.Start, dateRanges.Previous.End, timeEntriesFilter)

	userRows, err := db.Query(userBreakdownQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query user breakdown: %w", err)
	}
	defer userRows.Close()

	userBreakdowns := make(map[int]map[int]UserTimeContribution)
	for userRows.Next() {
		var taskID, userID, currentDuration, previousDuration int
		err := userRows.Scan(&taskID, &userID, &currentDuration, &previousDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user breakdown row: %w", err)
		}

		if _, exists := userBreakdowns[taskID]; !exists {
			userBreakdowns[taskID] = make(map[int]UserTimeContribution)
		}

		userBreakdowns[taskID][userID] = UserTimeContribution{
			UserID:       userID,
			CurrentTime:  formatDuration(currentDuration),
			PreviousTime: formatDuration(previousDuration),
		}
	}

	// Main query
	mainQuery := fmt.Sprintf(`
WITH %scurrent_period AS (
    SELECT task_id, SUM(duration) AS total_duration, 
           string_agg(DISTINCT NULLIF(description, ''), '; ' ORDER BY NULLIF(description, '')) AS descriptions
    FROM time_entries
    WHERE date::date >= '%s'::date AND date::date <= '%s'::date %s
    GROUP BY task_id
),
previous_period AS (
    SELECT task_id, SUM(duration) AS total_duration
    FROM time_entries
    WHERE date::date >= '%s'::date AND date::date <= '%s'::date %s
    GROUP BY task_id
),
days_worked AS (
    SELECT task_id, COUNT(DISTINCT date::date) AS days_worked
    FROM time_entries
    WHERE date::date >= '%s'::date AND date::date <= '%s'::date %s
    GROUP BY task_id
)
SELECT 
    COALESCE(tc.task_id, tp.task_id) AS task_id,
    t.parent_id,
    t.name,
    COALESCE(tc.total_duration, 0) AS current_duration, 
    COALESCE(tp.total_duration, 0) AS previous_duration,
    COALESCE(dw.days_worked, 0) AS days_worked,
    COALESCE(tc.descriptions, '') AS descriptions
FROM current_period tc
FULL OUTER JOIN previous_period tp ON tc.task_id = tp.task_id
LEFT JOIN days_worked dw ON COALESCE(tc.task_id, tp.task_id) = dw.task_id
LEFT JOIN tasks t ON COALESCE(tc.task_id, tp.task_id) = t.task_id
WHERE COALESCE(tc.total_duration, 0) > 0
  AND t.task_id IS NOT NULL
ORDER BY COALESCE(tc.total_duration, 0) DESC;`,
		projectCTE,
		dateRanges.Current.Start, dateRanges.Current.End, timeEntriesFilter,
		dateRanges.Previous.Start, dateRanges.Previous.End, timeEntriesFilter,
		dateRanges.Current.Start, dateRanges.Current.End, timeEntriesFilter)

	rows, err := db.Query(mainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query dynamic task time entries: %w", err)
	}
	defer rows.Close()

	var taskInfos []TaskUpdateInfo
	for rows.Next() {
		var taskID, parentID, currentDuration, previousDuration, daysWorked int
		var name, descriptions string
		err := rows.Scan(&taskID, &parentID, &name, &currentDuration, &previousDuration, &daysWorked, &descriptions)
		if err != nil {
			return nil, fmt.Errorf("failed to scan dynamic task time entry row: %w", err)
		}

		comments := []string{}
		if descriptions != "" {
			comments = strings.Split(descriptions, "; ")
		}

		userBreakdown := userBreakdowns[taskID]
		if userBreakdown == nil {
			userBreakdown = make(map[int]UserTimeContribution)
		}

		taskInfo := TaskUpdateInfo{
			TaskID:         taskID,
			ParentID:       parentID,
			Name:           name,
			CurrentPeriod:  dateRanges.Current.Label,
			CurrentTime:    formatDuration(currentDuration),
			PreviousPeriod: dateRanges.Previous.Label,
			PreviousTime:   formatDuration(previousDuration),
			DaysWorked:     daysWorked,
			Comments:       comments,
			UserBreakdown:  userBreakdown,
		}

		taskInfos = append(taskInfos, taskInfo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	logger.Debugf("Found %d dynamic task updates with project filtering for period: %s", len(taskInfos), dateRanges.Current.Label)
	return taskInfos, nil
}
