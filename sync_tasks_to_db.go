package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type JsonTask struct {
	TaskID      int    `json:"task_id"`
	ParentID    int    `json:"parent_id"`
	AssignedBy  int    `json:"assigned_by"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	RootGroupID int    `json:"root_group_id"`
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
		logger.Debug("Starting FULL task synchronization with TimeCamp")
	} else {
		logger.Debug("Starting INCREMENTAL task synchronization with TimeCamp (cron mode)")
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

	// Prepare statements based on sync type
	var insertStatement *sql.Stmt
	if fullSync {
		// For full sync, use INSERT ... ON CONFLICT DO UPDATE to update existing tasks
		insertStatement, err = db.Prepare(`INSERT INTO tasks (task_id, parent_id, assigned_by, name, level, root_group_id) 
			VALUES ($1, $2, $3, $4, $5, $6) 
			ON CONFLICT (task_id) DO UPDATE SET 
			parent_id = EXCLUDED.parent_id,
			assigned_by = EXCLUDED.assigned_by,
			name = EXCLUDED.name,
			level = EXCLUDED.level,
			root_group_id = EXCLUDED.root_group_id`)
	} else {
		// For incremental sync, use INSERT ... ON CONFLICT DO NOTHING to only add new tasks
		insertStatement, err = db.Prepare(`INSERT INTO tasks (task_id, parent_id, assigned_by, name, level, root_group_id) 
			VALUES ($1, $2, $3, $4, $5, $6) 
			ON CONFLICT (task_id) DO NOTHING`)
	}
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer CloseWithErrorLog(insertStatement, "prepared statement")

	// For incremental sync, get existing tasks to compare changes
	var existingTasks map[int]JsonTask
	if !fullSync {
		existingTasks, err = getExistingTasks(db)
		if err != nil {
			logger.Warnf("Failed to fetch existing tasks for comparison: %v", err)
			// Continue with full processing if we can't get existing tasks
			existingTasks = make(map[int]JsonTask)
		}
	}

	index := 0
	errorCount := 0
	newTaskCount := 0
	updatedTaskCount := 0
	skippedTaskCount := 0

	for _, task := range timecampTasks {
		if !fullSync {
			// For incremental sync, check if task needs processing
			if existingTask, exists := existingTasks[task.TaskID]; exists {
				// Task exists, check if it needs updating
				if !taskNeedsUpdate(existingTask, task) {
					skippedTaskCount++
					continue
				}
				// Task needs update, process it
				updatedTaskCount++
			} else {
				// New task
				newTaskCount++
			}
		}

		// Check if task already exists to track changes
		var existingName string
		checkQuery := "SELECT name FROM tasks WHERE task_id = $1"
		err := db.QueryRow(checkQuery, task.TaskID).Scan(&existingName)

		if err == sql.ErrNoRows {
			// New task
			_, err := insertStatement.Exec(task.TaskID, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID)
			if err != nil {
				logger.Errorf("Failed to insert task %d (%s): %v", task.TaskID, task.Name, err)
				errorCount++
				continue
			}
			// Track new task creation
			if trackErr := TrackTaskChange(db, task.TaskID, task.Name, "created", "", task.Name); trackErr != nil {
				logger.Errorf("Failed to track task creation for task %d: %v", task.TaskID, trackErr)
			}
		} else if err != nil {
			logger.Errorf("Failed to check existing task %d: %v", task.TaskID, err)
			errorCount++
			continue
		} else if existingName != task.Name {
			// Task name changed, update it (only for full sync or when name actually changed)
			if fullSync {
				updateQuery := "UPDATE tasks SET parent_id = $1, assigned_by = $2, name = $3, level = $4, root_group_id = $5 WHERE task_id = $6"
				_, err := db.Exec(updateQuery, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID, task.TaskID)
				if err != nil {
					logger.Errorf("Failed to update task %d: %v", task.TaskID, err)
					errorCount++
					continue
				}
			} else {
				updateQuery := "UPDATE tasks SET name = $1 WHERE task_id = $2"
				_, err := db.Exec(updateQuery, task.Name, task.TaskID)
				if err != nil {
					logger.Errorf("Failed to update task %d name: %v", task.TaskID, err)
					errorCount++
					continue
				}
			}
			// Track name change
			if trackErr := TrackTaskChange(db, task.TaskID, task.Name, "name_changed", existingName, task.Name); trackErr != nil {
				logger.Errorf("Failed to track task name change for task %d: %v", task.TaskID, trackErr)
			}
		}
		index++
	}

	if fullSync {
		logger.Infof("Full task sync completed: %d tasks processed, %d errors encountered", index, errorCount)
	} else {
		logger.Infof("Incremental task sync completed: %d new tasks, %d updated tasks, %d skipped (unchanged), %d errors",
			newTaskCount, updatedTaskCount, skippedTaskCount, errorCount)
	}

	if errorCount > 0 && errorCount == len(timecampTasks) {
		return fmt.Errorf("all task operations failed during sync")
	}

	return nil
}

// getExistingTasks fetches all existing tasks from database for comparison
func getExistingTasks(db *sql.DB) (map[int]JsonTask, error) {
	query := "SELECT task_id, parent_id, assigned_by, name, level, root_group_id FROM tasks"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing tasks: %w", err)
	}
	defer rows.Close()

	existingTasks := make(map[int]JsonTask)
	for rows.Next() {
		var task JsonTask
		err := rows.Scan(&task.TaskID, &task.ParentID, &task.AssignedBy, &task.Name, &task.Level, &task.RootGroupID)
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
		existing.RootGroupID != fetched.RootGroupID
}

// TrackTaskChange records a task change in the history table
func TrackTaskChange(db *sql.DB, taskID int, taskName, changeType, previousValue, currentValue string) error {
	query := `INSERT INTO task_history (task_id, name, change_type, previous_value, current_value) 
			  VALUES ($1, $2, $3, $4, $5)`

	_, err := db.Exec(query, taskID, taskName, changeType, previousValue, currentValue)
	if err != nil {
		return fmt.Errorf("failed to track task change: %w", err)
	}

	return nil
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

	request.Header.Add("Authorization", authBearer)
	request.Header.Add("Accept", "application/json")

	logger.Debugf("Fetching tasks from TimeCamp API: %s", getAllTasksURL)

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

	logger.Debugf("Successfully fetched %d tasks from TimeCamp", len(tasks))

	return tasks, nil
}

// SyncTasksToDatabaseIncremental is a wrapper that performs incremental sync (for cron jobs)
// This only processes tasks that have changed, making it efficient for regular updates
func SyncTasksToDatabaseIncremental() error {
	return SyncTasksToDatabase(false)
}

// SyncTasksToDatabaseFull is a wrapper that performs full sync
// This processes all tasks and is used for manual syncs and full-sync operations
func SyncTasksToDatabaseFull() error {
	return SyncTasksToDatabase(true)
}
