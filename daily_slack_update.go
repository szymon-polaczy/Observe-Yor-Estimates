package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

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
	// Note: Using shared database connection, no need to close here

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

	message := formatDailySlackMessage(taskInfos)

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

// formatDailySlackMessage formats the task information into a Slack message
func formatDailySlackMessage(taskInfos []TaskTimeInfo) SlackMessage {
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
		taskBlock := formatDailyTaskBlock(task)
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

// formatDailyTaskBlock formats a single task into Slack blocks
func formatDailyTaskBlock(task TaskTimeInfo) []Block {
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
