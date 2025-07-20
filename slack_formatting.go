package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatTaskMessage creates a Slack message with configurable options
func FormatTaskMessage(tasks []TaskInfo, period string, options FormatOptions) SlackMessage {
	if len(tasks) == 0 {
		return formatNoTasksMessage(period, options)
	}

	if options.Threshold != nil {
		return formatThresholdMessage(tasks, period, *options.Threshold, options)
	}

	return formatStandardMessage(tasks, period, options)
}

// formatStandardMessage creates standard task update messages
func formatStandardMessage(tasks []TaskInfo, period string, options FormatOptions) SlackMessage {
	title := buildTitle(period, options)
	
	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n\n", title))

	blocks := []Block{}
	
	if options.ShowHeader {
		blocks = append(blocks, buildHeaderBlocks(title, period)...)
	}

	// Add task blocks
	for _, task := range tasks {
		if options.MaxTasks > 0 && len(blocks) >= options.MaxTasks {
			break
		}
		taskBlock := buildTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskToText(&messageText, task)
	}

	if options.ShowFooter {
		blocks = append(blocks, buildFooterBlock())
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// formatThresholdMessage creates threshold-specific messages
func formatThresholdMessage(tasks []TaskInfo, period string, threshold float64, options FormatOptions) SlackMessage {
	status := GetThresholdStatus(threshold)
	title := fmt.Sprintf(TEMPLATE_THRESHOLD_TITLE, status.Emoji, status.Description, threshold)
	
	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n", title))
	messageText.WriteString(fmt.Sprintf("📅 Period: %s | Found %d tasks\n\n", strings.Title(period), len(tasks)))

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: fmt.Sprintf("📅 Period: %s | Found %d tasks", strings.Title(period), len(tasks))},
			},
		},
		{Type: "divider"},
	}

	// Add task blocks
	for _, task := range tasks {
		taskBlock := buildTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskToText(&messageText, task)
	}

	// Add suggestion
	suggestion := GetSuggestionForThreshold(threshold)
	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Element{
			{Type: "mrkdwn", Text: suggestion},
		},
	})

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// formatNoTasksMessage handles empty task lists
func formatNoTasksMessage(period string, options FormatOptions) SlackMessage {
	var message string
	
	if options.Threshold != nil {
		message = fmt.Sprintf("%s No tasks found over %.0f%% threshold for %s period", 
			EMOJI_TARGET, *options.Threshold, period)
	} else {
		message = fmt.Sprintf(TEMPLATE_NO_CHANGES, EMOJI_CHART, period, EMOJI_CELEBRATION)
	}

	return SlackMessage{
		Text: message,
		Blocks: []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: message},
			},
		},
	}
}

// buildTitle creates appropriate title based on context
func buildTitle(period string, options FormatOptions) string {
	if options.IsPersonal {
		if options.InThread {
			return fmt.Sprintf(TEMPLATE_PERSONAL_UPDATE+" (Personal Report)", EMOJI_CHART, period)
		}
		return fmt.Sprintf(TEMPLATE_PERSONAL_UPDATE, EMOJI_CHART, period)
	}
	return fmt.Sprintf(TEMPLATE_TASK_UPDATE, EMOJI_CHART, strings.Title(period))
}

// buildHeaderBlocks creates header blocks for messages
func buildHeaderBlocks(title, period string) []Block {
	return []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: " "}, // spacing
		},
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: time.Now().Format("January 2, 2006")},
			},
		},
		{Type: "divider"},
	}
}

// buildTaskBlock creates a block for a single task
func buildTaskBlock(task TaskInfo) Block {
	taskName := sanitizeText(task.Name)

	var taskInfo strings.Builder
	taskInfo.WriteString(fmt.Sprintf("*%s*\n", taskName))

	// Time information
	timeInfo := fmt.Sprintf("• %s: *%s* | %s: *%s*",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime)
	taskInfo.WriteString(timeInfo + "\n")

	// Estimation info
	if task.EstimationInfo.Text != "" {
		estimationText := sanitizeText(task.EstimationInfo.Text)
		taskInfo.WriteString(fmt.Sprintf("• %s\n", estimationText))
	}

	// Comments
	task.Comments = removeEmptyComments(task.Comments)
	if len(task.Comments) > 0 {
		taskInfo.WriteString("• Comments:\n")
		for i, comment := range task.Comments {
			if comment == "" {
				continue
			}
			comment = sanitizeText(comment)
			// Limit comment length to prevent huge blocks
			if len(comment) > 200 {
				comment = comment[:197] + "..."
			}
			taskInfo.WriteString(fmt.Sprintf("  %d. %s\n", i+1, comment))
			
			// Limit total comments to prevent message overflow
			if i >= 10 {
				remaining := len(task.Comments) - i - 1
				if remaining > 0 {
					taskInfo.WriteString(fmt.Sprintf("  ... and %d more comments\n", remaining))
				}
				break
			}
		}
	}

	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: taskInfo.String()},
	}
}

// buildFooterBlock creates footer spacing
func buildFooterBlock() Block {
	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "}, // spacing
	}
}

