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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type TaskUpdateInfo struct {
	TaskID           int
	ParentID         int
	Name             string
	EstimationInfo   string
	EstimationStatus string
	CurrentPeriod    string
	CurrentTime      string
	PreviousPeriod   string
	PreviousTime     string
	DaysWorked       int
	Comments         []string
	// User breakdown: map of user_id to time contributions
	UserBreakdown map[int]UserTimeContribution
}

// UserTimeContribution represents time contributed by a specific user
type UserTimeContribution struct {
	UserID       int
	CurrentTime  string
	PreviousTime string
}

// SlackMessage represents the structure of a Slack message
type SlackMessage struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

// Block represents a block in Slack blocks
type Block struct {
	Type      string                 `json:"type"`
	Text      *Text                  `json:"text,omitempty"`
	Fields    []Field                `json:"fields,omitempty"`
	Elements  []Element              `json:"elements,omitempty"`
	Accessory *Accessory             `json:"accessory,omitempty"`
	BlockID   string                 `json:"block_id,omitempty"`
	Label     *Text                  `json:"label,omitempty"`
	Element   map[string]interface{} `json:"element,omitempty"`
}

// Text represents text in Slack blocks
type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Field represents a field in Slack blocks
type Field struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Element represents an element in Slack blocks
type Element struct {
	Type     string      `json:"type"`
	Text     interface{} `json:"text,omitempty"`
	ActionID string      `json:"action_id,omitempty"`
	Style    string      `json:"style,omitempty"`
	Value    string      `json:"value,omitempty"`
}

// Accessory represents an accessory in Slack blocks
type Accessory struct {
	Type           string                   `json:"type"`
	Text           *Text                    `json:"text,omitempty"`
	ActionID       string                   `json:"action_id,omitempty"`
	Options        []map[string]interface{} `json:"options,omitempty"`
	InitialOptions []map[string]interface{} `json:"initial_options,omitempty"`
}

// ThresholdAlert represents a task that has crossed a usage threshold
type ThresholdAlert struct {
	TaskID           int
	ParentID         int
	Name             string
	EstimationInfo   string
	CurrentTime      string
	PreviousTime     string
	Percentage       float64
	ThresholdCrossed int  // 50, 80, 90, or 100
	JustCrossed      bool // true if this threshold was just crossed
}

// Constants for Slack API limits
const (
	MaxSlackBlocks            = 50
	MaxSlackMessageChars      = 3000
	MaxBlocksPerMessage       = 47   // Leave buffer for header/footer blocks
	MaxMessageCharsWithBuffer = 2900 // Leave buffer for safety margin
)

// MessageValidationResult holds validation results
type MessageValidationResult struct {
	IsValid        bool
	BlockCount     int
	CharacterCount int
	ExceedsBlocks  bool
	ExceedsChars   bool
	ErrorMessage   string
}

// validateSlackMessage checks if a message meets Slack API limits
func validateSlackMessage(message SlackMessage) MessageValidationResult {
	blockCount := len(message.Blocks)

	// Calculate total character count of the entire message payload
	// This is what Slack actually limits
	messageJson, err := json.Marshal(message)
	var charCount int
	if err != nil {
		// Fallback: estimate character count
		charCount = len(message.Text)
		if blockCount > 0 {
			blockJson, err := json.Marshal(message.Blocks)
			if err == nil {
				charCount += len(string(blockJson))
			}
		}
	} else {
		charCount = len(string(messageJson))
	}

	exceedsBlocks := blockCount > MaxSlackBlocks
	exceedsChars := charCount > MaxSlackMessageChars
	isValid := !exceedsBlocks && !exceedsChars

	var errorMsg string
	if exceedsBlocks && exceedsChars {
		errorMsg = fmt.Sprintf("Message exceeds both block limit (%d > %d) and character limit (%d > %d)",
			blockCount, MaxSlackBlocks, charCount, MaxSlackMessageChars)
	} else if exceedsBlocks {
		errorMsg = fmt.Sprintf("Message exceeds block limit (%d > %d)", blockCount, MaxSlackBlocks)
	} else if exceedsChars {
		errorMsg = fmt.Sprintf("Message exceeds character limit (%d > %d)", charCount, MaxSlackMessageChars)
	}

	return MessageValidationResult{
		IsValid:        isValid,
		BlockCount:     blockCount,
		CharacterCount: charCount,
		ExceedsBlocks:  exceedsBlocks,
		ExceedsChars:   exceedsChars,
		ErrorMessage:   errorMsg,
	}
}

// countBlocksInTasks estimates how many blocks will be needed for a list of tasks
func countBlocksInTasks(tasks []TaskUpdateInfo) int {
	// Each task typically creates 1 block, but may create more if it has many comments
	blockCount := 0
	for _, task := range tasks {
		blockCount++ // Base task block

		// Additional blocks for comments if they're very long
		task.Comments = removeEmptyComments(task.Comments)
		if len(task.Comments) > 10 {
			// Estimate additional blocks for comment overflow
			blockCount += (len(task.Comments) - 10) / 15 // Rough estimate
		}
	}
	return blockCount
}

// splitTasksByBlockLimit splits tasks into chunks that respect both block and character limits
func splitTasksByBlockLimit(tasks []TaskUpdateInfo, headerBlocks int) [][]TaskUpdateInfo {
	var chunks [][]TaskUpdateInfo
	var currentChunk []TaskUpdateInfo
	currentBlockCount := headerBlocks // Start with header blocks

	for _, task := range tasks {
		taskBlocks := 1 // Base assumption: 1 block per task

		// Check if adding this task would exceed the block limit
		if currentBlockCount+taskBlocks+1 > MaxBlocksPerMessage { // +1 for footer
			// Start a new chunk
			if len(currentChunk) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = []TaskUpdateInfo{}
				currentBlockCount = headerBlocks
			}
		}

		// Add the task to current chunk
		newChunk := append(currentChunk, task)

		// Create a test message to check character limits
		testMessage := formatProjectMessage("Test", newChunk, "test")
		validation := validateSlackMessage(testMessage)

		// If adding this task would exceed character limits (with buffer), start a new chunk
		if validation.CharacterCount > MaxMessageCharsWithBuffer && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = []TaskUpdateInfo{task}
			currentBlockCount = headerBlocks + taskBlocks

			// Check if even a single task exceeds limits
			singleTaskMessage := formatProjectMessage("Test", []TaskUpdateInfo{task}, "test")
			singleTaskValidation := validateSlackMessage(singleTaskMessage)
			if singleTaskValidation.CharacterCount > MaxSlackMessageChars {
				// This single task is too large - we need to truncate its comments
				logger := GetGlobalLogger()
				logger.Warnf("Task '%s' exceeds character limit even alone (%d chars), truncating comments",
					task.Name, singleTaskValidation.CharacterCount)

				// Truncate comments to make it fit
				truncatedTask := task
				truncatedTask.Comments = truncateCommentsToFit(task, MaxSlackMessageChars-500) // Leave buffer for headers

				// Verify the truncated task now fits
				verifyMessage := formatProjectMessage("Test", []TaskUpdateInfo{truncatedTask}, "test")
				verifyValidation := validateSlackMessage(verifyMessage)
				if verifyValidation.CharacterCount > MaxSlackMessageChars {
					logger.Errorf("Task still exceeds limit even after comment truncation: %d chars", verifyValidation.CharacterCount)
					// As last resort, remove all comments
					truncatedTask.Comments = []string{}
				}

				currentChunk = []TaskUpdateInfo{truncatedTask}
			}
		} else {
			currentChunk = newChunk
			currentBlockCount += taskBlocks
		}
	}

	// Add the last chunk if it has tasks
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	// If no chunks were created (empty task list), return empty slice
	if len(chunks) == 0 {
		chunks = append(chunks, []TaskUpdateInfo{})
	}

	return chunks
}

// truncateCommentsToFit reduces comments until the task fits within character limits
func truncateCommentsToFit(task TaskUpdateInfo, maxChars int) []string {
	if len(task.Comments) == 0 {
		return task.Comments
	}

	// Try reducing comments one by one until it fits
	for commentsToKeep := len(task.Comments); commentsToKeep >= 0; commentsToKeep-- {
		testTask := task
		if commentsToKeep == 0 {
			testTask.Comments = []string{}
		} else {
			testTask.Comments = task.Comments[:commentsToKeep]
			// Add truncation notice if we removed comments
			if commentsToKeep < len(task.Comments) {
				remaining := len(task.Comments) - commentsToKeep
				testTask.Comments = append(testTask.Comments,
					fmt.Sprintf("... and %d more comments (truncated due to size limits)", remaining))
			}
		}

		// Test if this fits using the actual message format that will be used
		testMessage := formatProjectMessage("Test", []TaskUpdateInfo{testTask}, "test")
		validation := validateSlackMessage(testMessage)

		if validation.CharacterCount <= maxChars {
			return testTask.Comments
		}
	}

	// If even with no comments it doesn't fit, we need to truncate the comment text itself
	logger := GetGlobalLogger()
	logger.Warnf("Task still too large even with no comments, will truncate individual comment text")

	// Try with very short comments
	if len(task.Comments) > 0 {
		shortComments := []string{"[Comments truncated - too large to display]"}
		testTask := task
		testTask.Comments = shortComments

		testMessage := formatProjectMessage("Test", []TaskUpdateInfo{testTask}, "test")
		validation := validateSlackMessage(testMessage)

		if validation.CharacterCount <= maxChars {
			return shortComments
		}
	}

	// If even with minimal comments it doesn't fit, return empty comments
	return []string{}
}

