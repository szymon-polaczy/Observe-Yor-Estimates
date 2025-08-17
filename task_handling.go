package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	pq "github.com/lib/pq"
)

// groupTasksByProject groups tasks by project name
func groupTasksByProject(tasks []TaskInfo) map[string][]TaskInfo {
	db, err := GetDB()
	if err != nil {
		return map[string][]TaskInfo{"Other": tasks}
	}

	allTasks, err := getAllTasks(db)
	if err != nil {
		return map[string][]TaskInfo{"Other": tasks}
	}

	groups := make(map[string][]TaskInfo)
	for _, task := range tasks {
		project := getProjectNameForTask(task.TaskID, allTasks)
		if project == "" {
			project = "Other"
		}
		groups[project] = append(groups[project], task)
	}

	return groups
}

// getProjectNameForTask finds project name for a task
func getProjectNameForTask(taskID int, allTasks map[int]Task) string {
	currentID := taskID
	var previousName string

	for i := 0; i < 10; i++ { // max depth
		task, ok := allTasks[currentID]
		if !ok {
			return previousName
		}

		if task.ParentID == 0 {
			return previousName
		}

		previousName = task.Name
		currentID = task.ParentID
	}

	return previousName
}

// getAllTasks retrieves all tasks from database
func getAllTasks(db *sql.DB) (map[int]Task, error) {
	rows, err := db.Query("SELECT task_id, parent_id, name FROM tasks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make(map[int]Task)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ParentID, &task.Name); err == nil {
			tasks[task.ID] = task
		}
	}

	return tasks, nil
}

// getFilteredTasksWithTimeout gets tasks with time entries for a period, optionally filtered by projects
// If projectNames is empty, returns all tasks with time entries in the period
func getFilteredTasksWithTimeout(startTime time.Time, endTime time.Time, projectNames []string, percentage string) []TaskInfo {
	logger := GetGlobalLogger()
	logger.Infof("getFilteredTasksWithTimeout called with: startTime=%s, endTime=%s, projectNames='%s', percentage='%s'",
		startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"), projectNames, percentage)

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection: %v", err)
		return []TaskInfo{}
	}

	// Create context with 15 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Convert time range to date strings for SQL query (time_entries uses TEXT date field)
	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")
	logger.Infof("Searching for tasks between dates: %s and %s", startDateStr, endDateStr)

	// Simplified query that should work with the actual database structure
	var query string
	var args []interface{}

	// Filter out empty project names and check if we have valid project names
	validProjectNames := make([]string, 0)
	for _, projectName := range projectNames {
		projectName = strings.TrimSpace(projectName)
		if projectName != "" {
			validProjectNames = append(validProjectNames, strings.ToLower(projectName))
		}
	}

	if len(validProjectNames) > 0 {

		// Build dynamic placeholders for IN clause
		placeholders := make([]string, len(validProjectNames))
		for i := range validProjectNames {
			placeholders[i] = fmt.Sprintf("$%d", i+3) // Start from $3 since $1,$2 are dates
		}
		inClause := strings.Join(placeholders, ",")

		// When filtering by project, join with projects table
		query = fmt.Sprintf(`
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE 
				WHEN te.date >= $1 AND te.date <= $2
				THEN te.duration 
				ELSE 0 
			END), 0) as current_period_duration,
			COALESCE(SUM(te.duration), 0) as total_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		LEFT JOIN projects p ON t.project_id = p.id
		WHERE LOWER(p.name) IN (%s)
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(CASE 
			WHEN te.date >= $%d AND te.date <= $%d
			THEN te.duration 
			ELSE 0 
		END), 0) > 0
		ORDER BY t.name;`, inClause, len(validProjectNames)+3, len(validProjectNames)+4)

		// Build args array with individual project names
		args = []interface{}{startDateStr, endDateStr}
		for _, projectName := range validProjectNames {
			args = append(args, projectName)
		}
		args = append(args, startDateStr, endDateStr)
	} else {
		// When not filtering by project, get all tasks with time entries in the period
		query = `
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE 
				WHEN te.date >= $1 AND te.date <= $2
				THEN te.duration 
				ELSE 0 
			END), 0) as current_period_duration,
			COALESCE(SUM(te.duration), 0) as total_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(CASE 
			WHEN te.date >= $3 AND te.date <= $4
			THEN te.duration 
			ELSE 0 
		END), 0) > 0
		ORDER BY t.name;`
		args = []interface{}{startDateStr, endDateStr, startDateStr, endDateStr}
	}

	logger.Infof("Query: %s", query)
	logger.Infof("Args: %v", args)

	logger.Infof("Executing query with timeout context and args: %v", args)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Errorf("Database query timed out after 15 seconds")
		} else {
			logger.Errorf("Database query failed: %v", err)
		}
		logger.Errorf("Query was: %s", query)
		return []TaskInfo{}
	}
	defer rows.Close()

	var allTasks []TaskInfo
	var percentageThreshold float64
	taskCount := 0

	// Parse percentage threshold if filtering by percentage
	if percentage != "" {
		if percentageFloat, err := strconv.ParseFloat(strings.TrimSuffix(percentage, "%"), 64); err == nil {
			percentageThreshold = percentageFloat
			logger.Infof("Filtering by percentage threshold: %.1f%%", percentageThreshold)
		} else {
			logger.Errorf("Invalid percentage format: %s", percentage)
			return []TaskInfo{} // Invalid percentage format
		}
	}

	for rows.Next() {
		taskCount++
		var task TaskInfo
		var currentDuration, totalDuration int

		err := rows.Scan(
			&task.TaskID,
			&task.ParentID,
			&task.Name,
			&currentDuration,
			&totalDuration,
		)
		if err != nil {
			logger.Errorf("Failed to scan task row %d: %v", taskCount, err)
			continue
		}

		logger.Infof("Found task %d: ID=%d, Name='%s', CurrentDuration=%d seconds", taskCount, task.TaskID, task.Name, currentDuration)

		// Format durations using existing formatDuration function (takes seconds)
		task.CurrentTime = formatDuration(currentDuration)
		task.TotalDuration = formatDuration(totalDuration)

		// Parse estimation from task name and calculate usage based on total time spent
		estimationInfo := ParseTaskEstimationWithUsage(task.Name, task.TotalDuration, "0h 0m")
		task.EstimationInfo = estimationInfo

		// If filtering by percentage, apply the logic
		if percentage != "" {
			// Skip tasks without valid estimations
			if estimationInfo.ErrorMessage != "" {
				logger.Infof("Skipping task %d (no valid estimation): %s", task.TaskID, estimationInfo.ErrorMessage)
				continue
			}

			// Skip tasks below threshold
			if estimationInfo.Percentage < percentageThreshold {
				logger.Infof("Skipping task %d (below threshold): %.1f%% < %.1f%%", task.TaskID, estimationInfo.Percentage, percentageThreshold)
				continue
			}
		}

		allTasks = append(allTasks, task)
	}

	logger.Infof("Query returned %d total tasks, filtered to %d tasks", taskCount, len(allTasks))
	return allTasks
}

