package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Threshold levels for notifications (50%, 70%, 90%, 100%)
var thresholdLevels = []int{100, 90, 70, 50}

// checkThresholdNotifications detects threshold crossings and sends notifications
// This function is called after time entries sync to check tasks that were updated
func checkThresholdNotifications(db *sql.DB, updatedTaskIDs []int) error {
	logger := GetGlobalLogger()

	if len(updatedTaskIDs) == 0 {
		logger.Debug("No updated tasks to check for threshold notifications")
		return nil
	}

	logger.Infof("Checking threshold notifications for %d updated tasks", len(updatedTaskIDs))

	// Get threshold alerts for updated tasks
	alerts, err := detectThresholdCrossings(db, updatedTaskIDs)
	if err != nil {
		return fmt.Errorf("failed to detect threshold crossings: %w", err)
	}

	if len(alerts) == 0 {
		logger.Debug("No threshold crossings detected")
		return nil
	}

	logger.Infof("Detected %d threshold crossings", len(alerts))

	// Send notifications to users
	return sendThresholdNotifications(db, alerts)
}

// detectThresholdCrossings finds tasks that have crossed threshold levels
func detectThresholdCrossings(db *sql.DB, taskIDs []int) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	// Get recent time entries for these tasks
	now := time.Now()
	startDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	// Build dynamic placeholders for task IDs
	if len(taskIDs) == 0 {
		return []ThresholdAlert{}, nil
	}

	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, 0, len(taskIDs)+2)

	// Add date parameters first
	args = append(args, startDate, endDate)

	// Add task IDs to args and create placeholders
	for i, taskID := range taskIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+3) // +3 because $1 and $2 are dates
		args = append(args, taskID)
	}

	// Build query with dynamic IN clause
	inClause := strings.Join(placeholders, ",")
	query := fmt.Sprintf(`
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE 
				WHEN te.date >= $1::text AND te.date <= $2::text
				THEN te.duration 
				ELSE 0 
			END), 0) as current_period_duration,
			COALESCE(SUM(te.duration), 0) as total_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.task_id IN (%s)
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(CASE 
			WHEN te.date >= $1::text AND te.date <= $2::text
			THEN te.duration 
			ELSE 0 
		END), 0) > 0
		ORDER BY t.name
	`, inClause)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query task usage: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert

	for rows.Next() {
		var taskID, parentID, currentDuration, totalDuration int
		var name string

		err := rows.Scan(&taskID, &parentID, &name, &currentDuration, &totalDuration)
		if err != nil {
			logger.Errorf("Failed to scan task usage row: %v", err)
			continue
		}

		// Calculate usage percentage using existing functions (same as main task handling)
		currentTime := formatDuration(currentDuration)
		totalTime := formatDuration(totalDuration)

		// Parse estimation from task name using total time to calculate usage
		estimation := ParseTaskEstimationWithUsage(name, totalTime, "0h 0m")
		if estimation.ErrorMessage != "" {
			logger.Debugf("Skipping task %d (%s): %s", taskID, name, estimation.ErrorMessage)
			continue
		}

		percentage := estimation.Percentage

		// Check and record threshold crossing in a transaction
		thresholdCrossed, shouldNotify, err := checkAndRecordThresholdCrossing(db, taskID, percentage)
		if err != nil {
			logger.Errorf("Failed to check threshold crossing for task %d: %v", taskID, err)
			continue
		}

		if shouldNotify {
			alert := ThresholdAlert{
				TaskID:           taskID,
				ParentID:         parentID,
				Name:             name,
				EstimationInfo:   estimation,
				CurrentTime:      currentTime,
				TotalDuration:    totalTime,
				Percentage:       percentage,
				ThresholdCrossed: thresholdCrossed,
				JustCrossed:      true,
			}
			alerts = append(alerts, alert)
		}
	}

	return alerts, nil
}

