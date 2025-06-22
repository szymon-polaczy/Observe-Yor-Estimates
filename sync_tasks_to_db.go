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

// SyncTasksToDatabase fetches all tasks from TimeCamp and stores them in the database
// Note: Tasks are synced completely each time since the TimeCamp API doesn't support
// filtering by modification date and the task list is relatively small
func SyncTasksToDatabase() error {
	logger := NewLogger()

	// Load environment variables - but don't panic here since main already validated them
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	logger.Debug("Starting task synchronization with TimeCamp")

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

	// Use INSERT OR IGNORE to handle existing tasks
	insertStatement, err := db.Prepare("INSERT OR IGNORE INTO tasks values(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer CloseWithErrorLog(insertStatement, "prepared statement")

	index := 0
	errorCount := 0
	for _, task := range timecampTasks {
		// Check if task already exists to track changes
		var existingName string
		checkQuery := "SELECT name FROM tasks WHERE task_id = ?"
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
			// Task name changed, update it
			updateQuery := "UPDATE tasks SET name = ? WHERE task_id = ?"
			_, err := db.Exec(updateQuery, task.Name, task.TaskID)
			if err != nil {
				logger.Errorf("Failed to update task %d name: %v", task.TaskID, err)
				errorCount++
				continue
			}
			// Track name change
			if trackErr := TrackTaskChange(db, task.TaskID, task.Name, "name_changed", existingName, task.Name); trackErr != nil {
				logger.Errorf("Failed to track task name change for task %d: %v", task.TaskID, trackErr)
			}
		}
		index++
	}

	logger.Infof("Task sync completed: %d tasks processed, %d errors encountered", index, errorCount)

	if errorCount > 0 && errorCount == len(timecampTasks) {
		return fmt.Errorf("all task operations failed during sync")
	}

	return nil
}

// Example of critical close error handling (for reference - not changing existing code)
// func criticalDatabaseOperation() error {
//     db, err := GetDB()
//     if err != nil {
//         return err
//     }
//
//     // For critical operations where close errors matter
//     defer func() {
//         if closeErr := db.Close(); closeErr != nil {
//             // In critical operations, you might want to return this error
//             logger.Errorf("Critical: Failed to close database: %v", closeErr)
//         }
//     }()
//
//     // ... critical operations
//     return nil
// }

// TrackTaskChange records a task change in the history table
func TrackTaskChange(db *sql.DB, taskID int, taskName, changeType, previousValue, currentValue string) error {
	query := `INSERT INTO task_history (task_id, name, change_type, previous_value, current_value) 
			  VALUES (?, ?, ?, ?, ?)`

	_, err := db.Exec(query, taskID, taskName, changeType, previousValue, currentValue)
	if err != nil {
		return fmt.Errorf("failed to track task change: %w", err)
	}

	return nil
}

func getTimecampTasks() ([]JsonTask, error) {
	logger := NewLogger()

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