func SendSlackUpdate(period string, responseURL string, asJSON bool) {
	logger := GetGlobalLogger()
	logger.Infof("Starting %s Slack update", period)

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		sendFailureNotification("Database connection failed", err)
		return
	}

	taskInfos, err := getTaskChanges(db, period)
	if err != nil {
		logger.Errorf("Failed to get %s task changes: %v", period, err)
		sendFailureNotification("Failed to retrieve task changes", err)
		return
	}

	if len(taskInfos) == 0 {
		logger.Infof("No task changes to report for %s", period)
		if err := sendNoChangesNotification(period, responseURL, asJSON); err != nil {
			logger.Errorf("Failed to send 'no changes' notification: %v", err)
		}
		return
	}

	// Fetch all tasks for hierarchy mapping
	allTasks, err := getAllTasks(db)
	if err != nil {
		logger.Errorf("Failed to get all tasks for hierarchy mapping: %v", err)
		sendFailureNotification("Failed to retrieve task hierarchy", err)
		return
	}

	projectGroups := groupTasksByTopParent(taskInfos, allTasks)

	// Sort project names for consistent output
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
	for _, project := range projectNames {
		tasks := projectGroups[project]
		projectMessages := formatProjectMessageWithComments(project, tasks, period)
		messages = append(messages, projectMessages...)
	}

	if asJSON {
		outputJSON(messages)
		return
	}

	if responseURL != "" {
		for _, message := range messages {
			if err := sendDelayedResponseShared(responseURL, message); err != nil {
				logger.Errorf("Failed to send delayed response: %v", err)
			}
			time.Sleep(1 * time.Second) // Increased spacing between project messages
		}
		return
	}

	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warn("SLACK_WEBHOOK_URL not configured. Updates would contain:")
		for _, message := range messages {
			logger.Info(strings.Repeat("=", 50))
			logger.Info(message.Text)
			logger.Info(strings.Repeat("=", 50))
		}
		return
	}

	for _, message := range messages {
		err = sendSlackMessage(message)
		if err != nil {
			logger.Errorf("Failed to send Slack message: %v", err)
			// Continue sending other messages
		}
		time.Sleep(1500 * time.Millisecond) // Increased delay for better visual separation between projects
	}

	logger.Infof("%s Slack update sent successfully", period)
}

func getTaskChanges(db *sql.DB, period string) ([]TaskUpdateInfo, error) {
	// Handle backwards compatibility for old period names and new natural language periods
	switch period {
	case "daily":
		return GetDynamicTaskTimeEntriesWithProject(db, "yesterday", 1, nil)
	case "weekly":
		return GetWeeklyTaskTimeEntries(db)
	case "monthly":
		return GetMonthlyTaskTimeEntries(db)
	case "yesterday":
		return GetDynamicTaskTimeEntriesWithProject(db, "yesterday", 1, nil)
	case "today":
		return GetDynamicTaskTimeEntriesWithProject(db, "today", 0, nil)
	case "last_week":
		return GetDynamicTaskTimeEntriesWithProject(db, "last_week", 7, nil)
	case "this_week":
		return GetDynamicTaskTimeEntriesWithProject(db, "this_week", 0, nil)
	case "last_month":
		return GetDynamicTaskTimeEntriesWithProject(db, "last_month", 30, nil)
	case "this_month":
		return GetDynamicTaskTimeEntriesWithProject(db, "this_month", 0, nil)
	case "last_x_days":
		// This shouldn't happen as last_x_days should be handled with specific days
		return GetDynamicTaskTimeEntriesWithProject(db, "last_x_days", 7, nil)
	default:
		// Check if it's a last_x_days pattern with specific days
		if strings.HasPrefix(period, "last_") && strings.HasSuffix(period, "_days") {
			// Extract days from pattern like "last_7_days"
			daysPart := strings.TrimPrefix(period, "last_")
			daysPart = strings.TrimSuffix(daysPart, "_days")
			if days, err := strconv.Atoi(daysPart); err == nil && days >= 1 && days <= 60 {
				return GetDynamicTaskTimeEntriesWithProject(db, "last_x_days", days, nil)
			}
		}
		return nil, fmt.Errorf("invalid period: %s", period)
	}
}

func getTaskChangesWithProject(db *sql.DB, period string, projectTaskID *int) ([]TaskUpdateInfo, error) {
	// Handle backwards compatibility for old period names
	switch period {
	case "daily":
		return GetTaskTimeEntriesWithProject(db, projectTaskID)
	case "weekly":
		return GetWeeklyTaskTimeEntriesWithProject(db, projectTaskID)
	case "monthly":
		return GetMonthlyTaskTimeEntriesWithProject(db, projectTaskID)
	case "yesterday":
		return GetDynamicTaskTimeEntriesWithProject(db, "yesterday", 1, projectTaskID)
	case "today":
		return GetDynamicTaskTimeEntriesWithProject(db, "today", 0, projectTaskID)
	case "last_week":
		return GetDynamicTaskTimeEntriesWithProject(db, "last_week", 7, projectTaskID)
	case "this_week":
		return GetDynamicTaskTimeEntriesWithProject(db, "this_week", 0, projectTaskID)
	case "last_month":
		return GetDynamicTaskTimeEntriesWithProject(db, "last_month", 30, projectTaskID)
	case "this_month":
		return GetDynamicTaskTimeEntriesWithProject(db, "this_month", 0, projectTaskID)
	case "last_x_days":
		// This shouldn't happen as last_x_days should be handled with specific days
		return GetDynamicTaskTimeEntriesWithProject(db, "last_x_days", 7, projectTaskID)
	default:
		// Check if it's a last_x_days pattern with specific days
		if strings.HasPrefix(period, "last_") && strings.HasSuffix(period, "_days") {
			// Extract days from pattern like "last_7_days"
			daysPart := strings.TrimPrefix(period, "last_")
			daysPart = strings.TrimSuffix(daysPart, "_days")
			if days, err := strconv.Atoi(daysPart); err == nil && days >= 1 && days <= 60 {
				return GetDynamicTaskTimeEntriesWithProject(db, "last_x_days", days, projectTaskID)
			}
		}
		return nil, fmt.Errorf("invalid period: %s", period)
	}
}

func formatProjectMessage(project string, tasks []TaskUpdateInfo, period string) SlackMessage {
	var title string
	var date string
	switch period {
	case "daily":
		title = fmt.Sprintf("📊 Daily Task Update for %s", project)
		date = time.Now().Format("January 2, 2006")
	case "weekly":
		title = fmt.Sprintf("📈 Weekly Task Summary for %s", project)
		date = time.Now().Format("January 2, 2006")
	case "monthly":
		title = fmt.Sprintf("📅 Monthly Task Summary for %s", project)
		date = time.Now().Format("January 2006")
	}

	if project == "Other" {
		title = "📋 Other Tasks Update"
	} else if project != "" {
		title = fmt.Sprintf("📁 %s Project Update", project)
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s* - %s\n\n", title, date))

	// Header blocks (4 blocks total)
	headerBlocks := []Block{
		// Add spacing at the top for better separation between projects
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: " "},
		},
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: date},
			},
		},
		{Type: "divider"},
	}

	// Check if we need to split tasks due to block limits
	headerBlockCount := len(headerBlocks)
	footerBlockCount := 1 // One spacing block at the bottom
	maxTasksForMessage := MaxBlocksPerMessage - headerBlockCount - footerBlockCount

	blocks := make([]Block, 0, MaxBlocksPerMessage)
	blocks = append(blocks, headerBlocks...)

	// Only add tasks that fit within the block limit
	taskCount := 0
	for _, task := range tasks {
		if taskCount >= maxTasksForMessage {
			break // Stop adding tasks to avoid exceeding block limit
		}
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskTextMessage(&messageText, task)
		taskCount++
	}

	// Add spacing at the bottom for better separation between projects
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "},
	})

	message := SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}

	// Validate the message
	validation := validateSlackMessage(message)
	if !validation.IsValid {
		logger := GetGlobalLogger()
		logger.Warnf("Message validation failed: %s", validation.ErrorMessage)

		// If still too large, truncate text and try again
		if validation.ExceedsChars {
			truncatedText := message.Text
			if len(truncatedText) > MaxSlackMessageChars-100 {
				truncatedText = truncatedText[:MaxSlackMessageChars-100] + "...\n\n(Message truncated due to size limit)"
			}
			message.Text = truncatedText
		}
	}

	return message
}

