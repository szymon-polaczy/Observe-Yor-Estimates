package main

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SendWeeklySlackUpdate sends a weekly update to Slack with task changes over the past week
func SendWeeklySlackUpdate() {
	logger := NewLogger()
	logger.Info("Starting weekly Slack update")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		// Send a notification to user about the failure if webhook is configured
		sendFailureNotification("Database connection failed", err)
		return
	}
	// Note: Using shared database connection, no need to close here

	taskInfos, err := getWeeklyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get weekly task changes: %v", err)
		sendFailureNotification("Failed to retrieve weekly task changes", err)
		return
	}

	if len(taskInfos) == 0 {
		logger.Info("No task changes to report this week")
		// Still send a brief update to let users know the system is working
		if err := sendNoWeeklyChangesNotification(); err != nil {
			logger.Errorf("Failed to send 'no weekly changes' notification: %v", err)
		}
		return
	}

	message := formatWeeklySlackMessage(taskInfos)

	// Check if we have webhook URL configured
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warn("SLACK_WEBHOOK_URL not configured. Weekly update would contain:")
		logger.Info(strings.Repeat("=", 50))
		logger.Info(message.Text)
		logger.Info(strings.Repeat("=", 50))
		return
	}

	err = sendSlackMessage(message)
	if err != nil {
		logger.Errorf("Failed to send weekly Slack message: %v", err)
		return
	}

	logger.Info("Weekly Slack update sent successfully")
}

// getWeeklyTaskChanges retrieves task time changes for the past week
func getWeeklyTaskChanges(db *sql.DB) ([]WeeklyTaskTimeInfo, error) {
	logger := NewLogger()

	// Get actual time entries data from the database for the past week
	logger.Debug("Querying tasks with weekly time entries")

	return GetWeeklyTaskTimeEntries(db)
}

// formatWeeklySlackMessage formats the weekly task information into a Slack message
func formatWeeklySlackMessage(taskInfos []WeeklyTaskTimeInfo) SlackMessage {
	var messageText strings.Builder
	messageText.WriteString("📈 *Weekly Task Summary* - ")
	messageText.WriteString(time.Now().Format("January 2, 2006"))
	messageText.WriteString("\n\n")

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{
				Type: "plain_text",
				Text: "📈 Weekly Task Summary",
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

	// Limit to 10 tasks to stay within Slack's 50 block limit
	// Each task = 3 blocks (header + fields + divider), plus 3 header blocks = max 33 blocks
	maxTasks := 10
	if len(taskInfos) > maxTasks {
		taskInfos = taskInfos[:maxTasks]
		logger := NewLogger()
		logger.Infof("Limiting weekly report to %d tasks to fit within Slack limits", maxTasks)
	}

	for _, task := range taskInfos {
		taskBlock := formatWeeklyTaskBlock(task)
		blocks = append(blocks, taskBlock...)

		// Add to plain text version too
		messageText.WriteString(fmt.Sprintf("*%s*\n", task.Name))
		messageText.WriteString(fmt.Sprintf("• Start: %s\n", task.StartTime))
		messageText.WriteString(fmt.Sprintf("• This Week: %s\n", task.WeeklyTime))
		messageText.WriteString(fmt.Sprintf("• Last Week: %s\n", task.LastWeekTime))
		messageText.WriteString(fmt.Sprintf("• Days Worked: %d\n", task.DaysWorked))
		if task.EstimationInfo != "" {
			messageText.WriteString(fmt.Sprintf("• %s", task.EstimationInfo))
			if task.EstimationStatus != "" {
				messageText.WriteString(fmt.Sprintf(" (%s)", task.EstimationStatus))
			}

			// Add color indicator to plain text version
			percentage, _, err := calculateWeeklyTimeUsagePercentage(task)
			if err == nil {
				emoji, description, _ := getColorIndicator(percentage)
				messageText.WriteString(fmt.Sprintf("\n• Usage: %s %s", emoji, description))
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

// formatWeeklyTaskBlock formats a single weekly task into Slack blocks
func formatWeeklyTaskBlock(task WeeklyTaskTimeInfo) []Block {
	var fields []Field

	// Ensure all values are valid and not empty
	startTime := task.StartTime
	if startTime == "" {
		startTime = "N/A"
	}

	weeklyTime := task.WeeklyTime
	if weeklyTime == "" {
		weeklyTime = "0h 0m"
	}

	lastWeekTime := task.LastWeekTime
	if lastWeekTime == "" {
		lastWeekTime = "0h 0m"
	}

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Start Time:*\n%s", startTime),
	})

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*This Week:*\n%s", weeklyTime),
	})

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Last Week:*\n%s", lastWeekTime),
	})

	fields = append(fields, Field{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Days Worked:*\n%d", task.DaysWorked),
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
		percentage, _, err := calculateWeeklyTimeUsagePercentage(task)
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

	// Ensure task name is not empty and limit length
	taskName := task.Name
	if taskName == "" {
		taskName = "Unnamed Task"
	}
	// Limit task name length to avoid Slack formatting issues
	if len(taskName) > 100 {
		taskName = taskName[:97] + "..."
	}

	// Create the main section block
	sectionBlock := Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s*", taskName),
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

// sendNoWeeklyChangesNotification sends a brief notification when there are no weekly changes
func sendNoWeeklyChangesNotification() error {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return nil // Not an error, just not configured
	}

	message := SlackMessage{
		Text: "📊 Weekly Update: No task changes this week",
		Blocks: []Block{
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("📊 *Weekly Update* - %s\n\nNo task changes to report this week. System is running normally.",
						time.Now().Format("January 2, 2006")),
				},
			},
		},
	}

	return sendSlackMessage(message)
}

// calculateWeeklyTimeUsagePercentage calculates the percentage of estimation used based on weekly total time spent
func calculateWeeklyTimeUsagePercentage(task WeeklyTaskTimeInfo) (float64, int, error) {
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
	weeklySeconds := parseTimeToSeconds(task.WeeklyTime)

	// Convert pessimistic estimation from hours to seconds
	pessimisticSeconds := pessimistic * 3600

	// Calculate percentage
	percentage := (float64(weeklySeconds) / float64(pessimisticSeconds)) * 100

	logger.Debugf("Weekly task '%s': %d weekly seconds vs %d pessimistic seconds = %.1f%%",
		task.Name, weeklySeconds, pessimisticSeconds, percentage)

	return percentage, pessimisticSeconds, nil
}
