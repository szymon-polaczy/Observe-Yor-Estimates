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

// SendMonthlySlackUpdate sends a monthly update to Slack with task changes over the past month
func SendMonthlySlackUpdate() {
	logger := GetGlobalLogger()
	logger.Info("Starting monthly Slack update")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		// Send a notification to user about the failure if webhook is configured
		sendFailureNotification("Database connection failed", err)
		return
	}
	// Note: Using shared database connection, no need to close here

	taskInfos, err := getMonthlyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get monthly task changes: %v", err)
		sendFailureNotification("Failed to retrieve monthly task changes", err)
		return
	}

	if len(taskInfos) == 0 {
		logger.Info("No task changes to report this month")
		// Still send a brief update to let users know the system is working
		if err := sendNoMonthlyChangesNotification(); err != nil {
			logger.Errorf("Failed to send 'no monthly changes' notification: %v", err)
		}
		return
	}

	message := formatMonthlySlackMessage(taskInfos)

	// Check if we have webhook URL configured
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warn("SLACK_WEBHOOK_URL not configured. Monthly update would contain:")
		logger.Info(strings.Repeat("=", 50))
		logger.Info(message.Text)
		logger.Info(strings.Repeat("=", 50))
		return
	}

	err = sendSlackMessage(message)
	if err != nil {
		logger.Errorf("Failed to send monthly Slack message: %v", err)
		return
	}

	logger.Info("Monthly Slack update sent successfully")
}

// getMonthlyTaskChanges retrieves task time changes for the past month
func getMonthlyTaskChanges(db *sql.DB) ([]MonthlyTaskTimeInfo, error) {
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
		
		logger.Info("Full sync completed successfully - retrying monthly task query")
		
		// After successful sync, try again to get the data
		taskInfos, err := GetMonthlyTaskTimeEntries(db)
		if err != nil {
			return nil, err
		}
		
		// If still no data after sync, create a special message about the sync
		if len(taskInfos) == 0 {
			// Create a dummy task info to indicate that sync was performed
			syncInfo := MonthlyTaskTimeInfo{
				TaskID:           0,
				Name:             "🔄 Database Sync Completed",
				MonthlyTime:      "Database was empty and has been populated with tasks from TimeCamp",
				LastMonthTime:    "",
				StartTime:        time.Now().Format("15:04"),
				EstimationInfo:   "",
				EstimationStatus: "",
				DaysWorked:       0,
				Comments:         []string{"Full synchronization completed successfully", "Tasks and time entries are now available for future reports"},
			}
			return []MonthlyTaskTimeInfo{syncInfo}, nil
		}
		
		return taskInfos, nil
	}

	// Get actual time entries data from the database for the past month
	logger.Debug("Querying tasks with monthly time entries")

	return GetMonthlyTaskTimeEntries(db)
}