// formatSingleTaskBlock formats a single task into one comprehensive markdown block
func formatSingleTaskBlock(task TaskUpdateInfo) Block {
	taskName := sanitizeSlackText(task.Name)

	var taskInfo strings.Builder
	taskInfo.WriteString(fmt.Sprintf("*%s*\n", taskName))

	// Time information with user breakdown if multiple users
	timeInfo := fmt.Sprintf("• %s: %s | %s: %s",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime)

	// Add user breakdown if there are multiple users
	if len(task.UserBreakdown) > 1 {
		timeInfo += " ["
		var userContribs []string
		var sortedUserIDs []int

		// Collect and sort user IDs for consistent ordering
		for userID := range task.UserBreakdown {
			sortedUserIDs = append(sortedUserIDs, userID)
		}
		sort.Ints(sortedUserIDs)

		// Get database connection for user name lookups
		db, err := GetDB()
		var userDisplayNames map[int]string
		if err == nil {
			userDisplayNames = GetAllUserDisplayNames(db, sortedUserIDs)
		}

		for _, userID := range sortedUserIDs {
			contrib := task.UserBreakdown[userID]
			// Only show users who contributed time in the current period
			if contrib.CurrentTime != "0h 0m" {
				userName := fmt.Sprintf("user%d", userID) // fallback
				if userDisplayNames != nil {
					if displayName, exists := userDisplayNames[userID]; exists {
						userName = displayName
					}
				}
				userContribs = append(userContribs, fmt.Sprintf("%s: %s", userName, contrib.CurrentTime))
			}
		}

		if len(userContribs) > 0 {
			timeInfo += strings.Join(userContribs, ", ") + "]"
		} else {
			// Remove the opening bracket if no users contributed
			timeInfo = strings.TrimSuffix(timeInfo, " [")
		}
	}
	taskInfo.WriteString(timeInfo)
	taskInfo.WriteString("\n")

	// Estimation info
	if task.EstimationInfo != "" {
		estimationInfo := sanitizeSlackText(task.EstimationInfo)
		taskInfo.WriteString(fmt.Sprintf("• %s\n", estimationInfo))
	}

	// remove all empty comments
	task.Comments = removeEmptyComments(task.Comments)

	// Comments - display all comments instead of summarizing
	if len(task.Comments) > 0 {
		taskInfo.WriteString("• Comments:\n")
		for i, comment := range task.Comments {
			if comment == "" {
				continue
			}
			comment = sanitizeSlackText(comment)
			// Check if adding this comment would exceed reasonable block size
			currentText := taskInfo.String()
			commentText := fmt.Sprintf("  %d. %s\n", i+1, comment)

			// If the current block would be too long, truncate and indicate more comments
			if len(currentText+commentText) > 2800 {
				remaining := len(task.Comments) - i
				taskInfo.WriteString(fmt.Sprintf("  ... and %d more comments (see additional message)\n", remaining))
				break
			}
			taskInfo.WriteString(commentText)
		}
	}

	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: taskInfo.String()},
	}
}

// formatTaskCommentsBlocks creates additional blocks for comments that don't fit in the main task block
func formatTaskCommentsBlocks(task TaskUpdateInfo, startIndex int) []Block {
	var blocks []Block
	task.Comments = removeEmptyComments(task.Comments)

	if len(task.Comments) <= startIndex {
		return blocks
	}

	var commentText strings.Builder
	commentText.WriteString(fmt.Sprintf("*%s - Additional Comments:*\n", sanitizeSlackText(task.Name)))

	for i := startIndex; i < len(task.Comments); i++ {
		comment := task.Comments[i]
		if comment == "" {
			continue
		}
		comment = sanitizeSlackText(comment)
		newCommentText := fmt.Sprintf("%d. %s\n", i+1, comment)

		// Check if adding this comment would exceed block size
		if len(commentText.String()+newCommentText) > 2800 {
			// Create a block with current comments
			if commentText.Len() > 0 {
				blocks = append(blocks, Block{
					Type: "section",
					Text: &Text{Type: "mrkdwn", Text: commentText.String()},
				})
			}

			// Start a new block
			commentText.Reset()
			commentText.WriteString(fmt.Sprintf("*%s - More Comments:*\n", sanitizeSlackText(task.Name)))
		}

		commentText.WriteString(newCommentText)
	}

	// Add the final block if there's content
	if commentText.Len() > 0 {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: commentText.String()},
		})
	}

	return blocks
}

func removeEmptyComments(comments []string) []string {
	var nonEmptyComments []string
	for _, comment := range comments {
		if comment != "" {
			nonEmptyComments = append(nonEmptyComments, comment)
		}
	}
	return nonEmptyComments
}

// getProjectNameForTask finds the project-level parent for a task.
// A "project" is defined as the ancestor that is one level below the ultimate root task.
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

// groupTasksByTopParent groups tasks by their project, which is one level below the root.
func groupTasksByTopParent(tasks []TaskUpdateInfo, allTasks map[int]Task) map[string][]TaskUpdateInfo {
	projects := make(map[string][]TaskUpdateInfo)

	for _, task := range tasks {
		projectName := getProjectNameForTask(task.TaskID, allTasks)
		projects[projectName] = append(projects[projectName], task)
	}

	return projects
}

func appendTaskTextMessage(builder *strings.Builder, task TaskUpdateInfo) {
	builder.WriteString(fmt.Sprintf("*%s*", task.Name))
	if task.EstimationInfo != "" {
		builder.WriteString(fmt.Sprintf(" | %s", task.EstimationInfo))
	}

	timeText := fmt.Sprintf("\nTime worked: %s: %s, %s: %s", task.CurrentPeriod, task.CurrentTime, task.PreviousPeriod, task.PreviousTime)

	// Add user breakdown if there are multiple users
	if len(task.UserBreakdown) > 1 {
		timeText += " ["
		var userContribs []string
		var sortedUserIDs []int

		// Collect and sort user IDs for consistent ordering
		for userID := range task.UserBreakdown {
			sortedUserIDs = append(sortedUserIDs, userID)
		}
		sort.Ints(sortedUserIDs)

		// Get database connection for user name lookups
		db, err := GetDB()
		var userDisplayNames map[int]string
		if err == nil {
			userDisplayNames = GetAllUserDisplayNames(db, sortedUserIDs)
		}

		for _, userID := range sortedUserIDs {
			contrib := task.UserBreakdown[userID]
			// Only show users who contributed time in the current period
			if contrib.CurrentTime != "0h 0m" {
				userName := fmt.Sprintf("user%d", userID) // fallback
				if userDisplayNames != nil {
					if displayName, exists := userDisplayNames[userID]; exists {
						userName = displayName
					}
				}
				userContribs = append(userContribs, fmt.Sprintf("%s: %s", userName, contrib.CurrentTime))
			}
		}

		if len(userContribs) > 0 {
			timeText += strings.Join(userContribs, ", ") + "]"
		} else {
			// Remove the opening bracket if no users contributed
			timeText = strings.TrimSuffix(timeText, " [")
		}
	}

	builder.WriteString(timeText)
	builder.WriteString("\n\n")
}

func sanitizeSlackText(text string) string {
	// Remove or escape characters that can cause issues in Slack blocks
	// Replace problematic characters that might break JSON or Slack formatting
	text = strings.ReplaceAll(text, "\n", " ") // Replace newlines with spaces in task names
	text = strings.ReplaceAll(text, "\r", " ") // Replace carriage returns
	text = strings.ReplaceAll(text, "\t", " ") // Replace tabs
	text = strings.ReplaceAll(text, "\"", "'") // Replace double quotes with single quotes
	text = strings.ReplaceAll(text, "\\", "/") // Replace backslashes

	// Trim excessive whitespace
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	// Limit text length to prevent Slack API issues (Slack has limits on block text length)
	if len(text) > 3000 {
		text = text[:2997] + "..."
	}

	return text
}

func sendNoChangesNotification(period, responseURL string, asJSON bool) error {
	message := SlackMessage{
		Text: fmt.Sprintf("No task changes to report for %s.", period),
	}
	if asJSON {
		outputJSON([]SlackMessage{message})
		return nil
	}
	if responseURL != "" {
		return sendDelayedResponseShared(responseURL, message)
	}
	return sendSlackMessage(message)
}

