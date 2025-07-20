package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// RunThresholdMonitoring checks for tasks that crossed thresholds and sends alerts
func RunThresholdMonitoring() error {
	logger := GetGlobalLogger()
	logger.Debug("Starting threshold monitoring check")

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	alerts, err := CheckThresholdAlerts(db)
	if err != nil {
		return fmt.Errorf("failed to check threshold alerts: %w", err)
	}

	if len(alerts) == 0 {
		logger.Debug("No threshold crossings detected")
		return nil
	}

	logger.Infof("Detected %d threshold crossings, sending alerts", len(alerts))
	return SendThresholdAlerts(alerts)
}

// CheckThresholdAlerts checks for tasks crossing specific thresholds
func CheckThresholdAlerts(db *sql.DB) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()
	thresholds := []int{50, 80, 90, 100}
	var allAlerts []ThresholdAlert

	now := time.Now()
	sixHoursAgo := now.Add(-6 * time.Hour)
	oneHourAgo := now.Add(-1 * time.Hour)

	for _, threshold := range thresholds {
		// Check for newly crossed thresholds
		alerts, err := getTasksJustCrossedThreshold(db, float64(threshold), sixHoursAgo)
		if err != nil {
			logger.Errorf("Failed to check %d%% threshold crossings: %v", threshold, err)
		} else {
			allAlerts = append(allAlerts, alerts...)
		}

		// For 100% threshold, send periodic alerts for worsening tasks
		if threshold == 100 {
			persistentAlerts, err := getTasksPersistentlyOverThreshold(db, float64(threshold), oneHourAgo)
			if err != nil {
				logger.Errorf("Failed to check persistent %d%% threshold tasks: %v", threshold, err)
			} else {
				allAlerts = append(allAlerts, persistentAlerts...)
			}
		}
	}

	return allAlerts, nil
}

// getTasksJustCrossedThreshold finds tasks that crossed a threshold recently
func getTasksJustCrossedThreshold(db *sql.DB, threshold float64, since time.Time) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()
	
	query := `
		WITH recent_entries AS (
			SELECT task_id, duration
			FROM time_entries 
			WHERE date = to_char(CURRENT_DATE, 'YYYY-MM-DD') AND duration > 0
		),
		tasks_with_estimations AS (
			SELECT DISTINCT t.task_id, t.parent_id, t.name
			FROM tasks t
			INNER JOIN recent_entries re ON t.task_id = re.task_id
			WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'
		),
		task_totals AS (
			SELECT 
				twe.task_id, twe.parent_id, twe.name,
				COALESCE(SUM(te.duration), 0) as total_duration,
				COALESCE(SUM(CASE WHEN re.task_id IS NOT NULL THEN re.duration ELSE 0 END), 0) as recent_duration
			FROM tasks_with_estimations twe
			LEFT JOIN time_entries te ON twe.task_id = te.task_id
			LEFT JOIN recent_entries re ON twe.task_id = re.task_id
			GROUP BY twe.task_id, twe.parent_id, twe.name
		)
		SELECT task_id, parent_id, name, total_duration, recent_duration
		FROM task_totals
		WHERE total_duration > 0 AND recent_duration > 0`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("could not query threshold crossings: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert

	for rows.Next() {
		var taskID, parentID, totalDuration, recentDuration int
		var name string

		err := rows.Scan(&taskID, &parentID, &name, &totalDuration, &recentDuration)
		if err != nil {
			logger.Warnf("Failed to scan task row: %v", err)
			continue
		}

		// Parse estimation and calculate percentages
		estimation := ParseTaskEstimation(name)
		if estimation.ErrorMessage != "" {
			continue
		}

		previousTotal := totalDuration - recentDuration
		currentPercentage, _ := CalcUsagePercent(
			FormatDuration(recentDuration),
			FormatDuration(previousTotal),
			estimation)
		previousPercentage, _ := CalcUsagePercent(
			"0h 0m",
			FormatDuration(previousTotal),
			estimation)

		// Check if threshold was just crossed
		if previousPercentage < threshold && currentPercentage >= threshold {
			// Check if already notified
			alreadyNotified, err := hasNotificationBeenSent(db, taskID, int(threshold))
			if err != nil || alreadyNotified {
				continue
			}

			alert := ThresholdAlert{
				TaskID:           taskID,
				ParentID:         parentID,
				Name:             name,
				CurrentTime:      FormatDuration(totalDuration),
				PreviousTime:     FormatDuration(previousTotal),
				Percentage:       currentPercentage,
				ThresholdCrossed: int(threshold),
				JustCrossed:      true,
			}

			alert.EstimationInfo = ParseTaskEstimationWithUsage(alert.Name, alert.CurrentTime, alert.PreviousTime)
			alerts = append(alerts, alert)

			logger.Infof("THRESHOLD CROSSING: Task %s crossed %.1f%% threshold", name, threshold)
		}
	}

	return alerts, nil
}

