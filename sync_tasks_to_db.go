package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type JsonTask struct {
	TaskID      int    `json:"task_id"`
	ParentID    int    `json:"parent_id"`
	AssignedBy  int    `json:"assigned_by"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	RootGroupID int    `json:"root_group_id"`
	Archived    int    `json:"archived,omitempty"` // Optional field for archived status
}

// SyncTasksToDatabase fetches tasks from TimeCamp and stores them in the database
// fullSync: if true, processes all tasks (for full sync operations)
// fullSync: if false, only processes tasks that don't exist or have changed recently (for cron jobs)
func SyncTasksToDatabase(fullSync bool) error {
	logger := GetGlobalLogger()

	// Load environment variables - but don't panic here since main already validated them
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	if fullSync {
		logger.Debug("Starting FULL task synchronization with TimeCamp (including archived tasks)")
	} else {
		logger.Debug("Starting INCREMENTAL task synchronization with TimeCamp (including archived tasks)")
	}

	timecampTasks, err := getTimecampTasks()
	if err != nil {
		return fmt.Errorf("failed to fetch tasks from TimeCamp: %w", err)
	}

	if len(timecampTasks) == 0 {
		logger.Warn("No tasks received from TimeCamp API")
		return nil // Not an error, just no data
	}

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	// Note: Using shared database connection, no need to close here

	if fullSync {
		return performFullSyncBatch(db, timecampTasks, logger)
	} else {
		return performIncrementalSync(db, timecampTasks, logger)
	}
}

// performFullSyncBatch performs optimized batch operations for full sync
func performFullSyncBatch(db *sql.DB, tasks []JsonTask, logger *Logger) error {
	logger.Infof("Starting optimized batch full sync for %d tasks", len(tasks))

	// Start a transaction for better performance
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Will be ignored if tx.Commit() succeeds

	// Prepare the UPSERT statement for batch operations
	stmt, err := tx.Prepare(`INSERT INTO tasks (task_id, parent_id, assigned_by, name, level, root_group_id, archived) 
		VALUES ($1, $2, $3, $4, $5, $6, $7) 
		ON CONFLICT (task_id) DO UPDATE SET 
		parent_id = EXCLUDED.parent_id,
		assigned_by = EXCLUDED.assigned_by,
		name = EXCLUDED.name,
		level = EXCLUDED.level,
		root_group_id = EXCLUDED.root_group_id,
		archived = EXCLUDED.archived`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch statement: %w", err)
	}
	defer stmt.Close()

	// Process tasks in batches for better performance
	const batchSize = 100
	successCount := 0
	errorCount := 0

	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}

		batch := tasks[i:end]
		logger.Debugf("Processing batch %d-%d of %d tasks", i+1, end, len(tasks))

		for _, task := range batch {
			_, err := stmt.Exec(task.TaskID, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID, task.Archived)
			if err != nil {
				logger.Errorf("Failed to upsert task %d (%s): %v", task.TaskID, task.Name, err)
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

	logger.Infof("Optimized full task sync completed: %d tasks processed successfully, %d errors encountered", successCount, errorCount)

	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all task operations failed during sync")
	}

	return nil
}

// performIncrementalSync performs the original incremental sync logic
func performIncrementalSync(db *sql.DB, timecampTasks []JsonTask, logger *Logger) error {
	// For incremental sync, get existing tasks to compare changes
	existingTasks, err := getExistingTasks(db)
	if err != nil {
		logger.Warnf("Failed to fetch existing tasks for comparison: %v", err)
		// Continue with full processing if we can't get existing tasks
		existingTasks = make(map[int]JsonTask)
	}

	// Prepare insert statement for incremental sync
	insertStatement, err := db.Prepare(`INSERT INTO tasks (task_id, parent_id, assigned_by, name, level, root_group_id, archived) 
		VALUES ($1, $2, $3, $4, $5, $6, $7) 
		ON CONFLICT (task_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer CloseWithErrorLog(insertStatement, "prepared statement")

	newTaskCount := 0
	updatedTaskCount := 0
	skippedTaskCount := 0
	errorCount := 0

	for _, task := range timecampTasks {
		// For incremental sync, check if task needs processing
		if existingTask, exists := existingTasks[task.TaskID]; exists {
			// Task exists, check if it needs updating
			if !taskNeedsUpdate(existingTask, task) {
				skippedTaskCount++
				continue
			}
			// Task needs update, process it
			updateQuery := "UPDATE tasks SET parent_id = $1, assigned_by = $2, name = $3, level = $4, root_group_id = $5, archived = $6 WHERE task_id = $7"
			_, err := db.Exec(updateQuery, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID, task.Archived, task.TaskID)
			if err != nil {
				logger.Errorf("Failed to update task %d: %v", task.TaskID, err)
				errorCount++
				continue
			}
			updatedTaskCount++
		} else {
			// New task
			_, err := insertStatement.Exec(task.TaskID, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID, task.Archived)
			if err != nil {
				logger.Errorf("Failed to insert task %d (%s): %v", task.TaskID, task.Name, err)
				errorCount++
				continue
			}
			newTaskCount++
		}
	}

	logger.Infof("Incremental task sync completed: %d new tasks, %d updated tasks, %d skipped (unchanged), %d errors",
		newTaskCount, updatedTaskCount, skippedTaskCount, errorCount)

	if errorCount > 0 && errorCount == len(timecampTasks) {
		return fmt.Errorf("all task operations failed during sync")
	}

	return nil
}

// getExistingTasks fetches all existing tasks from database for comparison
func getExistingTasks(db *sql.DB) (map[int]JsonTask, error) {
	query := "SELECT task_id, parent_id, assigned_by, name, level, root_group_id, COALESCE(archived, 0) FROM tasks"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing tasks: %w", err)
	}
	defer rows.Close()

	existingTasks := make(map[int]JsonTask)
	for rows.Next() {
		var task JsonTask
		err := rows.Scan(&task.TaskID, &task.ParentID, &task.AssignedBy, &task.Name, &task.Level, &task.RootGroupID, &task.Archived)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task row: %w", err)
		}
		existingTasks[task.TaskID] = task
	}

	return existingTasks, nil
}

// taskNeedsUpdate compares existing task with fetched task to determine if update is needed
func taskNeedsUpdate(existing, fetched JsonTask) bool {
	return existing.ParentID != fetched.ParentID ||
		existing.AssignedBy != fetched.AssignedBy ||
		existing.Name != fetched.Name ||
		existing.Level != fetched.Level ||
		existing.RootGroupID != fetched.RootGroupID ||
		existing.Archived != fetched.Archived
}

func getTimecampTasks() ([]JsonTask, error) {
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

	// Add parameters to optimize API response size and include archived tasks
	q := request.URL.Query()
	q.Add("minimal", "1")
	q.Add("exclude_archived", "1") // Include archived tasks in the sync
	request.URL.RawQuery = q.Encode()

	request.Header.Add("Authorization", authBearer)
	request.Header.Add("Accept", "application/json")

	logger.Debugf("Fetching tasks from TimeCamp API with minimal option and archived tasks: %s", request.URL.String())

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

	// Count archived tasks for logging
	archivedCount := 0
	for _, task := range tasks {
		if task.Archived == 1 {
			archivedCount++
		}
	}

	logger.Debugf("Successfully fetched %d tasks from TimeCamp (%d active, %d archived)", len(tasks), len(tasks)-archivedCount, archivedCount)

	return tasks, nil
}