// formatMonthlySlackMessage formats the monthly task information into a Slack message
func formatMonthlySlackMessage(taskInfos []MonthlyTaskTimeInfo) SlackMessage {
	var messageText strings.Builder
	messageText.WriteString("📅 *Monthly Task Summary* - ")
	messageText.WriteString(time.Now().Format("January 2006"))
	messageText.WriteString("\n\n")

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{
				Type: "plain_text",
				Text: "📅 Monthly Task Summary",
			},
		},
		{
			Type: "context",
			Elements: []Element{
				{
					Type: "mrkdwn",
					Text: time.Now().Format("*January 2006*"),
				},
			},
		},
		{
			Type: "divider",
		},
	}

	// Limit to 10 tasks to stay within Slack's 50 block limit
	// Each task = 2 blocks (section + divider), plus 3 header blocks = max 23 blocks
	maxTasks := 10
	if len(taskInfos) > maxTasks {
		taskInfos = taskInfos[:maxTasks]
		logger := GetGlobalLogger()
		logger.Infof("Limiting monthly report to %d tasks to fit within Slack limits", maxTasks)
	}

	for _, task := range taskInfos {
		taskBlock := formatMonthlyTaskBlock(task)
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

		messageText.WriteString(fmt.Sprintf("Time worked: This month %s, Last month %s, Days worked: %d", task.MonthlyTime, task.LastMonthTime, task.DaysWorked))
		if task.EstimationInfo != "" {
			percentage, _, err := calculateMonthlyTimeUsagePercentage(task)
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

// formatMonthlyTaskBlock formats a single monthly task into Slack blocks
func formatMonthlyTaskBlock(task MonthlyTaskTimeInfo) []Block {
	// Ensure all values are valid and not empty
	startTime := task.StartTime
	if startTime == "" {
		startTime = "N/A"
	}

	monthlyTime := task.MonthlyTime
	if monthlyTime == "" {
		monthlyTime = "0h 0m"
	}

	lastMonthTime := task.LastMonthTime
	if lastMonthTime == "" {
		lastMonthTime = "0h 0m"
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
	timeLine.WriteString(fmt.Sprintf("*Time worked:* This month %s, Last month %s, Days worked: %d", monthlyTime, lastMonthTime, task.DaysWorked))

	// Add percentage if available
	if task.EstimationInfo != "" {
		percentage, _, err := calculateMonthlyTimeUsagePercentage(task)
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

// sendNoMonthlyChangesNotification sends a brief notification when there are no monthly changes
func sendNoMonthlyChangesNotification() error {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return nil // Not an error, just not configured
	}

	message := SlackMessage{
		Text: "📅 Monthly Update: No task changes this month",
		Blocks: []Block{
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("📅 *Monthly Update* - %s\n\nNo task changes to report this month. System is running normally.",
						time.Now().Format("January 2006")),
				},
			},
		},
	}

	return sendSlackMessage(message)
}

// SendMonthlySlackUpdateWithResponseURL sends a monthly update to Slack using a response URL
func SendMonthlySlackUpdateWithResponseURL(responseURL string) {
	logger := GetGlobalLogger()
	logger.Info("Starting monthly Slack update with response URL")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		})
		return
	}

	taskInfos, err := getMonthlyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get monthly task changes: %v", err)
		sendDelayedResponseToURL(responseURL, SlackMessage{
			Text: "❌ Error: Failed to retrieve monthly task changes",
		})
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📅 No task changes to report this month",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📅 *Monthly Task Summary*\n\nNo task changes to report this month. System is working normally.",
					},
				},
			},
		}
		if err := sendDelayedResponseToURL(responseURL, message); err != nil {
			logger.Errorf("Failed to send 'no changes' response: %v", err)
		} else {
			logger.Info("Successfully sent 'no changes' monthly update via response URL")
		}
		return
	}

	message := formatMonthlySlackMessage(taskInfos)
	if err := sendDelayedResponseToURL(responseURL, message); err != nil {
		logger.Errorf("Failed to send delayed response: %v", err)
	} else {
		logger.Info("Successfully sent monthly update via response URL")
	}
}

// SendMonthlySlackUpdateJSON generates a monthly update and outputs it as JSON to stdout
func SendMonthlySlackUpdateJSON() {
	logger := GetGlobalLogger()
	logger.Info("Starting monthly Slack update JSON output")

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to connect to database",
		}
		outputJSON(errorMessage)
		return
	}

	taskInfos, err := getMonthlyTaskChanges(db)
	if err != nil {
		logger.Errorf("Failed to get monthly task changes: %v", err)
		errorMessage := SlackMessage{
			Text: "❌ Error: Failed to retrieve monthly task changes",
		}
		outputJSON(errorMessage)
		return
	}

	if len(taskInfos) == 0 {
		message := SlackMessage{
			Text: "📅 No task changes to report this month",
			Blocks: []Block{
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: "📅 *Monthly Task Summary*\n\nNo task changes to report this month. System is working normally.",
					},
				},
			},
		}
		outputJSON(message)
		return
	}

	message := formatMonthlySlackMessage(taskInfos)
	outputJSON(message)
	logger.Info("Successfully generated monthly update JSON")
}

// calculateMonthlyTimeUsagePercentage calculates the percentage of estimation used based on monthly total time spent
func calculateMonthlyTimeUsagePercentage(task MonthlyTaskTimeInfo) (float64, int, error) {
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
	monthlySeconds := parseTimeToSeconds(task.MonthlyTime)

	// Convert pessimistic estimation from hours to seconds
	pessimisticSeconds := pessimistic * 3600

	// Calculate percentage
	percentage := (float64(monthlySeconds) / float64(pessimisticSeconds)) * 100

	logger.Debugf("Monthly task '%s': %d monthly seconds vs %d pessimistic seconds = %.1f%%",
		task.Name, monthlySeconds, pessimisticSeconds, percentage)

	return percentage, pessimisticSeconds, nil
}
