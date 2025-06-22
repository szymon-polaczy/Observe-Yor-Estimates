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
	logger := GetGlobalLogger()
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
	logger := GetGlobalLogger()

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

		// Add to plain text version too - compact format
		messageText.WriteString(fmt.Sprintf("*%s*", task.Name))
		if task.EstimationInfo != "" {
			estimationText := task.EstimationInfo
			if task.EstimationStatus != "" {
				estimationText += fmt.Sprintf(" (%s)", task.EstimationStatus)
			}
			messageText.WriteString(fmt.Sprintf(" | %s", estimationText))
		} else {
			messageText.WriteString(" | _no estimation given_")
		}
		messageText.WriteString("\n")

		messageText.WriteString(fmt.Sprintf("Time worked: Yesterday %s, Today %s", task.YesterdayTime, task.TodayTime))
		if task.EstimationInfo != "" {
			percentage, _, err := calculateTimeUsagePercentage(task)
			if err == nil {
				emoji, description, _ := getColorIndicator(percentage)
				messageText.WriteString(fmt.Sprintf(" | Usage: %s %.0f%% (%s)", emoji, percentage, description))
			}
		}
		messageText.WriteString(fmt.Sprintf("\nStart: %s", task.StartTime))

		// Add comments to plain text version
		if len(task.Comments) > 0 {
			messageText.WriteString("\nComments:")
			for _, comment := range task.Comments {
				messageText.WriteString(fmt.Sprintf("\n• %s", comment))
			}
		}
		messageText.WriteString("\n\n")
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// formatDailyTaskBlock formats a single task into Slack blocks
func formatDailyTaskBlock(task TaskTimeInfo) []Block {
	// Build compact formatting with name and estimation on one line
	var titleLine strings.Builder
	titleLine.WriteString(fmt.Sprintf("*%s*", task.Name))

	// Add estimation info to the same line if available
	if task.EstimationInfo != "" {
		estimationText := task.EstimationInfo
		if task.EstimationStatus != "" {
			estimationText += fmt.Sprintf(" (%s)", task.EstimationStatus)
		}
		titleLine.WriteString(fmt.Sprintf(" | %s", estimationText))
	} else {
		titleLine.WriteString(" | _no estimation given_")
	}

	// Build time and percentage line
	var timeLine strings.Builder
	timeLine.WriteString(fmt.Sprintf("*Time worked:* Yesterday %s, Today %s", task.YesterdayTime, task.TodayTime))

	// Add percentage if available
	if task.EstimationInfo != "" {
		percentage, _, err := calculateTimeUsagePercentage(task)
		if err == nil {
			emoji, description, _ := getColorIndicator(percentage)
			timeLine.WriteString(fmt.Sprintf(" | *Usage:* %s %.0f%% (%s)", emoji, percentage, description))
		}
	}

	// Build the main text content
	mainText := fmt.Sprintf("%s\n%s\n*Start:* %s", titleLine.String(), timeLine.String(), task.StartTime)

	// Add comments if available
	if len(task.Comments) > 0 {
		mainText += "\n*Comments:*"
		for _, comment := range task.Comments {
			mainText += fmt.Sprintf("\n• %s", comment)
		}
	}

	// Create a single compact section block
	sectionBlock := Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: mainText,
		},
	}

	return []Block{
		sectionBlock,
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

// SendDailySlackUpdateWithResponseURL sends a daily update to Slack using a response URL
func SendDailySlackUpdateWithResponseURL(responseURL string) {
	logger := GetGlobalLogger()
	logger.Info("Starting daily Slack update with response URL")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		})
		return
	}

	taskInfos, err := getTaskTimeChanges(db)
	if err != nil {
		logger.Errorf("Failed to get task time changes: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to retrieve task changes",
		})
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📊 No task changes to report today",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📊 *Daily Task Update*\n\nNo task changes to report today. System is working normally.",
					},
				},
			},
		}
		if err := sendDelayedResponseToURL(responseURL, message); err != nil {
			logger.Errorf("Failed to send 'no changes' response: %v", err)
		} else {
			logger.Info("Successfully sent 'no changes' daily update via response URL")
		}
		return
	}

	message := formatDailySlackMessage(taskInfos)
	if err := sendDelayedResponseToURL(responseURL, message); err != nil {
		logger.Errorf("Failed to send delayed response: %v", err)
	} else {
		logger.Info("Successfully sent daily update via response URL")
	}
}

// SendDailySlackUpdateJSON generates a daily update and outputs it as JSON to stdout
func SendDailySlackUpdateJSON() {
	logger := GetGlobalLogger()
	logger.Info("Starting daily Slack update JSON output")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		}
		outputJSON(errorMessage)
		return
	}

	taskInfos, err := getTaskTimeChanges(db)
	if err != nil {
		logger.Errorf("Failed to get task time changes: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to retrieve task changes",
		}
		outputJSON(errorMessage)
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📊 No task changes to report today",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📊 *Daily Task Update*\n\nNo task changes to report today. System is working normally.",
					},
				},
			},
		}
		outputJSON(message)
		return
	}

	message := formatDailySlackMessage(taskInfos)
	outputJSON(message)
	logger.Info("Successfully generated daily update JSON")
}
