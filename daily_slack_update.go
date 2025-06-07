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
	logger := NewLogger()
	logger.Info("Starting daily Slack update")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		// Send a notification to user about the failure if webhook is configured
		sendFailureNotification("Database connection failed", err)
		return
	}
	defer CloseWithErrorLog(db, "database connection")

	taskInfos, err := getTaskTimeChanges(db)
	if err != nil {
		logger.Errorf("Failed to get task time changes: %v", err)
		sendFailureNotification("Failed to retrieve task changes", err)
		return
	}

	if len(taskInfos) == 0 {
		logger.Info("No task changes to report today")
		// Still send a brief update to let users know the system is working
		if err := sendNoChangesNotification(); err != nil {
			logger.Errorf("Failed to send 'no changes' notification: %v", err)
		}
		return
	}

	message := formatSlackMessage(taskInfos)

	// Check if we have webhook URL configured
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warn("SLACK_WEBHOOK_URL not configured. Daily update would contain:")
		logger.Info(strings.Repeat("=", 50))
		logger.Info(message.Text)
		logger.Info(strings.Repeat("=", 50))
		return
	}

	err = sendSlackMessage(message)
	if err != nil {
		logger.Errorf("Failed to send Slack message: %v", err)
		return
	}

	logger.Info("Daily Slack update sent successfully")
}

// getTaskTimeChanges retrieves task time changes between yesterday and today
func getTaskTimeChanges(db *sql.DB) ([]TaskTimeInfo, error) {
	logger := NewLogger()

	// Get actual time entries data from the database
	logger.Debug("Querying tasks with recent time entries")

	return GetTaskTimeEntries(db)
}

// parseEstimation extracts estimation information from task name
func parseEstimation(taskName string) (string, string) {
	logger := NewLogger()

	// Regex to match patterns like [7-13] or [13-29]
	re := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	matches := re.FindStringSubmatch(taskName)

	if len(matches) != 3 {
		logger.Debugf("No estimation pattern found in task name: %s", taskName)
		return "", "no estimation given"
	}

	optimistic, err1 := strconv.Atoi(matches[1])
	pessimistic, err2 := strconv.Atoi(matches[2])

	if err1 != nil || err2 != nil {
		logger.Warnf("Failed to parse estimation numbers from task name '%s': optimistic=%v, pessimistic=%v",
			taskName, err1, err2)
		return "", "invalid estimation format"
	}

	if optimistic > pessimistic {
		logger.Warnf("Invalid estimation range in task '%s': optimistic (%d) > pessimistic (%d)",
			taskName, optimistic, pessimistic)
		return fmt.Sprintf("Estimation: %d-%d hours", optimistic, pessimistic), "broken estimation (optimistic > pessimistic)"
	}

	logger.Debugf("Parsed estimation for task '%s': %d-%d hours", taskName, optimistic, pessimistic)

	return fmt.Sprintf("Estimation: %d-%d hours", optimistic, pessimistic), ""
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

			// Add color indicator to plain text version
			percentage, _, err := calculateTimeUsagePercentage(task)
			if err == nil {
				emoji, description, _ := getColorIndicator(percentage)
				progressBar := generateProgressBar(percentage)
				messageText.WriteString(fmt.Sprintf("\n• Usage: %s %s %s", emoji, description, progressBar))
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

	// Calculate percentage and get color indicator
	var accessory *Accessory
	estimationText := ""

	if task.EstimationInfo != "" {
		estimationText = task.EstimationInfo
		if task.EstimationStatus != "" {
			estimationText += fmt.Sprintf("\n_(%s)_", task.EstimationStatus)
		}

		// Try to calculate percentage usage
		percentage, _, err := calculateTimeUsagePercentage(task)
		if err == nil {
			emoji, _, _ := getColorIndicator(percentage)

			accessory = &Accessory{
				Type: "button",
				Text: &Text{
					Type: "plain_text",
					Text: fmt.Sprintf("%s %.0f%%", emoji, percentage),
				},
			}
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

	// Create the main section block
	sectionBlock := Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s*", task.Name),
		},
		Accessory: accessory,
	}

	return []Block{
		sectionBlock,
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

// Accessory represents a Slack block accessory element
type Accessory struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
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

// sendFailureNotification sends a notification about system failures
func sendFailureNotification(operation string, err error) {
	logger := NewLogger()

	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warnf("Cannot send failure notification - SLACK_WEBHOOK_URL not configured. %s failed: %v", operation, err)
		return
	}

	message := SlackMessage{
		Text: fmt.Sprintf("⚠️ System Alert: %s failed", operation),
		Blocks: []Block{
			{
				Type: "header",
				Text: &Text{
					Type: "plain_text",
					Text: "⚠️ System Alert",
				},
			},
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Operation:* %s\n*Error:* `%v`\n*Time:* %s",
						operation, err, time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	if sendErr := sendSlackMessage(message); sendErr != nil {
		logger.Errorf("Failed to send failure notification: %v", sendErr)
	}
}

// sendNoChangesNotification sends a brief notification when there are no changes
func sendNoChangesNotification() error {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return nil // Not an error, just not configured
	}

	message := SlackMessage{
		Text: "📊 Daily Update: No task changes today",
		Blocks: []Block{
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("📊 *Daily Update* - %s\n\nNo task changes to report today. System is running normally.",
						time.Now().Format("January 2, 2006")),
				},
			},
		},
	}

	return sendSlackMessage(message)
}

// calculateTimeUsagePercentage calculates the percentage of estimation used based on total time spent
func calculateTimeUsagePercentage(task TaskTimeInfo) (float64, int, error) {
	logger := NewLogger()

	// Parse the pessimistic (maximum) estimation from task name
	re := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	matches := re.FindStringSubmatch(task.Name)

	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("no estimation pattern found")
	}

	pessimistic, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse pessimistic estimation: %w", err)
	}

	// Convert time strings back to seconds for calculation
	yesterdaySeconds := parseTimeToSeconds(task.YesterdayTime)
	todaySeconds := parseTimeToSeconds(task.TodayTime)
	totalSeconds := yesterdaySeconds + todaySeconds

	// Convert pessimistic estimation from hours to seconds
	pessimisticSeconds := pessimistic * 3600

	// Calculate percentage
	percentage := (float64(totalSeconds) / float64(pessimisticSeconds)) * 100

	logger.Debugf("Task '%s': %d total seconds vs %d pessimistic seconds = %.1f%%",
		task.Name, totalSeconds, pessimisticSeconds, percentage)

	return percentage, pessimisticSeconds, nil
}