// addCommentsToTasksCtx is the single implementation that enriches tasks with comments using the provided context
func addCommentsToTasksCtx(ctx context.Context, tasks []TaskInfo, startTime time.Time, endTime time.Time) []TaskInfo {
	logger := GetGlobalLogger()
	if len(tasks) == 0 {
		logger.Info("No tasks to add comments to")
		return tasks
	}

	logger.Infof("Adding comments to %d tasks", len(tasks))

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection for comments: %v", err)
		return tasks
	}

	// Collect all task IDs
	taskIDs := make([]string, 0, len(tasks))
	taskMap := make(map[int]*TaskInfo)

	for i := range tasks {
		taskIDs = append(taskIDs, strconv.Itoa(tasks[i].TaskID))
		taskMap[tasks[i].TaskID] = &tasks[i]
	}

	logger.Infof("Querying comments for task IDs: %v", taskIDs)

	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")

	// Convert taskIDs to []int64 and pass as Postgres int array
	intIDs := make([]int64, 0, len(taskIDs))
	for _, idStr := range taskIDs {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			intIDs = append(intIDs, id)
		}
	}

	queryWithTaskIDs := `
        SELECT task_id, description
        FROM time_entries 
        WHERE task_id = ANY($1) 
        AND date >= $2 AND date <= $3
        AND description IS NOT NULL 
        AND description != ''
        ORDER BY task_id, date DESC`

	rows, err := db.QueryContext(ctx, queryWithTaskIDs, pq.Array(intIDs), startDateStr, endDateStr)
	if err != nil {
		if ctx != nil && ctx.Err() == context.DeadlineExceeded {
			logger.Errorf("Comments query timed out")
		} else {
			logger.Errorf("Failed to query comments: %v", err)
		}
		return tasks
	}
	defer rows.Close()

	// Map to collect comments by task_id
	taskComments := make(map[int][]string)
	commentCount := 0

	for rows.Next() {
		var taskID int
		var description sql.NullString

		if err := rows.Scan(&taskID, &description); err != nil {
			logger.Errorf("Failed to scan comment row: %v", err)
			continue
		}

		if description.Valid && description.String != "" {
			comments := strings.Split(description.String, "; ")
			taskComments[taskID] = append(taskComments[taskID], comments...)
			commentCount++
		}
	}

	logger.Infof("Found %d comment entries for %d tasks", commentCount, len(taskComments))

	// Add comments to tasks and remove duplicates
	for taskID, comments := range taskComments {
		if task, exists := taskMap[taskID]; exists {
			// Remove duplicates and empty strings
			uniqueComments := make([]string, 0)
			seen := make(map[string]bool)

			for _, comment := range comments {
				comment = strings.TrimSpace(comment)
				if comment != "" && !seen[comment] {
					uniqueComments = append(uniqueComments, comment)
					seen[comment] = true
				}
			}

			task.Comments = uniqueComments
		}
	}

	logger.Infof("Completed adding comments to tasks")
	return tasks
}

// addCommentsToTasksWithTimeout wraps addCommentsToTasksCtx with a 10s timeout
func addCommentsToTasksWithTimeout(tasks []TaskInfo, startTime time.Time, endTime time.Time) []TaskInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return addCommentsToTasksCtx(ctx, tasks, startTime, endTime)
}

// fully AI generated
func addCommentsToTasks(tasks []TaskInfo, startTime time.Time, endTime time.Time) []TaskInfo {
	return addCommentsToTasksCtx(context.Background(), tasks, startTime, endTime)
}