// sendSlackMessage sends a message to Slack using the webhook
func sendSlackMessage(message SlackMessage) error {
	logger := GetGlobalLogger()

	// Validate message before sending
	validation := validateSlackMessage(message)
	if !validation.IsValid {
		logger.Errorf("Message validation failed before sending: %s", validation.ErrorMessage)
		return fmt.Errorf("message validation failed: %s", validation.ErrorMessage)
	}

	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("SLACK_WEBHOOK_URL environment variable not set")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling message: %w", err)
	}

	// Log the JSON payload for debugging (first 500 chars to avoid spam)
	jsonStr := string(jsonData)
	if len(jsonStr) > 500 {
		logger.Debugf("Sending Slack message payload (truncated): %s...", jsonStr[:500])
	} else {
		logger.Debugf("Sending Slack message payload: %s", jsonStr)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Errorf("Slack API error - Status: %d, Response: %s", resp.StatusCode, string(body))
		return fmt.Errorf("slack API returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Debugf("Successfully sent message to Slack webhook")
	return nil
}

func outputJSON(messages []SlackMessage) {
	json.NewEncoder(os.Stdout).Encode(messages)
}

func parseEstimation(taskName string) (string, string) {
	logger := GetGlobalLogger()

	// Helper function to parse float from string, supporting both . and , as decimal separators
	parseFloat := func(s string) (float64, error) {
		// Replace comma with dot for consistent parsing
		s = strings.ReplaceAll(s, ",", ".")
		return strconv.ParseFloat(s, 64)
	}

	// Helper function to format float for display (remove unnecessary decimals)
	formatFloat := func(f float64) string {
		if f == float64(int(f)) {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprintf("%.1f", f)
	}

	// Try to match different estimation patterns
	patterns := []struct {
		regex      *regexp.Regexp
		format     string
		isRange    bool
		isAddition bool
	}{
		// Range formats (supporting floats with . or , as decimal separator)
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)\]`), "hours", true, false},   // [number-number]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)h\]`), "hours", true, false}, // [numberh-numberh]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)h\]`), "hours", true, false},  // [number-numberh]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)\]`), "hours", true, false},  // [numberh-number]

		// Addition formats (min + addition = max)
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)\]`), "hours", true, true},   // [number+number]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)h\]`), "hours", true, true},  // [number+numberh]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)\]`), "hours", true, true},  // [numberh+number]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)h\]`), "hours", true, true}, // [numberh+numberh]

		// Single number formats (supporting floats)
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\]`), "hours", false, false},  // [number]
		{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\]`), "hours", false, false}, // [numberh]
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindStringSubmatch(taskName)

		if pattern.isRange && len(matches) == 3 {
			// Handle range and addition formats
			first, err1 := parseFloat(matches[1])
			second, err2 := parseFloat(matches[2])

			if err1 != nil || err2 != nil {
				logger.Warnf("Failed to parse estimation numbers from task name '%s': first=%v, second=%v",
					taskName, err1, err2)
				continue
			}

			var optimistic, pessimistic float64
			if pattern.isAddition {
				// Addition format: min = first, max = first + second
				optimistic = first
				pessimistic = first + second
			} else {
				// Range format: min = first, max = second
				optimistic = first
				pessimistic = second
			}

			// Validate numbers are not bigger than 100
			if optimistic > 100 || pessimistic > 100 {
				logger.Warnf("Estimation numbers too large in task '%s': min=%s, max=%s (max allowed: 100)",
					taskName, formatFloat(optimistic), formatFloat(pessimistic))
				return fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)), "estimation numbers too large (max: 100)"
			}

			if optimistic > pessimistic {
				logger.Warnf("Invalid estimation range in task '%s': optimistic (%s) > pessimistic (%s)",
					taskName, formatFloat(optimistic), formatFloat(pessimistic))
				return fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)), "broken estimation (optimistic > pessimistic)"
			}

			logger.Debugf("Parsed estimation for task '%s': %s-%s hours", taskName, formatFloat(optimistic), formatFloat(pessimistic))
			return fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)), ""

		} else if !pattern.isRange && len(matches) == 2 {
			// Handle single number formats
			estimate, err := parseFloat(matches[1])

			if err != nil {
				logger.Warnf("Failed to parse estimation number from task name '%s': %v", taskName, err)
				continue
			}

			// Validate number is not bigger than 100
			if estimate > 100 {
				logger.Warnf("Estimation number too large in task '%s': %s (max allowed: 100)", taskName, formatFloat(estimate))
				return fmt.Sprintf("Estimation: %s hours", formatFloat(estimate)), "estimation number too large (max: 100)"
			}

			logger.Debugf("Parsed single estimation for task '%s': %s hours", taskName, formatFloat(estimate))
			return fmt.Sprintf("Estimation: %s hours", formatFloat(estimate)), ""
		}
	}

	logger.Debugf("No estimation pattern found in task name: %s", taskName)
	return "", "no estimation given"
}

// parseEstimationWithUsage enhances parseEstimation by adding usage percentage calculation
func parseEstimationWithUsage(taskName, currentTime, previousTime string) (string, string) {
	estimation, status := parseEstimation(taskName)

	if status != "" {
		return estimation, status
	}

	// Calculate usage percentage
	percentage, _, err := calculateTimeUsagePercentage(currentTime, previousTime, taskName)
	if err != nil {
		return estimation, status
	}

	// Get color indicator
	emoji, description, _ := getColorIndicator(percentage)

	// Enhanced estimation info with percentage and indicator
	return fmt.Sprintf("%s | %s %.1f%% (%s)", estimation, emoji, percentage, description), status
}

func sendFailureNotification(operation string, err error) {
	logger := GetGlobalLogger()

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

func parseTimeToSeconds(timeStr string) int {
	if timeStr == "0h 0m" || timeStr == "" {
		return 0
	}

	var hours, minutes int
	hRegex := regexp.MustCompile(`(\d+)h`)
	mRegex := regexp.MustCompile(`(\d+)m`)

	hMatch := hRegex.FindStringSubmatch(timeStr)
	if len(hMatch) > 1 {
		hours, _ = strconv.Atoi(hMatch[1])
	}

	mMatch := mRegex.FindStringSubmatch(timeStr)
	if len(mMatch) > 1 {
		minutes, _ = strconv.Atoi(mMatch[1])
	}

	return hours*3600 + minutes*60
}

func getColorIndicator(percentage float64) (string, string, bool) {
	var emoji, description string
	var isBold bool

	midPoint := 50.0
	highPoint := 90.0

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

func calculateTimeUsagePercentage(currentTime, previousTime, estimation string) (float64, int, error) {
	// Helper function to parse float from string, supporting both . and , as decimal separators
	parseFloat := func(s string) (float64, error) {
		// Replace comma with dot for consistent parsing
		s = strings.ReplaceAll(s, ",", ".")
		return strconv.ParseFloat(s, 64)
	}

	// Try range patterns first (including addition patterns)
	rangePatterns := []*regexp.Regexp{
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)\]`),   // [number-number]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)h\]`), // [numberh-numberh]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)h\]`),  // [number-numberh]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)\]`),  // [numberh-number]
	}

	for _, re := range rangePatterns {
		matches := re.FindStringSubmatch(estimation)
		if len(matches) == 3 {
			pessimistic, err := parseFloat(matches[2])
			if err != nil {
				continue
			}

			currentSeconds := parseTimeToSeconds(currentTime)
			previousSeconds := parseTimeToSeconds(previousTime)
			totalSeconds := currentSeconds + previousSeconds

			pessimisticSeconds := pessimistic * 3600

			if pessimisticSeconds == 0 {
				return 0, totalSeconds, nil
			}

			percentage := (float64(totalSeconds) / pessimisticSeconds) * 100
			return percentage, totalSeconds, nil
		}
	}

	// Try addition patterns (min + addition = max)
	additionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)\]`),   // [number+number]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)h\]`),  // [number+numberh]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)\]`),  // [numberh+number]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)h\]`), // [numberh+numberh]
	}

	for _, re := range additionPatterns {
		matches := re.FindStringSubmatch(estimation)
		if len(matches) == 3 {
			first, err1 := parseFloat(matches[1])
			second, err2 := parseFloat(matches[2])
			if err1 != nil || err2 != nil {
				continue
			}

			// For addition patterns: max = first + second
			pessimistic := first + second

			currentSeconds := parseTimeToSeconds(currentTime)
			previousSeconds := parseTimeToSeconds(previousTime)
			totalSeconds := currentSeconds + previousSeconds

			pessimisticSeconds := pessimistic * 3600

			if pessimisticSeconds == 0 {
				return 0, totalSeconds, nil
			}

			percentage := (float64(totalSeconds) / pessimisticSeconds) * 100
			return percentage, totalSeconds, nil
		}
	}

	// Try single number patterns
	singlePatterns := []*regexp.Regexp{
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\]`),  // [number]
		regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\]`), // [numberh]
	}

	for _, re := range singlePatterns {
		matches := re.FindStringSubmatch(estimation)
		if len(matches) == 2 {
			estimate, err := parseFloat(matches[1])
			if err != nil {
				continue
			}

			currentSeconds := parseTimeToSeconds(currentTime)
			previousSeconds := parseTimeToSeconds(previousTime)
			totalSeconds := currentSeconds + previousSeconds

			estimateSeconds := estimate * 3600

			if estimateSeconds == 0 {
				return 0, totalSeconds, nil
			}

			percentage := (float64(totalSeconds) / estimateSeconds) * 100
			return percentage, totalSeconds, nil
		}
	}

	return 0, 0, fmt.Errorf("no estimation pattern found")
}

