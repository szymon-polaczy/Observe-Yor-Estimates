package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// getOutputJSON already defined in main.go

// outputJSON outputs messages as JSON to stdout
func outputJSON(messages []SlackMessage) {
	for _, message := range messages {
		jsonData, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling message: %v\n", err)
			continue
		}
		fmt.Println(string(jsonData))
	}
}

// getTaskChanges gets task changes for a specific period
func getTaskChanges(db *sql.DB, period string) ([]TaskUpdateInfo, error) {
	switch period {
	case "daily":
		return GetTaskTimeEntries(db)
	case "weekly":
		return GetWeeklyTaskTimeEntries(db)
	case "monthly":
		return GetMonthlyTaskTimeEntries(db)
	default:
		return GetTaskTimeEntries(db)
	}
}

// getTaskChangesWithProject gets task changes for a specific period and project
func getTaskChangesWithProject(db *sql.DB, period string, projectTaskID *int) ([]TaskUpdateInfo, error) {
	switch period {
	case "daily":
		return GetTaskTimeEntriesWithProject(db, projectTaskID)
	case "weekly":
		return GetWeeklyTaskTimeEntriesWithProject(db, projectTaskID)
	case "monthly":
		return GetMonthlyTaskTimeEntriesWithProject(db, projectTaskID)
	default:
		return GetTaskTimeEntriesWithProject(db, projectTaskID)
	}
}

// sendSlackMessage sends a message via webhook
func sendSlackMessage(message SlackMessage) error {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("SLACK_WEBHOOK_URL not configured")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling message: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// getAllTasks retrieves all tasks from database
func getAllTasks(db *sql.DB) (map[int]Task, error) {
	query := `SELECT task_id, parent_id, name FROM tasks ORDER BY task_id`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	tasks := make(map[int]Task)
	for rows.Next() {
		var task Task
		err := rows.Scan(&task.ID, &task.ParentID, &task.Name)
		if err != nil {
			continue
		}
		tasks[task.ID] = task
	}

	return tasks, nil
}

// groupTasksByTopParent groups tasks by their top-level parent project
func groupTasksByTopParent(taskInfos []TaskUpdateInfo, allTasks map[int]Task) map[string][]TaskUpdateInfo {
	groups := make(map[string][]TaskUpdateInfo)
	
	for _, taskInfo := range taskInfos {
		projectName := getProjectNameForTask(taskInfo.TaskID, allTasks)
		if projectName == "" {
			projectName = "Other"
		}
		groups[projectName] = append(groups[projectName], taskInfo)
	}
	
	return groups
}

// formatProjectMessageWithComments formats a message for a specific project
func formatProjectMessageWithComments(project string, tasks []TaskUpdateInfo, period string, projectNum, totalProjects int) SlackMessage {
	// Convert to TaskInfo and use new formatting
	convertedTasks := convertTaskUpdateInfoToTaskInfo(tasks)
	
	return FormatTaskMessage(convertedTasks, period, FormatOptions{
		ShowHeader: true,
		ShowFooter: true,
	})
}

// convertTaskInfoToTaskUpdateInfo converts TaskInfo back to TaskUpdateInfo for compatibility
func convertTaskInfoToTaskUpdateInfo(tasks []TaskInfo) []TaskUpdateInfo {
	var taskUpdates []TaskUpdateInfo
	for _, task := range tasks {
		taskUpdate := TaskUpdateInfo{
			TaskID:           task.TaskID,
			ParentID:         task.ParentID,
			Name:             task.Name,
			EstimationInfo:   task.EstimationInfo.Text,
			CurrentPeriod:    task.CurrentPeriod,
			CurrentTime:      task.CurrentTime,
			PreviousPeriod:   task.PreviousPeriod,
			PreviousTime:     task.PreviousTime,
			DaysWorked:       task.DaysWorked,
			Comments:         task.Comments,
			UserBreakdown:    task.UserBreakdown,
		}
		taskUpdates = append(taskUpdates, taskUpdate)
	}
	return taskUpdates
}