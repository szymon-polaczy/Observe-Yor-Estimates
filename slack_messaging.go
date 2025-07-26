package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SendTaskMessage sends task updates grouped by project via webhook
func SendTaskMessage(tasks []TaskInfo, period string) error {
	if len(tasks) == 0 {
		return sendNoTasksMessage(period)
	}

	// Group by project
	projectGroups := groupTasksByProject(tasks)

	// Send message for each project
	for project, projectTasks := range projectGroups {
		message := formatProjectMessage(project, projectTasks, period)

		if err := validateAndSend(message); err != nil {
			logger := GetGlobalLogger()
			logger.Errorf("Failed to send message for project %s: %v", project, err)
			logger.Errorf("Message: %v", message)
			return err
		}
	}

	return nil
}

// formatProjectMessage creates message for a project's tasks
func formatProjectMessage(project string, tasks []TaskInfo, period string) SlackMessage {
	title := fmt.Sprintf("%s %s Report", EMOJI_CHART, strings.Title(period))

	var text strings.Builder
	text.WriteString(fmt.Sprintf("*%s*\n", title))
	text.WriteString(fmt.Sprintf("📅 %s\n\n", time.Now().Format("January 2, 2006")))

	blocks := []Block{
		{Type: "header", Text: &Text{Type: "plain_text", Text: title}},
		{Type: "context", Elements: []Element{{Type: "mrkdwn", Text: time.Now().Format("January 2, 2006")}}},
		{Type: "divider"},
	}

	// Project header
	projectTitle := fmt.Sprintf("%s %s", EMOJI_FOLDER, project)
	if project == "Other" {
		projectTitle = fmt.Sprintf("%s Other Tasks", EMOJI_CLIPBOARD)
	}

	text.WriteString(fmt.Sprintf("*%s*\n", projectTitle))
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*", projectTitle)},
	})

	// Add tasks
	for _, task := range tasks {
		taskText, taskBlock := formatTask(task)
		text.WriteString(taskText)
		blocks = append(blocks, taskBlock)
	}

	return SlackMessage{Text: text.String(), Blocks: blocks}
}

// formatThresholdMessage creates threshold-specific message
func formatThresholdMessage(project string, tasks []TaskInfo, period string, threshold float64) SlackMessage {
	status := GetThresholdStatus(threshold)
	title := fmt.Sprintf("%s Tasks Over %.0f%% Threshold", status.Emoji, threshold)

	var text strings.Builder
	text.WriteString(fmt.Sprintf("*%s*\n", title))
	text.WriteString(fmt.Sprintf("📅 Period: %s | Project: %s\n\n", period, project))

	blocks := []Block{
		{Type: "header", Text: &Text{Type: "plain_text", Text: title}},
		{Type: "context", Elements: []Element{{Type: "mrkdwn", Text: fmt.Sprintf("📅 Period: %s | Project: %s", period, project)}}},
		{Type: "divider"},
	}

	// Add tasks
	for _, task := range tasks {
		taskText, taskBlock := formatTask(task)
		text.WriteString(taskText)
		blocks = append(blocks, taskBlock)
	}

	return SlackMessage{Text: text.String(), Blocks: blocks}
}