// appendTaskToText adds task info to plain text message
func appendTaskToText(builder *strings.Builder, task TaskInfo) {
	builder.WriteString(fmt.Sprintf("*%s*", task.Name))
	if task.EstimationInfo.Text != "" {
		builder.WriteString(fmt.Sprintf(" | %s", task.EstimationInfo.Text))
	}

	timeText := fmt.Sprintf("\nTime worked: %s: %s, %s: %s", 
		task.CurrentPeriod, task.CurrentTime, 
		task.PreviousPeriod, task.PreviousTime)

	builder.WriteString(timeText + "\n\n")
}

// GroupTasksByProject groups tasks by their top-level project
func GroupTasksByProject(tasks []TaskInfo, allTasks map[int]Task) map[string][]TaskInfo {
	projects := make(map[string][]TaskInfo)

	for _, task := range tasks {
		projectName := getProjectNameForTask(task.TaskID, allTasks)
		projects[projectName] = append(projects[projectName], task)
	}

	return projects
}

// SplitMessagesByProject creates separate messages for each project
func SplitMessagesByProject(tasks []TaskInfo, period string, options FormatOptions) []SlackMessage {
	db, err := GetDB()
	if err != nil {
		// Fallback to single message
		return []SlackMessage{FormatTaskMessage(tasks, period, options)}
	}

	allTasks, err := getAllTasks(db)
	if err != nil {
		// Fallback to single message
		return []SlackMessage{FormatTaskMessage(tasks, period, options)}
	}

	projectGroups := GroupTasksByProject(tasks, allTasks)
	
	// Sort project names
	var projectNames []string
	for project := range projectGroups {
		projectNames = append(projectNames, project)
	}
	sort.Slice(projectNames, func(i, j int) bool {
		if projectNames[i] == "Other" {
			return false
		}
		if projectNames[j] == "Other" {
			return true
		}
		return projectNames[i] < projectNames[j]
	})

	var messages []SlackMessage
	for i, project := range projectNames {
		projectTasks := projectGroups[project]
		
		// Create project-specific options
		projectOptions := options
		projectOptions.ShowHeader = true
		
		// Modify title for project context
		if project == "Other" {
			// Keep original title structure for "Other"
			messages = append(messages, FormatTaskMessage(projectTasks, period, projectOptions))
		} else {
			// Add project context to title
			projectPeriod := fmt.Sprintf("%s - %s Project (%d/%d)", period, project, i+1, len(projectNames))
			messages = append(messages, FormatTaskMessage(projectTasks, projectPeriod, projectOptions))
		}
	}

	return messages
}

// ValidateMessage checks if message meets Slack limits
func ValidateMessage(message SlackMessage) MessageValidation {
	blockCount := len(message.Blocks)
	
	// Calculate character count of entire payload
	messageBytes, err := json.Marshal(message)
	charCount := len(string(messageBytes))
	if err != nil {
		charCount = len(message.Text) // fallback estimate
	}

	exceedsBlocks := blockCount > MAX_SLACK_BLOCKS
	exceedsChars := charCount > MAX_SLACK_MESSAGE_CHARS
	isValid := !exceedsBlocks && !exceedsChars

	var errorMsg string
	if exceedsBlocks && exceedsChars {
		errorMsg = fmt.Sprintf("Message exceeds both block limit (%d > %d) and character limit (%d > %d)",
			blockCount, MAX_SLACK_BLOCKS, charCount, MAX_SLACK_MESSAGE_CHARS)
	} else if exceedsBlocks {
		errorMsg = fmt.Sprintf("Message exceeds block limit (%d > %d)", blockCount, MAX_SLACK_BLOCKS)
	} else if exceedsChars {
		errorMsg = fmt.Sprintf("Message exceeds character limit (%d > %d)", charCount, MAX_SLACK_MESSAGE_CHARS)
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

// Utility functions

func sanitizeText(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\"", "'")
	text = strings.ReplaceAll(text, "\\", "/")
	text = strings.TrimSpace(text)
	
	if len(text) > 3000 {
		text = text[:2997] + "..."
	}
	
	return text
}

func removeEmptyComments(comments []string) []string {
	var filtered []string
	for _, comment := range comments {
		if comment != "" {
			filtered = append(filtered, comment)
		}
	}
	return filtered
}

func getProjectNameForTask(taskID int, allTasks map[int]Task) string {
	const maxDepth = 10
	currentID := taskID
	var previousID = taskID

	for i := 0; i < maxDepth; i++ {
		task, ok := allTasks[currentID]
		if !ok {
			if projectTask, ok := allTasks[previousID]; ok {
				return projectTask.Name
			}
			return "Unknown Project (Orphan Task)"
		}

		if task.ParentID == 0 {
			projectTask, ok := allTasks[previousID]
			if !ok {
				return "Unknown Project (Hierarchy Issue)"
			}
			return projectTask.Name
		}

		previousID = currentID
		currentID = task.ParentID
	}

	return "Unknown Project (Max Recursion)"
}