// parseTimeToSeconds converts time strings like "1h 30m" or "45m" back to seconds
func parseTimeToSeconds(timeStr string) int {
	if timeStr == "0h 0m" {
		return 0
	}

	var hours, minutes int

	// Try to parse "Xh Ym" format
	if strings.Contains(timeStr, "h") && strings.Contains(timeStr, "m") {
		fmt.Sscanf(timeStr, "%dh %dm", &hours, &minutes)
	} else if strings.Contains(timeStr, "h") {
		// Just hours
		fmt.Sscanf(timeStr, "%dh", &hours)
	} else if strings.Contains(timeStr, "m") {
		// Just minutes
		fmt.Sscanf(timeStr, "%dm", &minutes)
	}

	return hours*3600 + minutes*60
}

// getColorIndicator returns emoji and formatting based on percentage
func getColorIndicator(percentage float64) (string, string, bool) {
	var emoji, description string
	var isBold bool

	// Get thresholds from environment variables with defaults
	midPoint := 50.0  // default
	highPoint := 90.0 // default

	if midPointStr := os.Getenv("MID_POINT"); midPointStr != "" {
		if parsed, err := strconv.ParseFloat(midPointStr, 64); err == nil {
			midPoint = parsed
		}
	}

	if highPointStr := os.Getenv("HIGH_POINT"); highPointStr != "" {
		if parsed, err := strconv.ParseFloat(highPointStr, 64); err == nil {
			highPoint = parsed
		}
	}

	switch {
	case percentage == 0:
		emoji = "⚫"
	case percentage > 0 && percentage <= midPoint:
		emoji = "🟢"
	case percentage > midPoint && percentage <= highPoint:
		emoji = "🟠"
	case percentage > highPoint:
		emoji = "🔴"
	default:
		emoji = "⚫"
		description = "unknown"
	}

	return emoji, description, isBold
}

// generateProgressBar creates a visual progress bar for the percentage
func generateProgressBar(percentage float64) string {
	const barLength = 10
	filledBars := int((percentage / 100) * float64(barLength))

	if filledBars > barLength {
		filledBars = barLength
	}

	var bar strings.Builder
	for i := 0; i < filledBars; i++ {
		bar.WriteString("█")
	}
	for i := filledBars; i < barLength; i++ {
		bar.WriteString("░")
	}

	return bar.String()
}