// getTasksPersistentlyOverThreshold finds tasks over 100% that got worse
func getTasksPersistentlyOverThreshold(db *sql.DB, threshold float64, since time.Time) ([]ThresholdAlert, error) {
	if threshold != 100 {
		return []ThresholdAlert{}, nil
	}

	query := `
		WITH recent_entries AS (
			SELECT task_id, duration
			FROM time_entries 
			WHERE modify_time >= $1 AND duration > 0
		),
		tasks_with_estimations AS (
			SELECT DISTINCT t.task_id, t.parent_id, t.name
			FROM tasks t
			INNER JOIN recent_entries re ON t.task_id = re.task_id
			WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'
		),
		task_totals AS (
			SELECT 
				twe.task_id, twe.parent_id, twe.name,
				COALESCE(SUM(te.duration), 0) as total_duration,
				COALESCE(SUM(CASE WHEN re.task_id IS NOT NULL THEN re.duration ELSE 0 END), 0) as recent_duration
			FROM tasks_with_estimations twe
			LEFT JOIN time_entries te ON twe.task_id = te.task_id
			LEFT JOIN recent_entries re ON twe.task_id = re.task_id
			GROUP BY twe.task_id, twe.parent_id, twe.name
		)
		SELECT task_id, parent_id, name, total_duration, recent_duration
		FROM task_totals
		WHERE total_duration > 0 AND recent_duration > 0`

	rows, err := db.Query(query, since)
	if err != nil {
		return nil, fmt.Errorf("could not query persistent threshold violations: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert
	for rows.Next() {
		var taskID, parentID, totalDuration, recentDuration int
		var name string

		err := rows.Scan(&taskID, &parentID, &name, &totalDuration, &recentDuration)
		if err != nil {
			continue
		}

		estimation := ParseTaskEstimation(name)
		if estimation.ErrorMessage != "" {
			continue
		}

		previousTotal := totalDuration - recentDuration
		currentPercentage, _ := CalcUsagePercent(
			FormatDuration(recentDuration),
			FormatDuration(previousTotal),
			estimation)
		previousPercentage, _ := CalcUsagePercent(
			"0h 0m",
			FormatDuration(previousTotal),
			estimation)

		// Alert if task was already over 100% and got worse
		if currentPercentage >= threshold && previousPercentage >= threshold && currentPercentage > previousPercentage {
			alert := ThresholdAlert{
				TaskID:           taskID,
				ParentID:         parentID,
				Name:             name,
				CurrentTime:      FormatDuration(totalDuration),
				PreviousTime:     FormatDuration(previousTotal),
				Percentage:       currentPercentage,
				ThresholdCrossed: int(threshold),
				JustCrossed:      false,
			}

			alert.EstimationInfo = ParseTaskEstimationWithUsage(alert.Name, alert.CurrentTime, alert.PreviousTime)
			alerts = append(alerts, alert)
		}
	}

	return alerts, nil
}

// SendThresholdAlerts sends Slack notifications for threshold crossings
func SendThresholdAlerts(alerts []ThresholdAlert) error {
	if len(alerts) == 0 {
		return nil
	}

	logger := GetGlobalLogger()

	// Group alerts by threshold and type
	newCrossingGroups := make(map[int][]ThresholdAlert)
	persistentGroups := make(map[int][]ThresholdAlert)

	for _, alert := range alerts {
		if alert.JustCrossed {
			newCrossingGroups[alert.ThresholdCrossed] = append(newCrossingGroups[alert.ThresholdCrossed], alert)
		} else {
			persistentGroups[alert.ThresholdCrossed] = append(persistentGroups[alert.ThresholdCrossed], alert)
		}
	}

	// Get all tasks for hierarchy mapping
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	allTasks, err := getAllTasks(db)
	if err != nil {
		return fmt.Errorf("failed to get all tasks for hierarchy mapping: %w", err)
	}

	// Send messages for new threshold crossings
	for threshold, thresholdAlerts := range newCrossingGroups {
		err := sendThresholdAlertsForGroup(thresholdAlerts, threshold, true, allTasks, db)
		if err != nil {
			logger.Errorf("Failed to send new crossing alerts for %d%% threshold: %v", threshold, err)
		}
	}

	// Send messages for persistent threshold violations (100% only)
	for threshold, thresholdAlerts := range persistentGroups {
		if threshold == 100 {
			err := sendThresholdAlertsForGroup(thresholdAlerts, threshold, false, allTasks, db)
			if err != nil {
				logger.Errorf("Failed to send persistent alerts for %d%% threshold: %v", threshold, err)
			}
		}
	}

	return nil
}

// sendThresholdAlertsForGroup sends alerts for a specific threshold group
func sendThresholdAlertsForGroup(thresholdAlerts []ThresholdAlert, threshold int, isNewCrossing bool, allTasks map[int]Task, db *sql.DB) error {
	logger := GetGlobalLogger()

	// Convert ThresholdAlert to TaskInfo for compatibility with existing formatting
	var taskInfos []TaskInfo
	for _, alert := range thresholdAlerts {
		taskInfo := TaskInfo{
			TaskID:         alert.TaskID,
			ParentID:       alert.ParentID,
			Name:           alert.Name,
			EstimationInfo: alert.EstimationInfo,
			CurrentPeriod:  "Current",
			CurrentTime:    alert.CurrentTime,
			PreviousPeriod: "Previous",
			PreviousTime:   alert.PreviousTime,
			Comments:       []string{},
		}
		taskInfos = append(taskInfos, taskInfo)
	}

	// Group by project
	projectGroups := GroupTasksByProject(taskInfos, allTasks)

	// Format and send alert messages
	for project, tasks := range projectGroups {
		message := formatThresholdAlertMessage(project, tasks, threshold, isNewCrossing)

		if err := sendSlackMessage(message); err != nil {
			logger.Errorf("Failed to send threshold alert for %s at %d%%: %v", project, threshold, err)
		} else {
			alertType := "new crossing"
			if !isNewCrossing {
				alertType = "persistent violation"
			}
			logger.Infof("Sent %s alert for %s: %d tasks at %d%% threshold", alertType, project, len(tasks), threshold)

			// Record the notifications as sent
			for _, task := range tasks {
				var percentage float64
				for _, alert := range thresholdAlerts {
					if alert.TaskID == task.TaskID {
						percentage = alert.Percentage
						break
					}
				}

				if err := recordNotificationSent(db, task.TaskID, threshold, percentage); err != nil {
					logger.Warnf("Failed to record notification for task %d, threshold %d: %v", task.TaskID, threshold, err)
				}
			}
		}
	}

	return nil
}

// formatThresholdAlertMessage formats a threshold crossing alert message
func formatThresholdAlertMessage(project string, tasks []TaskInfo, threshold int, isNewCrossing bool) SlackMessage {
	options := FormatOptions{
		ShowHeader: true,
		ShowFooter: true,
		Threshold:  func() *float64 { f := float64(threshold); return &f }(),
	}

	period := "threshold alert"
	if isNewCrossing {
		period = "threshold crossing"
	}

	return FormatTaskMessage(tasks, period, options)
}

// hasNotificationBeenSent checks if we've already sent a notification for this task/threshold
func hasNotificationBeenSent(db *sql.DB, taskID int, threshold int) (bool, error) {
	query := `
		SELECT 1 
		FROM threshold_notifications 
		WHERE task_id = $1 AND threshold_percentage = $2 
		  AND last_time_entry_date = to_char(CURRENT_DATE, 'YYYY-MM-DD')
		LIMIT 1`

	var dummy int
	err := db.QueryRow(query, taskID, threshold).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// recordNotificationSent records that we sent a notification for this task/threshold
func recordNotificationSent(db *sql.DB, taskID int, threshold int, percentage float64) error {
	query := `
		INSERT INTO threshold_notifications 
		(task_id, threshold_percentage, current_percentage, last_time_entry_date)
		VALUES ($1, $2, $3, to_char(CURRENT_DATE, 'YYYY-MM-DD'))
		ON CONFLICT (task_id, threshold_percentage) 
		DO UPDATE SET 
			current_percentage = EXCLUDED.current_percentage,
			notified_at = CURRENT_TIMESTAMP,
			last_time_entry_date = EXCLUDED.last_time_entry_date`

	_, err := db.Exec(query, taskID, threshold, percentage)
	return err
}

// GetTasksOverThreshold returns tasks that are over a specific percentage
func GetTasksOverThreshold(db *sql.DB, threshold float64, period string, days int) ([]TaskInfo, error) {
	return GetTasksOverThresholdWithProject(db, threshold, period, days, nil)
}

// GetTasksOverThresholdWithProject returns tasks over threshold, optionally filtered by project
func GetTasksOverThresholdWithProject(db *sql.DB, threshold float64, period string, days int, projectTaskID *int) ([]TaskInfo, error) {
	dateRanges := CalcDateRanges(period, days)
	fromDate := dateRanges.Current.Start
	toDate := dateRanges.Current.End

	// Build project filtering
	var projectFilterClause string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, fromDate, toDate)

	if projectTaskID != nil {
		projectTaskIDs, err := GetProjectTaskIDs(db, *projectTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project task IDs: %w", err)
		}
		if len(projectTaskIDs) == 0 {
			return []TaskInfo{}, nil
		}
		projectFilterClause = " AND t.task_id = ANY($3)"
		queryArgs = append(queryArgs, pq.Array(projectTaskIDs))
	}

	query := fmt.Sprintf(`
		SELECT 
			t.task_id, t.parent_id, t.name,
			COALESCE(SUM(CASE WHEN te.date BETWEEN $1 AND $2 THEN te.duration ELSE 0 END), 0) as current_duration,
			COALESCE(SUM(CASE WHEN te.date < $1 THEN te.duration ELSE 0 END), 0) as previous_duration
		FROM tasks t
		INNER JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'%s
		  AND te.date BETWEEN $1 AND $2
		GROUP BY t.task_id, t.parent_id, t.name
		ORDER BY t.task_id`, projectFilterClause)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("could not query tasks over threshold: %w", err)
	}
	defer rows.Close()

	var tasks []TaskInfo
	for rows.Next() {
		var task TaskInfo
		var currentDuration, previousDuration int

		err := rows.Scan(&task.TaskID, &task.ParentID, &task.Name, &currentDuration, &previousDuration)
		if err != nil {
			continue
		}

		task.CurrentTime = FormatDuration(currentDuration)
		task.PreviousTime = FormatDuration(previousDuration)

		// Calculate percentage and filter by threshold
		estimation := ParseTaskEstimationWithUsage(task.Name, task.CurrentTime, task.PreviousTime)
		if estimation.ErrorMessage != "" || estimation.Percentage < threshold {
			continue
		}

		task.EstimationInfo = estimation
		task.CurrentPeriod = GetPeriodDisplayName(period, days)
		task.PreviousPeriod = "Before " + task.CurrentPeriod

		tasks = append(tasks, task)
	}

	return tasks, nil
}