// Task is a simplified struct for holding task hierarchy data
type Task struct {
	ID       int
	ParentID int
	Name     string
}

// getAllTasks fetches all tasks from the database for hierarchy mapping
func getAllTasks(db *sql.DB) (map[int]Task, error) {
	rows, err := db.Query("SELECT task_id, parent_id, name FROM tasks")
	if err != nil {
		return nil, fmt.Errorf("could not query all tasks: %w", err)
	}
	defer rows.Close()

	tasks := make(map[int]Task)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.ParentID, &t.Name); err != nil {
			return nil, fmt.Errorf("could not scan task row: %w", err)
		}
		tasks[t.ID] = t
	}
	return tasks, nil
}

// formatProjectMessageWithComments creates multiple messages if needed to display all comments
func formatProjectMessageWithComments(project string, tasks []TaskUpdateInfo, period string) []SlackMessage {
	var messages []SlackMessage

	// Split tasks into chunks that respect block limits
	headerBlockCount := 4 // spacing, header, context, divider
	// Note: splitTasksByBlockLimit internally accounts for footer blocks
	taskChunks := splitTasksByBlockLimit(tasks, headerBlockCount)

	// Create messages for each chunk
	for i, taskChunk := range taskChunks {
		var chunkMessage SlackMessage

		if len(taskChunks) == 1 {
			// Single message - use the original format
			chunkMessage = formatProjectMessage(project, taskChunk, period)
		} else {
			// Multiple messages - add part indicator
			chunkMessage = formatProjectMessageChunk(project, taskChunk, period, i+1, len(taskChunks))
		}

		// Validate the message before adding
		validation := validateSlackMessage(chunkMessage)
		if !validation.IsValid {
			logger := GetGlobalLogger()
			logger.Warnf("Message chunk validation failed: %s", validation.ErrorMessage)

			// If still too large, try to truncate text
			if validation.ExceedsChars {
				truncatedText := chunkMessage.Text
				if len(truncatedText) > MaxSlackMessageChars-100 {
					truncatedText = truncatedText[:MaxSlackMessageChars-100] + "...\n\n(Message truncated due to size limit)"
				}
				chunkMessage.Text = truncatedText
			}

			// If still too many blocks, we need to split further (shouldn't happen with our logic, but safety check)
			if validation.ExceedsBlocks {
				logger.Errorf("Message still exceeds block limit after splitting: %d blocks", validation.BlockCount)
				// Truncate blocks as last resort
				if len(chunkMessage.Blocks) > MaxSlackBlocks {
					chunkMessage.Blocks = chunkMessage.Blocks[:MaxSlackBlocks-1]
					// Add a truncation notice
					chunkMessage.Blocks = append(chunkMessage.Blocks, Block{
						Type: "section",
						Text: &Text{Type: "mrkdwn", Text: "_... (Some tasks truncated due to message size limits)_"},
					})
				}
			}
		}

		messages = append(messages, chunkMessage)
	}

	// Handle overflow comments if needed (legacy support, but now less likely to be needed)
	var additionalBlocks []Block
	var hasAdditionalComments bool

	for _, task := range tasks {
		task.Comments = removeEmptyComments(task.Comments)
		if len(task.Comments) == 0 {
			continue
		}

		// Count how many comments fit in the main task block
		var fittingComments int
		var currentLength int
		taskBaseLength := len(fmt.Sprintf("*%s*\n• %s: %s | %s: %s\n",
			sanitizeSlackText(task.Name),
			task.CurrentPeriod, task.CurrentTime,
			task.PreviousPeriod, task.PreviousTime))

		if task.EstimationInfo != "" {
			taskBaseLength += len(fmt.Sprintf("• %s\n", sanitizeSlackText(task.EstimationInfo)))
		}

		taskBaseLength += len("• Comments:\n")
		currentLength = taskBaseLength

		for i, comment := range task.Comments {
			if comment == "" {
				continue
			}
			commentLength := len(fmt.Sprintf("  %d. %s\n", i+1, sanitizeSlackText(comment)))
			if currentLength+commentLength > 2800 {
				break
			}
			currentLength += commentLength
			fittingComments++
		}

		// If there are more comments than what fit, create additional blocks
		if fittingComments < len(task.Comments) {
			commentBlocks := formatTaskCommentsBlocks(task, fittingComments)
			additionalBlocks = append(additionalBlocks, commentBlocks...)
			hasAdditionalComments = true
		}
	}

	// Create additional messages for overflow comments
	if hasAdditionalComments {
		for i := 0; i < len(additionalBlocks); i += MaxBlocksPerMessage {
			end := i + MaxBlocksPerMessage
			if end > len(additionalBlocks) {
				end = len(additionalBlocks)
			}

			blockChunk := additionalBlocks[i:end]

			// Create message header
			messageBlocks := []Block{
				{
					Type: "section",
					Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("📝 *Additional Comments for %s* (Part %d)", project, (i/MaxBlocksPerMessage)+1)},
				},
				{Type: "divider"},
			}

			messageBlocks = append(messageBlocks, blockChunk...)

			additionalMessage := SlackMessage{
				Text:   fmt.Sprintf("Additional Comments for %s", project),
				Blocks: messageBlocks,
			}

			// Validate additional message
			validation := validateSlackMessage(additionalMessage)
			if !validation.IsValid {
				logger := GetGlobalLogger()
				logger.Warnf("Additional comments message validation failed: %s", validation.ErrorMessage)
			}

			messages = append(messages, additionalMessage)
		}
	}

	return messages
}

// formatProjectMessageChunk creates a message for a chunk of tasks with part indicator
func formatProjectMessageChunk(project string, tasks []TaskUpdateInfo, period string, partNum, totalParts int) SlackMessage {
	var title string
	var date string
	switch period {
	case "daily":
		title = fmt.Sprintf("📊 Daily Task Update for %s", project)
		date = time.Now().Format("January 2, 2006")
	case "weekly":
		title = fmt.Sprintf("📈 Weekly Task Summary for %s", project)
		date = time.Now().Format("January 2, 2006")
	case "monthly":
		title = fmt.Sprintf("📅 Monthly Task Summary for %s", project)
		date = time.Now().Format("January 2006")
	}

	if project == "Other" {
		title = "📋 Other Tasks Update"
	} else if project != "" {
		title = fmt.Sprintf("📁 %s Project Update", project)
	}

	// Add part indicator if multiple parts
	if totalParts > 1 {
		title = fmt.Sprintf("%s (Part %d of %d)", title, partNum, totalParts)
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s* - %s\n\n", title, date))

	blocks := []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: " "},
		},
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: date},
			},
		},
		{Type: "divider"},
	}

	for _, task := range tasks {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskTextMessage(&messageText, task)
	}

	// Add spacing at the bottom
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "},
	})

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// GetTasksOverThreshold returns tasks that are over a specific percentage of their estimation
func GetTasksOverThreshold(db *sql.DB, threshold float64, period string, days int) ([]TaskUpdateInfo, error) {
	return GetTasksOverThresholdWithProject(db, threshold, period, days, nil)
}

