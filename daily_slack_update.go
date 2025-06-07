package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TaskTimeInfo represents time tracking information for a task
type TaskTimeInfo struct {
	TaskID           int
	Name             string
	YesterdayTime    string
	TodayTime        string
	StartTime        string
	EstimationInfo   string
	EstimationStatus string
}

// SlackMessage represents the structure of a Slack message
type SlackMessage struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

// SendDailySlackUpdate sends a daily update to Slack with task changes
func SendDailySlackUpdate() {
	db, err := GetDB()
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	defer db.Close()

	taskInfos, err := getTaskTimeChanges(db)
	if err != nil {
		fmt.Printf("Error getting task time changes: %v\n", err)
		return
	}

	if len(taskInfos) == 0 {
		fmt.Println("No task changes to report today")
		return
	}

	message := formatSlackMessage(taskInfos)

	// Check if we have webhook URL configured
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		fmt.Println("SLACK_WEBHOOK_URL not configured. Here's what would be sent:")
		fmt.Println(strings.Repeat("=", 50))
		fmt.Println(message.Text)
		fmt.Println(strings.Repeat("=", 50))
		return
	}

	err = sendSlackMessage(message)
	if err != nil {
		fmt.Printf("Error sending Slack message: %v\n", err)
		return
	}

	fmt.Println("Daily Slack update sent successfully")
}

// getTaskTimeChanges retrieves task time changes between yesterday and today
func getTaskTimeChanges(db *sql.DB) ([]TaskTimeInfo, error) {
	// For this example, we'll simulate task time data since we don't have actual time tracking
	// In a real scenario, you'd query your time tracking data
	query := `
		SELECT t.task_id, t.name 
		FROM tasks t 
		WHERE t.task_id IN (
			SELECT DISTINCT task_id 
			FROM task_history 
			WHERE DATE(timestamp) >= DATE('now', '-1 day')
		)
		LIMIT 10
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying tasks: %w", err)
	}
	defer rows.Close()

	var taskInfos []TaskTimeInfo

	for rows.Next() {
		var taskInfo TaskTimeInfo
		err := rows.Scan(&taskInfo.TaskID, &taskInfo.Name)
		if err != nil {
			continue
		}

		// Extract estimation information from task name
		taskInfo.EstimationInfo, taskInfo.EstimationStatus = parseEstimation(taskInfo.Name)

		// Simulate time data (in a real app, you'd get this from your time tracking system)
		taskInfo.YesterdayTime = simulateTimeData("yesterday")
		taskInfo.TodayTime = simulateTimeData("today")
		taskInfo.StartTime = simulateTimeData("start")

		taskInfos = append(taskInfos, taskInfo)
	}

	return taskInfos, nil
}

// parseEstimation extracts estimation information from task name
func parseEstimation(taskName string) (string, string) {
	// Regex to match patterns like [7-13] or [13-29]
	re := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	matches := re.FindStringSubmatch(taskName)

	if len(matches) != 3 {
		return "", "no estimation given"
	}

	optimistic, err1 := strconv.Atoi(matches[1])
	pessimistic, err2 := strconv.Atoi(matches[2])

	if err1 != nil || err2 != nil {
		return "", "no estimation given"
	}

	if optimistic > pessimistic {
		return fmt.Sprintf("Estimation: %d-%d hours", optimistic, pessimistic), "broken estimation"
	}

	// Calculate percentage used (simulated with random values for now)
	// In real implementation, you'd calculate based on actual time spent
	percentageUsed := simulatePercentageUsed()

	return fmt.Sprintf("Estimation: %d-%d hours (%d%% used)", optimistic, pessimistic, percentageUsed), ""
}

// simulateTimeData simulates time tracking data for demonstration
func simulateTimeData(timeType string) string {
	switch timeType {
	case "yesterday":
		return "2h 30m"
	case "today":
		return "1h 45m"
	case "start":
		return time.Now().AddDate(0, 0, -3).Format("2006-01-02 15:04")
	default:
		return "0h 0m"
	}
}

// simulatePercentageUsed simulates percentage calculation for estimation usage
func simulatePercentageUsed() int {
	// In real implementation, calculate: (actualTimeSpent / estimatedTime) * 100
	return 35 // Simulated value
}

// formatSlackMessage formats the task information into a Slack message
func formatSlackMessage(taskInfos []TaskTimeInfo) SlackMessage {
	var messageText strings.Builder
	messageText.WriteString("📊 *Daily Task Update* - ")
	messageText.WriteString(time.Now().Format("January 2, 2006"))
	messageText.WriteString("\n\n")

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{
				Type: "plain_text",
				Text: "📊 Daily Task Update",
			},
		},
		{
			Type: "context",
			Elements: []Element{
				{
					Type: "mrkdwn",
					Text: time.Now().Format("*January 2, 2006*"),
				},
			},
		},
		{
			Type: "divider",
		},
	}

	for _, task := range taskInfos {
		taskBlock := formatTaskBlock(task)
		blocks = append(blocks, taskBlock...)

		// Add to plain text version too
		messageText.WriteString(fmt.Sprintf("*%s*\n", task.Name))
		messageText.WriteString(fmt.Sprintf("• Start: %s\n", task.StartTime))
		messageText.WriteString(fmt.Sprintf("• Yesterday: %s\n", task.YesterdayTime))
		messageText.WriteString(fmt.Sprintf("• Today: %s\n", task.TodayTime))
		if task.EstimationInfo != "" {
			messageText.WriteString(fmt.Sprintf("• %s", task.EstimationInfo))
			if task.EstimationStatus != "" {
				messageText.WriteString(fmt.Sprintf(" (%s)", task.EstimationStatus))
			}
			messageText.WriteString("\n")
		}
		messageText.WriteString("\n")
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// formatTaskBlock formats a single task into Slack blocks
func formatTaskBlock(task TaskTimeInfo) []Block {
	var fields []Field

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Start Time:*\n%s", task.StartTime),
	})

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Yesterday:*\n%s", task.YesterdayTime),
	})

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Today:*\n%s", task.TodayTime),
	})

	if task.EstimationInfo != "" {
		estimationText := task.EstimationInfo
		if task.EstimationStatus != "" {
			estimationText += fmt.Sprintf("\n_(%s)_", task.EstimationStatus)
		}
		fields = append(fields, Field{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Estimation:*\n%s", estimationText),
		})
	} else {
		fields = append(fields, Field{
			Type: "mrkdwn",
			Text: "*Estimation:*\n_no estimation given_",
		})
	}

	return []Block{
		{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s*", task.Name),
			},
		},
		{
			Type:   "section",
			Fields: fields,
		},
		{
			Type: "divider",
		},
	}
}

// Element represents a Slack block element
type Element struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Field represents a Slack block field
type Field struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// sendSlackMessage sends a message to Slack using the webhook
func sendSlackMessage(message SlackMessage) error {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("SLACK_WEBHOOK_URL environment variable not set")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling message: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
