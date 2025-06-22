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
	logger := GetGlobalLogger()
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
	logger := GetGlobalLogger()

	// First check if the database has any tasks at all
	hasTasks, err := CheckDatabaseHasTasks()
	if err != nil {
		logger.Errorf("Failed to check if database has tasks: %v", err)
		return nil, fmt.Errorf("failed to check database tasks: %w", err)
	}

	if !hasTasks {
		logger.Info("Database is empty (no tasks found) - triggering full sync")
		
		// Trigger full sync to populate the database
		if err := FullSyncAll(); err != nil {
			logger.Errorf("Full sync failed: %v", err)
			return nil, fmt.Errorf("full sync failed after detecting empty database: %w", err)
		}
		
		logger.Info("Full sync completed successfully - retrying weekly task query")
		
		// After successful sync, try again to get the data
		return GetWeeklyTaskTimeEntries(db)
	}

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
		logger := GetGlobalLogger()
		logger.Infof("Limiting weekly report to %d tasks to fit within Slack limits", maxTasks)
	}

	for _, task := range taskInfos {
		taskBlock := formatWeeklyTaskBlock(task)
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

		messageText.WriteString(fmt.Sprintf("Time worked: This week %s, Last week %s, Days worked: %d", task.WeeklyTime, task.LastWeekTime, task.DaysWorked))
		if task.EstimationInfo != "" {
			percentage, _, err := calculateWeeklyTimeUsagePercentage(task)
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

// formatWeeklyTaskBlock formats a single weekly task into Slack blocks
func formatWeeklyTaskBlock(task WeeklyTaskTimeInfo) []Block {
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

	// Ensure task name is not empty and limit length
	taskName := task.Name
	if taskName == "" {
		taskName = "Unnamed Task"
	}
	// Limit task name length to avoid Slack formatting issues
	if len(taskName) > 100 {
		taskName = taskName[:97] + "..."
	}

	// Build compact formatting with name and estimation on one line
	var titleLine strings.Builder
	titleLine.WriteString(fmt.Sprintf("*%s*", taskName))

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
	timeLine.WriteString(fmt.Sprintf("*Time worked:* This week %s, Last week %s, Days worked: %d", weeklyTime, lastWeekTime, task.DaysWorked))

	// Add percentage if available
	if task.EstimationInfo != "" {
		percentage, _, err := calculateWeeklyTimeUsagePercentage(task)
		if err == nil {
			emoji, description, _ := getColorIndicator(percentage)
			timeLine.WriteString(fmt.Sprintf(" | *Usage:* %s %.0f%% (%s)", emoji, percentage, description))
		}
	}

	// Build the main text content
	mainText := fmt.Sprintf("%s\n%s\n*Start:* %s", titleLine.String(), timeLine.String(), startTime)

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
	logger := GetGlobalLogger()

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

// SendWeeklySlackUpdateWithResponseURL sends a weekly update to Slack using a response URL
func SendWeeklySlackUpdateWithResponseURL(responseURL string) {
	logger := GetGlobalLogger()
	logger.Info("Starting weekly Slack update with response URL")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		})
		return
	}

	taskInfos, err := getWeeklyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get weekly task changes: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to retrieve weekly task changes",
		})
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📈 No task changes to report this week",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📈 *Weekly Task Summary*\n\nNo task changes to report this week. System is working normally.",
					},
				},
			},
		}
		if err := sendDelayedResponseToURL(responseURL, message); err != nil {
			logger.Errorf("Failed to send 'no changes' response: %v", err)
		} else {
			logger.Info("Successfully sent 'no changes' weekly update via response URL")
		}
		return
	}

	message := formatWeeklySlackMessage(taskInfos)
	if err := sendDelayedResponseToURL(responseURL, message); err != nil {
		logger.Errorf("Failed to send delayed response: %v", err)
	} else {
		logger.Info("Successfully sent weekly update via response URL")
	}
}

// SendWeeklySlackUpdateJSON generates a weekly update and outputs it as JSON to stdout
func SendWeeklySlackUpdateJSON() {
	logger := GetGlobalLogger()
	logger.Info("Starting weekly Slack update JSON output")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		}
		outputJSON(errorMessage)
		return
	}

	taskInfos, err := getWeeklyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get weekly task changes: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to retrieve weekly task changes",
		}
		outputJSON(errorMessage)
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📈 No task changes to report this week",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📈 *Weekly Task Summary*\n\nNo task changes to report this week. System is working normally.",
					},
				},
			},
		}
		outputJSON(message)
		return
	}

	message := formatWeeklySlackMessage(taskInfos)
	outputJSON(message)
	logger.Info("Successfully generated weekly update JSON")
}