// GetTasksOverThresholdWithProject returns tasks that are over a specific percentage of their estimation, optionally filtered by project
func GetTasksOverThresholdWithProject(db *sql.DB, threshold float64, period string, days int, projectTaskID *int) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()

	// Use the same date calculation logic as other functions
	dateRanges := calculateDateRanges(period, days)
	fromDate := dateRanges.Current.Start
	toDate := dateRanges.Current.End

	// Build project filtering parameters
	var projectFilterClause string
	var userQueryArgs []interface{}
	var mainQueryArgs []interface{}

	userQueryArgs = append(userQueryArgs, fromDate, toDate)
	mainQueryArgs = append(mainQueryArgs, fromDate, toDate)

	if projectTaskID != nil {
		projectTaskIDs, err := GetProjectTaskIDs(db, *projectTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project task IDs: %w", err)
		}
		if len(projectTaskIDs) == 0 {
			return []TaskUpdateInfo{}, nil
		}
		projectFilterClause = " AND t.task_id = ANY($3)"
		userQueryArgs = append(userQueryArgs, pq.Array(projectTaskIDs))
		mainQueryArgs = append(mainQueryArgs, pq.Array(projectTaskIDs))
	}

	// First get user breakdown data for all tasks with estimations
	userBreakdownQuery := fmt.Sprintf(`
		SELECT 
			t.task_id,
			te.user_id,
			COALESCE(SUM(CASE WHEN te.date BETWEEN $1 AND $2 THEN te.duration ELSE 0 END), 0) as current_duration,
			COALESCE(SUM(CASE WHEN te.date < $1 THEN te.duration ELSE 0 END), 0) as previous_duration
		FROM tasks t
		INNER JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'%s  -- Only tasks with estimation patterns
		GROUP BY t.task_id, te.user_id
		HAVING COALESCE(SUM(te.duration), 0) > 0  -- Only users with time logged
	`, projectFilterClause)

	userRows, err := db.Query(userBreakdownQuery, userQueryArgs...)
	if err != nil {
		logger.Warnf("Failed to query user breakdown for threshold tasks: %v", err)
	}
	defer userRows.Close()

	// Build user breakdown map: taskID -> userID -> contribution
	userBreakdowns := make(map[int]map[int]UserTimeContribution)
	if userRows != nil {
		for userRows.Next() {
			var taskID, userID, currentDuration, previousDuration int
			err := userRows.Scan(&taskID, &userID, &currentDuration, &previousDuration)
			if err != nil {
				logger.Warnf("Failed to scan user breakdown row for threshold tasks: %v", err)
				continue
			}

			if _, exists := userBreakdowns[taskID]; !exists {
				userBreakdowns[taskID] = make(map[int]UserTimeContribution)
			}

			userBreakdowns[taskID][userID] = UserTimeContribution{
				UserID:       userID,
				CurrentTime:  formatDuration(currentDuration),
				PreviousTime: formatDuration(previousDuration),
			}
		}
	}

	// Main query - get all tasks with estimations and time entries in the current period
	query := fmt.Sprintf(`
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE WHEN te.date BETWEEN $1 AND $2 THEN te.duration ELSE 0 END), 0) as current_duration,
			COALESCE(SUM(CASE WHEN te.date < $1 THEN te.duration ELSE 0 END), 0) as previous_duration
		FROM tasks t
		INNER JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'%s  -- Only tasks with estimation patterns
		  AND te.date BETWEEN $1 AND $2  -- Only tasks with time entries in current period
		GROUP BY t.task_id, t.parent_id, t.name
		ORDER BY t.task_id
	`, projectFilterClause)

	rows, err := db.Query(query, mainQueryArgs...)
	if err != nil {
		return nil, fmt.Errorf("could not query tasks over threshold: %w", err)
	}
	defer rows.Close()

	var tasks []TaskUpdateInfo
	taskIDs := make([]int, 0)

	for rows.Next() {
		var task TaskUpdateInfo
		var currentDuration, previousDuration int

		err := rows.Scan(
			&task.TaskID,
			&task.ParentID,
			&task.Name,
			&currentDuration,
			&previousDuration,
		)
		if err != nil {
			logger.Warnf("Failed to scan task row: %v", err)
			continue
		}

		task.CurrentTime = formatDuration(currentDuration)
		task.PreviousTime = formatDuration(previousDuration)

		// Calculate percentage using existing Go function
		percentage, _, err := calculateTimeUsagePercentage(task.CurrentTime, task.PreviousTime, task.Name)
		if err != nil {
			logger.Warnf("Failed to calculate percentage for task %s: %v", task.Name, err)
			continue
		}

		// Only include tasks that meet the threshold
		if percentage < threshold {
			continue
		}

		// Set period labels based on the period parameter
		switch period {
		case "daily":
			task.CurrentPeriod = "Today"
			task.PreviousPeriod = "Before today"
		case "weekly":
			task.CurrentPeriod = "This week"
			task.PreviousPeriod = "Before this week"
		case "monthly":
			task.CurrentPeriod = "This month"
			task.PreviousPeriod = "Before this month"
		default:
			task.CurrentPeriod = "Recent"
			task.PreviousPeriod = "Previous"
		}

		// Parse estimation with usage percentage
		task.EstimationInfo, task.EstimationStatus = parseEstimationWithUsage(task.Name, task.CurrentTime, task.PreviousTime)

		// Add user breakdown
		if breakdown, exists := userBreakdowns[task.TaskID]; exists {
			task.UserBreakdown = breakdown
		}

		taskIDs = append(taskIDs, task.TaskID)
		tasks = append(tasks, task)
	}

	if len(taskIDs) > 0 {
		// Get comments for all tasks in batch
		comments, err := getTaskCommentsBulk(db, taskIDs, fromDate, toDate)
		if err != nil {
			logger.Warnf("Failed to get task comments: %v", err)
		} else {
			// Assign comments to tasks
			for i := range tasks {
				if taskComments, ok := comments[tasks[i].TaskID]; ok {
					tasks[i].Comments = taskComments
				}
			}
		}
	}

	logger.Infof("Found %d tasks over %.1f%% threshold for %s period", len(tasks), threshold, period)
	return tasks, nil
}

// CheckThresholdAlerts checks for tasks that just crossed specific thresholds or are persistently over critical thresholds
func CheckThresholdAlerts(db *sql.DB) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	// Define the thresholds we want to monitor
	thresholds := []int{50, 80, 90, 100}
	var allAlerts []ThresholdAlert

	// Get current time for comparison
	now := time.Now()
	sixHoursAgo := now.Add(-6 * time.Hour) // Look back 6 hours for testing (will reduce back to reasonable time)
	oneHourAgo := now.Add(-1 * time.Hour)

	for _, threshold := range thresholds {
		// Check for newly crossed thresholds (primary detection)
		alerts, err := getTasksJustCrossedThreshold(db, float64(threshold), sixHoursAgo)
		if err != nil {
			logger.Errorf("Failed to check %d%% threshold crossings: %v", threshold, err)
		} else {
			allAlerts = append(allAlerts, alerts...)
		}

		// For 100% threshold only, send periodic alerts for tasks that have been worked on recently and got worse
		if threshold == 100 {
			persistentAlerts, err := getTasksPersistentlyOverThreshold(db, float64(threshold), oneHourAgo)
			if err != nil {
				logger.Errorf("Failed to check persistent %d%% threshold tasks: %v", threshold, err)
			} else {
				allAlerts = append(allAlerts, persistentAlerts...)
			}
		}
	}

	return allAlerts, nil
}