// formatTask creates text and block for a single task
func formatTask(task TaskInfo) (string, Block) {
	var text strings.Builder
	var blockText strings.Builder

	// Task name with estimate and percentage
	taskName := sanitizeText(task.Name)
	text.WriteString(fmt.Sprintf("├─ %s", taskName))
	blockText.WriteString(fmt.Sprintf("*%s*", taskName))

	// Add estimation with percentage if available
	if task.EstimationInfo.Text != "" {
		estimationText := sanitizeText(task.EstimationInfo.Text)

		// Calculate percentage if we have estimation info
		var percentageText string
		if task.EstimationInfo.Percentage > 0 {
			percentage := task.EstimationInfo.Percentage
			emoji := EMOJI_ON_TRACK
			if percentage >= 100 {
				emoji = EMOJI_OVER_BUDGET
			} else if percentage >= 80 {
				emoji = EMOJI_WARNING
			}
			percentageText = fmt.Sprintf(" - %.0f%% used %s", percentage, emoji)
		}

		text.WriteString(fmt.Sprintf(" [%s%s]\n", estimationText, percentageText))
		blockText.WriteString(fmt.Sprintf(" [%s%s]\n", estimationText, percentageText))
	} else {
		text.WriteString("\n")
		blockText.WriteString("\n")
	}

	// Time information
	timeInfo := fmt.Sprintf("│  %s: %s | %s: %s",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime)

	text.WriteString(timeInfo + "\n")
	blockText.WriteString(fmt.Sprintf("• %s: *%s* | %s: *%s*\n",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime))

	// Comments (limit to 3 for brevity)
	if len(task.Comments) > 0 {
		comments := removeEmptyStrings(task.Comments)
		limit := 3
		if len(comments) > limit {
			comments = comments[:limit]
		}

		for i, comment := range comments {
			comment = sanitizeText(comment)
			if len(comment) > 100 {
				comment = comment[:97] + "..."
			}
			text.WriteString(fmt.Sprintf("│  %d. %s\n", i+1, comment))
			blockText.WriteString(fmt.Sprintf("  %d. %s\n", i+1, comment))
		}

		if len(task.Comments) > limit {
			remaining := len(task.Comments) - limit
			text.WriteString(fmt.Sprintf("│  ... and %d more\n", remaining))
			blockText.WriteString(fmt.Sprintf("  ... and %d more\n", remaining))
		}
	}

	text.WriteString("└─\n")

	return text.String(), Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: blockText.String()},
	}
}

// groupTasksByProject groups tasks by project name
func groupTasksByProject(tasks []TaskInfo) map[string][]TaskInfo {
	db, err := GetDB()
	if err != nil {
		return map[string][]TaskInfo{"Other": tasks}
	}

	allTasks, err := getAllTasks(db)
	if err != nil {
		return map[string][]TaskInfo{"Other": tasks}
	}

	groups := make(map[string][]TaskInfo)
	for _, task := range tasks {
		project := getProjectNameForTask(task.TaskID, allTasks)
		if project == "" {
			project = "Other"
		}
		groups[project] = append(groups[project], task)
	}

	return groups
}

// getProjectNameForTask finds project name for a task
func getProjectNameForTask(taskID int, allTasks map[int]Task) string {
	currentID := taskID
	var previousName string

	for i := 0; i < 10; i++ { // max depth
		task, ok := allTasks[currentID]
		if !ok {
			return previousName
		}

		if task.ParentID == 0 {
			return previousName
		}

		previousName = task.Name
		currentID = task.ParentID
	}

	return previousName
}

// getAllTasks retrieves all tasks from database
func getAllTasks(db *sql.DB) (map[int]Task, error) {
	rows, err := db.Query("SELECT task_id, parent_id, name FROM tasks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make(map[int]Task)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ParentID, &task.Name); err == nil {
			tasks[task.ID] = task
		}
	}

	return tasks, nil
}

// formatNoTasksMessage creates message when no tasks found
func formatNoTasksMessage(period string) SlackMessage {
	message := fmt.Sprintf("%s No time entries found for %s period %s",
		EMOJI_CHART, period, EMOJI_CELEBRATION)

	return SlackMessage{
		Text: message,
		Blocks: []Block{{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: message},
		}},
	}
}

// sendNoTasksMessage sends message when no tasks found
func sendNoTasksMessage(period string) error {
	slackMsg := formatNoTasksMessage(period)
	return validateAndSend(slackMsg)
}

