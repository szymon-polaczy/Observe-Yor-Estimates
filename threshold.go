package main

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

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