// getTasksJustCrossedThreshold finds tasks that just crossed a threshold in the last time period
func getTasksJustCrossedThreshold(db *sql.DB, threshold float64, since time.Time) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	logger.Debugf("Checking for %.1f%% threshold crossings since %s (%.1f minutes ago)",
		threshold, since.Format("15:04:05"), time.Since(since).Minutes())

	// First, let's check how many tasks have estimations at all
	estimationQuery := `
		SELECT COUNT(*) 
		FROM tasks 
		WHERE name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'
	`
	var estimationCount int
	err := db.QueryRow(estimationQuery).Scan(&estimationCount)
	if err != nil {
		logger.Warnf("Failed to count tasks with estimations: %v", err)
	} else {
		logger.Debugf("Total tasks with estimation patterns: %d", estimationCount)
	}

	// Check how many time entries are in our recent window (using date as text)
	recentQuery := `
		SELECT COUNT(*) 
		FROM time_entries 
		WHERE date = to_char(CURRENT_DATE, 'YYYY-MM-DD') AND duration > 0
	`
	var recentCount int
	err = db.QueryRow(recentQuery).Scan(&recentCount)
	if err != nil {
		logger.Warnf("Failed to count recent time entries: %v", err)
	} else {
		logger.Debugf("Recent time entries (today): %d", recentCount)
	}

	// Enhanced query - get tasks with estimations and recent activity using date as text
	query := `
		WITH recent_entries AS (
			-- Get time entries from today (date stored as text in YYYY-MM-DD format)
			SELECT task_id, duration
			FROM time_entries 
			WHERE date = to_char(CURRENT_DATE, 'YYYY-MM-DD')
			  AND duration > 0
		),
		tasks_with_estimations AS (
			-- Get tasks that have estimation patterns AND recent activity
			SELECT DISTINCT t.task_id, t.parent_id, t.name
			FROM tasks t
			INNER JOIN recent_entries re ON t.task_id = re.task_id
			WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'  -- Only tasks with estimation patterns
		),
		task_totals AS (
			-- Calculate total time and recent time for tasks with estimations
			SELECT 
				twe.task_id,
				twe.parent_id,
				twe.name,
				COALESCE(SUM(te.duration), 0) as total_duration,
				COALESCE(SUM(CASE WHEN re.task_id IS NOT NULL THEN re.duration ELSE 0 END), 0) as recent_duration
			FROM tasks_with_estimations twe
			LEFT JOIN time_entries te ON twe.task_id = te.task_id
			LEFT JOIN recent_entries re ON twe.task_id = re.task_id
			GROUP BY twe.task_id, twe.parent_id, twe.name
		)
		SELECT 
			task_id,
			parent_id,
			name,
			total_duration,
			recent_duration
		FROM task_totals
		WHERE total_duration > 0
		  AND recent_duration > 0  -- Only tasks that had recent activity
	`

	rows, err := db.Query(query) // No parameter needed since we use CURRENT_DATE
	if err != nil {
		return nil, fmt.Errorf("could not query threshold crossings: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert
	var alertTaskIDs []int
	taskCount := 0

	for rows.Next() {
		var taskID, parentID int
		var name string
		var totalDuration, recentDuration int

		err := rows.Scan(&taskID, &parentID, &name, &totalDuration, &recentDuration)
		if err != nil {
			logger.Warnf("Failed to scan task row: %v", err)
			continue
		}

		taskCount++
		logger.Debugf("Found task with recent activity: %s (total: %d min, recent: %d min)",
			name, totalDuration, recentDuration)

		// Parse estimation using existing Go function
		_, status := parseEstimation(name)
		if status != "" {
			// Skip tasks without valid estimations
			logger.Debugf("Skipping task %s: %s", name, status)
			continue
		}

		// Calculate current and previous totals
		previousTotal := totalDuration - recentDuration

		// Calculate percentages using existing Go function
		currentPercentage, _, err := calculateTimeUsagePercentage(
			formatDuration(recentDuration),
			formatDuration(previousTotal),
			name,
		)
		if err != nil {
			logger.Warnf("Failed to calculate percentage for task %s: %v", name, err)
			continue
		}

		previousPercentage, _, err := calculateTimeUsagePercentage(
			"0h 0m",
			formatDuration(previousTotal),
			name,
		)
		if err != nil {
			logger.Warnf("Failed to calculate previous percentage for task %s: %v", name, err)
			continue
		}

		logger.Debugf("Task %s: %.1f%% -> %.1f%% (threshold: %.1f%%)",
			name, previousPercentage, currentPercentage, threshold)

		// Check if this task just crossed the threshold
		if previousPercentage < threshold && currentPercentage >= threshold {
			// Check if we already sent a notification for this threshold
			alreadyNotified, err := hasNotificationBeenSent(db, taskID, int(threshold))
			if err != nil {
				logger.Warnf("Failed to check notification status for task %d, threshold %d: %v", taskID, int(threshold), err)
			} else if alreadyNotified {
				logger.Debugf("Skipping already notified task %s for %.1f%% threshold", name, threshold)
				continue
			}

			alert := ThresholdAlert{
				TaskID:           taskID,
				ParentID:         parentID,
				Name:             name,
				CurrentTime:      formatDuration(totalDuration),
				PreviousTime:     formatDuration(previousTotal),
				Percentage:       currentPercentage,
				ThresholdCrossed: int(threshold),
				JustCrossed:      true,
			}

			// Parse estimation info
			alert.EstimationInfo, _ = parseEstimationWithUsage(alert.Name, alert.CurrentTime, alert.PreviousTime)

			alerts = append(alerts, alert)
			alertTaskIDs = append(alertTaskIDs, taskID)

			logger.Infof("THRESHOLD CROSSING: Task %s crossed %.1f%% threshold (%.1f%% -> %.1f%%)",
				name, threshold, previousPercentage, currentPercentage)
		}
	}

	logger.Debugf("Processed %d tasks with recent activity for %.1f%% threshold", taskCount, threshold)

	if len(alerts) > 0 {
		logger.Infof("Found %d tasks that just crossed %.1f%% threshold", len(alerts), threshold)
	}

	return alerts, nil
}

// getTasksPersistentlyOverThreshold finds tasks that have been over 100% threshold and got worse with recent work
// Only triggers for 100% threshold when tasks have recent activity and percentage increased
func getTasksPersistentlyOverThreshold(db *sql.DB, threshold float64, since time.Time) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	// Only run for 100% threshold
	if threshold != 100 {
		return []ThresholdAlert{}, nil
	}

	// Query for tasks with estimations that are currently over 100% AND have recent activity
	query := `
		WITH recent_entries AS (
			-- Get time entries from the last 5 minutes (matching threshold crossing window)
			SELECT task_id, duration
			FROM time_entries 
			WHERE modify_time >= $1
			  AND duration > 0
		),
		tasks_with_estimations AS (
			-- Get tasks that have estimation patterns AND recent activity
			SELECT DISTINCT t.task_id, t.parent_id, t.name
			FROM tasks t
			INNER JOIN recent_entries re ON t.task_id = re.task_id
			WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'  -- Only tasks with estimation patterns
		),
		task_totals AS (
			-- Calculate total time and recent time for these tasks
			SELECT 
				twe.task_id,
				twe.parent_id,
				twe.name,
				COALESCE(SUM(te.duration), 0) as total_duration,
				COALESCE(SUM(CASE WHEN re.task_id IS NOT NULL THEN re.duration ELSE 0 END), 0) as recent_duration
			FROM tasks_with_estimations twe
			LEFT JOIN time_entries te ON twe.task_id = te.task_id
			LEFT JOIN recent_entries re ON twe.task_id = re.task_id
			GROUP BY twe.task_id, twe.parent_id, twe.name
		)
		SELECT 
			task_id,
			parent_id,
			name,
			total_duration,
			recent_duration
		FROM task_totals
		WHERE total_duration > 0
		  AND recent_duration > 0  -- Only tasks that had recent activity
	`

	rows, err := db.Query(query, since)
	if err != nil {
		return nil, fmt.Errorf("could not query persistent threshold violations: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert
	for rows.Next() {
		var taskID, parentID int
		var name string
		var totalDuration, recentDuration int

		err := rows.Scan(&taskID, &parentID, &name, &totalDuration, &recentDuration)
		if err != nil {
			logger.Warnf("Failed to scan task row: %v", err)
			continue
		}

		// Parse estimation using existing Go function
		_, status := parseEstimation(name)
		if status != "" {
			// Skip tasks without valid estimations
			continue
		}

		// Calculate current and previous percentages
		previousTotal := totalDuration - recentDuration

		// Calculate current percentage (with recent work)
		currentPercentage, _, err := calculateTimeUsagePercentage(
			formatDuration(recentDuration),
			formatDuration(previousTotal),
			name,
		)
		if err != nil {
			logger.Warnf("Failed to calculate current percentage for task %s: %v", name, err)
			continue
		}

		// Calculate previous percentage (before recent work)
		previousPercentage, _, err := calculateTimeUsagePercentage(
			"0h 0m",
			formatDuration(previousTotal),
			name,
		)
		if err != nil {
			logger.Warnf("Failed to calculate previous percentage for task %s: %v", name, err)
			continue
		}

		// Only alert if:
		// 1. Task is currently over 100% threshold
		// 2. The percentage has increased due to recent work (got worse)
		// 3. It was already over 100% before (persistent violation)
		if currentPercentage >= threshold && previousPercentage >= threshold && currentPercentage > previousPercentage {
			alert := ThresholdAlert{
				TaskID:           taskID,
				ParentID:         parentID,
				Name:             name,
				CurrentTime:      formatDuration(totalDuration),
				PreviousTime:     formatDuration(previousTotal),
				Percentage:       currentPercentage,
				ThresholdCrossed: int(threshold),
				JustCrossed:      false, // This is a persistent alert with worsening
			}

			// Parse estimation info
			alert.EstimationInfo, _ = parseEstimationWithUsage(alert.Name, alert.CurrentTime, alert.PreviousTime)

			alerts = append(alerts, alert)

			logger.Debugf("Persistent 100%% alert for task %s: %.1f%% -> %.1f%% (got %.1f%% worse)",
				name, previousPercentage, currentPercentage, currentPercentage-previousPercentage)
		}
	}

	if len(alerts) > 0 {
		logger.Infof("Found %d tasks over 100%% that got worse with recent work", len(alerts))
	}

	return alerts, nil
}

// SendThresholdAlerts sends Slack notifications for threshold crossings
func SendThresholdAlerts(alerts []ThresholdAlert) error {
	if len(alerts) == 0 {
		return nil
	}

	logger := GetGlobalLogger()

	// Group alerts by threshold and type (new crossing vs persistent)
	newCrossingGroups := make(map[int][]ThresholdAlert)
	persistentGroups := make(map[int][]ThresholdAlert)

	for _, alert := range alerts {
		if alert.JustCrossed {
			newCrossingGroups[alert.ThresholdCrossed] = append(newCrossingGroups[alert.ThresholdCrossed], alert)
		} else {
			persistentGroups[alert.ThresholdCrossed] = append(persistentGroups[alert.ThresholdCrossed], alert)
		}
	}

	// Get all tasks for hierarchy mapping
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	allTasks, err := getAllTasks(db)
	if err != nil {
		return fmt.Errorf("failed to get all tasks for hierarchy mapping: %w", err)
	}

	// Send messages for new threshold crossings (higher priority)
	for threshold, thresholdAlerts := range newCrossingGroups {
		err := sendThresholdAlertsForGroup(thresholdAlerts, threshold, true, allTasks, db)
		if err != nil {
			logger.Errorf("Failed to send new crossing alerts for %d%% threshold: %v", threshold, err)
		}
	}

	// Send messages for persistent threshold violations (only for 100% threshold)
	for threshold, thresholdAlerts := range persistentGroups {
		if threshold == 100 { // Only send persistent alerts for 100% threshold
			err := sendThresholdAlertsForGroup(thresholdAlerts, threshold, false, allTasks, db)
			if err != nil {
				logger.Errorf("Failed to send persistent alerts for %d%% threshold: %v", threshold, err)
			}
		}
	}

	return nil
}

// sendThresholdAlertsForGroup sends alerts for a specific threshold group
func sendThresholdAlertsForGroup(thresholdAlerts []ThresholdAlert, threshold int, isNewCrossing bool, allTasks map[int]Task, db *sql.DB) error {
	logger := GetGlobalLogger()

	// Get task IDs for user breakdown query
	var alertTaskIDs []int
	for _, alert := range thresholdAlerts {
		alertTaskIDs = append(alertTaskIDs, alert.TaskID)
	}

	// Get user breakdown data for threshold alerts
	var userBreakdowns map[int]map[int]UserTimeContribution
	if len(alertTaskIDs) > 0 {
		userBreakdownQuery := `
			SELECT 
				te.task_id,
				te.user_id,
				COALESCE(SUM(te.duration), 0) as total_duration
			FROM time_entries te
			WHERE te.task_id = ANY($1)
			GROUP BY te.task_id, te.user_id
			HAVING COALESCE(SUM(te.duration), 0) > 0
		`

		userRows, err := db.Query(userBreakdownQuery, pq.Array(alertTaskIDs))
		if err != nil {
			logger.Warnf("Failed to query user breakdown for threshold alerts: %v", err)
		} else {
			defer userRows.Close()
			userBreakdowns = make(map[int]map[int]UserTimeContribution)

			for userRows.Next() {
				var taskID, userID, totalDuration int
				err := userRows.Scan(&taskID, &userID, &totalDuration)
				if err != nil {
					logger.Warnf("Failed to scan user breakdown row for threshold alerts: %v", err)
					continue
				}

				if _, exists := userBreakdowns[taskID]; !exists {
					userBreakdowns[taskID] = make(map[int]UserTimeContribution)
				}

				userBreakdowns[taskID][userID] = UserTimeContribution{
					UserID:       userID,
					CurrentTime:  formatDuration(totalDuration),
					PreviousTime: "0h 0m", // For threshold alerts, we show total vs 0
				}
			}
		}
	}

	// Convert ThresholdAlert to TaskUpdateInfo for compatibility with existing functions
	var taskInfos []TaskUpdateInfo
	for _, alert := range thresholdAlerts {
		taskInfo := TaskUpdateInfo{
			TaskID:           alert.TaskID,
			ParentID:         alert.ParentID,
			Name:             alert.Name,
			EstimationInfo:   alert.EstimationInfo,
			EstimationStatus: "",
			CurrentPeriod:    "Current",
			CurrentTime:      alert.CurrentTime,
			PreviousPeriod:   "Previous",
			PreviousTime:     alert.PreviousTime,
			DaysWorked:       0,
			Comments:         []string{}, // We could add comments here if needed
		}

		// Add user breakdown if available
		if userBreakdowns != nil {
			if breakdown, exists := userBreakdowns[alert.TaskID]; exists {
				taskInfo.UserBreakdown = breakdown
			}
		}

		taskInfos = append(taskInfos, taskInfo)
	}

	// Group by project
	projectGroups := groupTasksByTopParent(taskInfos, allTasks)

	// Format and send the alert messages
	for project, tasks := range projectGroups {
		message := formatThresholdAlertMessage(project, tasks, threshold, isNewCrossing)

		if err := sendSlackMessage(message); err != nil {
			logger.Errorf("Failed to send threshold alert for %s at %d%%: %v", project, threshold, err)
		} else {
			alertType := "new crossing"
			if !isNewCrossing {
				alertType = "persistent violation"
			}
			logger.Infof("Sent %s alert for %s: %d tasks at %d%% threshold", alertType, project, len(tasks), threshold)

			// Record the notifications as sent for these tasks
			for _, task := range tasks {
				// Find the original alert to get percentage
				var percentage float64
				for _, alert := range thresholdAlerts {
					if alert.TaskID == task.TaskID {
						percentage = alert.Percentage
						break
					}
				}

				if err := recordNotificationSent(db, task.TaskID, threshold, percentage); err != nil {
					logger.Warnf("Failed to record notification for task %d, threshold %d: %v", task.TaskID, threshold, err)
				} else {
					logger.Debugf("Recorded notification for task %d at %d%% threshold", task.TaskID, threshold)
				}
			}
		}

		// Increased delay for better visual separation between projects
		time.Sleep(1500 * time.Millisecond)
	}

	return nil
}

// formatThresholdAlertMessage formats a threshold crossing alert message
func formatThresholdAlertMessage(project string, tasks []TaskUpdateInfo, threshold int, isNewCrossing bool) SlackMessage {
	var emoji string
	var urgency string

	switch threshold {
	case 50:
		emoji = "🟡"
		urgency = "Warning"
	case 80:
		emoji = "🟠"
		urgency = "High Usage"
	case 90:
		emoji = "🔴"
		urgency = "Critical"
	case 100:
		emoji = "🚨"
		urgency = "Over Budget"
	default:
		emoji = "⚠️"
		urgency = "Alert"
	}

	var title string
	if isNewCrossing {
		title = fmt.Sprintf("%s %s: Tasks Crossed %d%% Threshold", emoji, urgency, threshold)
		if project != "Other" && project != "" {
			title = fmt.Sprintf("%s %s: %s Tasks Crossed %d%% Threshold", emoji, urgency, project, threshold)
		}
	} else {
		title = fmt.Sprintf("%s %s: Tasks Over %d%% Got Worse", emoji, urgency, threshold)
		if project != "Other" && project != "" {
			title = fmt.Sprintf("%s %s: %s Tasks Over %d%% Got Worse", emoji, urgency, project, threshold)
		}
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n", title))
	if isNewCrossing {
		messageText.WriteString(fmt.Sprintf("⏰ Detected at %s\n\n", time.Now().Format("15:04 on January 2, 2006")))
	} else {
		messageText.WriteString(fmt.Sprintf("📈 Tasks got worse with recent work at %s\n\n", time.Now().Format("15:04 on January 2, 2006")))
	}

	blocks := []Block{
		// Add spacing at the top for better separation
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: " "},
		},
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: fmt.Sprintf("⏰ Detected at %s", time.Now().Format("15:04 on January 2, 2006"))},
			},
		},
		{Type: "divider"},
	}

	for _, task := range tasks {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskTextMessage(&messageText, task)
	}

	// Add footer with action suggestion
	var suggestion string
	switch threshold {
	case 50:
		suggestion = "💡 Consider reviewing the remaining work and updating estimates if needed."
	case 80:
		suggestion = "⚡ High usage detected. Review task scope and consider breaking down into smaller tasks."
	case 90:
		suggestion = "🔍 Critical usage level. Immediate review recommended to assess if additional time is needed."
	case 100:
		suggestion = "🎯 Budget exceeded. Please review and update estimates or task scope immediately."
	}

	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Element{
			{Type: "mrkdwn", Text: suggestion},
		},
	})

	// Add spacing at the bottom for better separation
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "},
	})

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

