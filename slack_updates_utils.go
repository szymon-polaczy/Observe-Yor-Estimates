package main

import (
	"bytes"
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
	Comments         []string
}

// SlackMessage represents the structure of a Slack message
type SlackMessage struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

// Field represents a field in Slack blocks
type Field struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Element represents an element in Slack blocks
type Element struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Accessory represents an accessory in Slack blocks
type Accessory struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
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

// parseTimeToSeconds converts time strings like "1h 30m" or "45m" back to seconds
func parseTimeToSeconds(timeStr string) int {
	if timeStr == "0h 0m" || timeStr == "" {
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
		description = "no time"
	case percentage > 0 && percentage <= midPoint:
		emoji = "🟢"
		description = "on track"
	case percentage > midPoint && percentage <= highPoint:
		emoji = "🟠"
		description = "high usage"
	case percentage > highPoint:
		emoji = "🔴"
		description = "over budget"
	default:
		emoji = "⚫"
		description = "unknown"
	}

	return emoji, description, isBold
}

// calculateTimeUsagePercentage calculates the percentage of estimation used based on total time spent (daily)
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