// validateAndSend validates message limits and sends
func validateAndSend(message SlackMessage) error {
	validation := validateMessageLimits(message)
	if !validation.IsValid {
		// Split message if too large
		if validation.ExceedsBlocks {
			return splitAndSendMessage(message)
		}
		// Log warning but still try to send
		logger := GetGlobalLogger()
		logger.Warnf("Message may exceed limits: %s", validation.ErrorMessage)
	}

	return sendSlackWebhook(message)
}

// validateMessageLimits checks Slack message limits
func validateMessageLimits(message SlackMessage) MessageValidation {
	blockCount := len(message.Blocks)

	messageBytes, err := json.Marshal(message)
	charCount := len(messageBytes)
	if err != nil {
		charCount = len(message.Text)
	}

	exceedsBlocks := blockCount > MAX_SLACK_BLOCKS
	exceedsChars := charCount > MAX_SLACK_MESSAGE_CHARS
	isValid := !exceedsBlocks && !exceedsChars

	var errorMsg string
	if exceedsBlocks && exceedsChars {
		errorMsg = fmt.Sprintf("Exceeds block limit (%d>%d) and char limit (%d>%d)",
			blockCount, MAX_SLACK_BLOCKS, charCount, MAX_SLACK_MESSAGE_CHARS)
	} else if exceedsBlocks {
		errorMsg = fmt.Sprintf("Exceeds block limit (%d>%d)", blockCount, MAX_SLACK_BLOCKS)
	} else if exceedsChars {
		errorMsg = fmt.Sprintf("Exceeds char limit (%d>%d)", charCount, MAX_SLACK_MESSAGE_CHARS)
	}

	return MessageValidation{
		IsValid:        isValid,
		BlockCount:     blockCount,
		CharacterCount: charCount,
		ExceedsBlocks:  exceedsBlocks,
		ExceedsChars:   exceedsChars,
		ErrorMessage:   errorMsg,
	}
}

// splitAndSendMessage splits large messages and sends them
func splitAndSendMessage(message SlackMessage) error {
	maxBlocks := MAX_SLACK_BLOCKS - 2 // reserve for header/footer

	if len(message.Blocks) <= maxBlocks {
		return sendSlackWebhook(message)
	}

	// Keep header blocks
	headerBlocks := []Block{}
	taskBlocks := []Block{}

	for i, block := range message.Blocks {
		if i < 3 { // header, context, divider
			headerBlocks = append(headerBlocks, block)
		} else {
			taskBlocks = append(taskBlocks, block)
		}
	}

	// Send in chunks
	for i := 0; i < len(taskBlocks); i += maxBlocks {
		end := i + maxBlocks
		if end > len(taskBlocks) {
			end = len(taskBlocks)
		}

		chunkBlocks := append(headerBlocks, taskBlocks[i:end]...)

		chunkMessage := SlackMessage{
			Text:   message.Text,
			Blocks: chunkBlocks,
		}

		if err := sendSlackWebhook(chunkMessage); err != nil {
			return err
		}
	}

	return nil
}

// sendSlackWebhook sends message via webhook
func sendSlackWebhook(message SlackMessage) error {
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendSlackResponse sends response via response URL
func sendSlackResponse(responseURL string, message SlackMessage) error {
	if responseURL == "" {
		return nil
	}

	response := map[string]interface{}{
		"response_type": "in_channel",
		"text":          message.Text,
	}

	if len(message.Blocks) > 0 {
		response["blocks"] = message.Blocks
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("response URL returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Legacy compatibility functions

// getTaskChanges unified dynamic function
func getTaskChanges(db *sql.DB, period string, days int) ([]TaskUpdateInfo, error) {
	return GetDynamicTaskTimeEntriesWithProject(db, period, days, nil)
}

// Utility functions

func sanitizeText(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\"", "'")
	text = strings.TrimSpace(text)

	if len(text) > 2000 {
		text = text[:1997] + "..."
	}

	return text
}

func removeEmptyStrings(strs []string) []string {
	var result []string
	for _, str := range strs {
		if strings.TrimSpace(str) != "" {
			result = append(result, str)
		}
	}
	return result
}