// RunThresholdMonitoring checks for tasks that just crossed thresholds and sends alerts
func RunThresholdMonitoring() error {
	logger := GetGlobalLogger()
	logger.Debug("Starting threshold monitoring check")

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	alerts, err := CheckThresholdAlerts(db)
	if err != nil {
		return fmt.Errorf("failed to check threshold alerts: %w", err)
	}

	if len(alerts) == 0 {
		logger.Debug("No threshold crossings detected")
		return nil
	}

	logger.Infof("Detected %d threshold crossings, sending alerts", len(alerts))

	if err := SendThresholdAlerts(alerts); err != nil {
		return fmt.Errorf("failed to send threshold alerts: %w", err)
	}

	return nil
}

// hasNotificationBeenSent checks if we've already sent a notification for this task/threshold combination
func hasNotificationBeenSent(db *sql.DB, taskID int, threshold int) (bool, error) {
	query := `
		SELECT 1 
		FROM threshold_notifications 
		WHERE task_id = $1 AND threshold_percentage = $2 
		  AND last_time_entry_date = to_char(CURRENT_DATE, 'YYYY-MM-DD')
		LIMIT 1
	`

	var dummy int
	err := db.QueryRow(query, taskID, threshold).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// recordNotificationSent records that we sent a notification for this task/threshold combination
func recordNotificationSent(db *sql.DB, taskID int, threshold int, percentage float64) error {
	query := `
		INSERT INTO threshold_notifications 
		(task_id, threshold_percentage, current_percentage, last_time_entry_date)
		VALUES ($1, $2, $3, to_char(CURRENT_DATE, 'YYYY-MM-DD'))
		ON CONFLICT (task_id, threshold_percentage) 
		DO UPDATE SET 
			current_percentage = EXCLUDED.current_percentage,
			notified_at = CURRENT_TIMESTAMP,
			last_time_entry_date = EXCLUDED.last_time_entry_date
	`

	_, err := db.Exec(query, taskID, threshold, percentage)
	return err
}