// checkAndRecordThresholdCrossing atomically checks and records threshold crossing
func checkAndRecordThresholdCrossing(db *sql.DB, taskID int, currentPercentage float64) (int, bool, error) {
	// Start a transaction for atomic check and record
	tx, err := db.Begin()
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Will be no-op if committed

	// Check what's the highest threshold this task has already been notified for (with row lock)
	var lastNotifiedThreshold sql.NullInt64
	query := `SELECT MAX(threshold_percentage) FROM threshold_notifications WHERE task_id = $1 FOR UPDATE`
	err = tx.QueryRow(query, taskID).Scan(&lastNotifiedThreshold)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, fmt.Errorf("failed to query threshold notifications: %w", err)
	}

	// Find the highest threshold the current percentage has crossed
	var highestCrossedThreshold int
	for _, threshold := range thresholdLevels {
		if currentPercentage >= float64(threshold) {
			highestCrossedThreshold = threshold
			break
		}
	}

	if highestCrossedThreshold == 0 {
		tx.Commit() // No work done, but commit to release lock
		return 0, false, nil // No threshold crossed
	}

	// Check if we've already notified for this threshold or higher
	if lastNotifiedThreshold.Valid && int(lastNotifiedThreshold.Int64) >= highestCrossedThreshold {
		tx.Commit() // No work done, but commit to release lock
		return highestCrossedThreshold, false, nil // Already notified
	}

	// Record the notification
	insertQuery := `
		INSERT INTO threshold_notifications (task_id, threshold_percentage, current_percentage, last_time_entry_date)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (task_id, threshold_percentage) 
		DO UPDATE SET 
			current_percentage = EXCLUDED.current_percentage,
			notified_at = CURRENT_TIMESTAMP,
			last_time_entry_date = EXCLUDED.last_time_entry_date
	`
	today := time.Now().Format("2006-01-02")
	_, err = tx.Exec(insertQuery, taskID, highestCrossedThreshold, currentPercentage, today)
	if err != nil {
		return 0, false, fmt.Errorf("failed to record threshold notification: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return highestCrossedThreshold, true, nil // New notification needed
}


// sendThresholdNotifications sends notifications to users about threshold crossings
func sendThresholdNotifications(db *sql.DB, alerts []ThresholdAlert) error {
	logger := GetGlobalLogger()

	// Get all Slack users
	users, err := GetSlackUsersFromDatabase()
	if err != nil {
		return fmt.Errorf("failed to get Slack users: %w", err)
	}

	logger.Infof("Sending threshold notifications to %d users", len(users))

	for _, user := range users {
		// Get user's assigned projects
		userProjects, err := GetUserProjects(db, user.ID)
		if err != nil {
			logger.Errorf("Failed to get projects for user %s: %v", user.ID, err)
			continue
		}

		// Filter alerts to only include user's projects
		userAlerts := filterAlertsForUser(alerts, userProjects)
		if len(userAlerts) == 0 {
			continue
		}

		// Convert ThresholdAlert to TaskInfo for existing messaging functions
		taskInfos := convertAlertsToTaskInfos(userAlerts)

		// Group by project using existing function
		projectGroups := groupTasksByProject(taskInfos)

		// Send using existing messaging function
		logger.Infof("Sending threshold notifications to user %s for %d tasks", user.ID, len(userAlerts))
		sendTasksGroupedByProjectToUser(user.ID, projectGroups)

		// Small delay between users to avoid rate limiting
		time.Sleep(250 * time.Millisecond)
	}

	return nil
}

// filterAlertsForUser filters alerts to only include tasks from user's assigned projects
func filterAlertsForUser(alerts []ThresholdAlert, userProjects []Project) []ThresholdAlert {
	if len(userProjects) == 0 {
		// If user has no project assignments, send all alerts (backward compatibility)
		return alerts
	}

	// Create a map of project task IDs for quick lookup
	projectTaskIDs := make(map[int]bool)
	for _, project := range userProjects {
		projectTaskIDs[project.TimeCampTaskID] = true
	}

	var filteredAlerts []ThresholdAlert
	for _, alert := range alerts {
		// Check if this task belongs to any of the user's projects
		// We need to check the task hierarchy to find the project
		if isTaskInUserProjects(alert.TaskID, alert.ParentID, projectTaskIDs) {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}

	return filteredAlerts
}

// isTaskInUserProjects checks if a task belongs to user's assigned projects
func isTaskInUserProjects(taskID, parentID int, projectTaskIDs map[int]bool) bool {
	// Direct project match
	if projectTaskIDs[taskID] {
		return true
	}

	// Parent project match (most common case - task under project)
	if projectTaskIDs[parentID] {
		return true
	}

	// TODO: Could add more sophisticated hierarchy checking if needed
	// For now, this covers the most common cases

	return false
}

// convertAlertsToTaskInfos converts ThresholdAlert to TaskInfo for messaging compatibility
func convertAlertsToTaskInfos(alerts []ThresholdAlert) []TaskInfo {
	var taskInfos []TaskInfo

	for _, alert := range alerts {
		taskInfo := TaskInfo{
			TaskID:         alert.TaskID,
			ParentID:       alert.ParentID,
			Name:           alert.Name,
			EstimationInfo: alert.EstimationInfo,
			CurrentTime:    alert.CurrentTime,
			TotalDuration:  alert.TotalDuration,
			DaysWorked:     1, // Not relevant for threshold notifications
			Comments:       []string{fmt.Sprintf("🚨 THRESHOLD ALERT: %d%% reached!", alert.ThresholdCrossed)},
		}

		taskInfos = append(taskInfos, taskInfo)
	}

	return taskInfos
}

