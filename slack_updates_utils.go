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
	UserBreakdown    map[int]UserTimeContribution
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
	Type      string     `json:"type"`
	Text      *Text      `json:"text,omitempty"`
	Fields    []Field    `json:"fields,omitempty"`
	Elements  []Element  `json:"elements,omitempty"`
	Accessory *Accessory `json:"accessory,omitempty"`
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
	Type string `json:"type"`
	Text string `json:"text"`
}

// Accessory represents an accessory in Slack blocks
type Accessory struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
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
	switch period {
	case "daily":
		return GetTaskTimeEntries(db)
	case "weekly":
		return GetWeeklyTaskTimeEntries(db)
	case "monthly":
		return GetMonthlyTaskTimeEntries(db)
	default:
		return nil, fmt.Errorf("invalid period: %s", period)
	}
}

func getTaskChangesWithProject(db *sql.DB, period string, projectTaskID *int) ([]TaskUpdateInfo, error) {
	switch period {
	case "daily":
		return GetTaskTimeEntriesWithProject(db, projectTaskID)
	case "weekly":
		return GetWeeklyTaskTimeEntriesWithProject(db, projectTaskID)
	case "monthly":
		return GetMonthlyTaskTimeEntriesWithProject(db, projectTaskID)
	default:
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

	blocks := []Block{
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

	for _, task := range tasks {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)
		appendTaskTextMessage(&messageText, task)
	}

	// Add spacing at the bottom for better separation between projects
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "},
	})

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
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
		Text: fmt.Sprintf("⚠️ System Alert: %s failed"),
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

	// Create the main message
	mainMessage := formatProjectMessage(project, tasks, period)
	messages = append(messages, mainMessage)

	// Check if any tasks have comments that were truncated and need additional messages
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
		// Split additional blocks into separate messages to respect the 50-block limit
		const maxBlocksPerMessage = 47 // Leave buffer for header blocks

		for i := 0; i < len(additionalBlocks); i += maxBlocksPerMessage {
			end := i + maxBlocksPerMessage
			if end > len(additionalBlocks) {
				end = len(additionalBlocks)
			}

			blockChunk := additionalBlocks[i:end]

			// Create message header
			messageBlocks := []Block{
				{
					Type: "section",
					Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("📝 *Additional Comments for %s* (Part %d)", project, (i/maxBlocksPerMessage)+1)},
				},
				{Type: "divider"},
			}

			messageBlocks = append(messageBlocks, blockChunk...)

			additionalMessage := SlackMessage{
				Text:   fmt.Sprintf("Additional Comments for %s", project),
				Blocks: messageBlocks,
			}

			messages = append(messages, additionalMessage)
		}
	}

	return messages
}

// GetTasksOverThreshold returns tasks that are over a specific percentage of their estimation
func GetTasksOverThreshold(db *sql.DB, threshold float64, period string) ([]TaskUpdateInfo, error) {
	logger := GetGlobalLogger()

	var fromDate, toDate string
	switch period {
	case "daily":
		fromDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		toDate = time.Now().Format("2006-01-02")
	case "weekly":
		fromDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
		toDate = time.Now().Format("2006-01-02")
	case "monthly":
		fromDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
		toDate = time.Now().Format("2006-01-02")
	default:
		// Default to daily
		fromDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		toDate = time.Now().Format("2006-01-02")
	}

	// First get user breakdown data for all tasks with estimations
	userBreakdownQuery := `
		SELECT 
			t.task_id,
			te.user_id,
			COALESCE(SUM(CASE WHEN te.date BETWEEN $1 AND $2 THEN te.duration ELSE 0 END), 0) as current_duration,
			COALESCE(SUM(CASE WHEN te.date < $1 THEN te.duration ELSE 0 END), 0) as previous_duration
		FROM tasks t
		INNER JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'  -- Only tasks with estimation patterns
		GROUP BY t.task_id, te.user_id
		HAVING COALESCE(SUM(te.duration), 0) > 0  -- Only users with time logged
	`

	userRows, err := db.Query(userBreakdownQuery, fromDate, toDate)
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

	// Main query - get all tasks with estimations and let Go handle the threshold logic
	query := `
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE WHEN te.date BETWEEN $1 AND $2 THEN te.duration ELSE 0 END), 0) as current_duration,
			COALESCE(SUM(CASE WHEN te.date < $1 THEN te.duration ELSE 0 END), 0) as previous_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'  -- Only tasks with estimation patterns
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(te.duration), 0) > 0  -- Only tasks with time logged
		ORDER BY t.task_id
	`

	rows, err := db.Query(query, fromDate, toDate)
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

// CheckThresholdAlerts checks for tasks that just crossed specific thresholds
func CheckThresholdAlerts(db *sql.DB) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	// Define the thresholds we want to monitor
	thresholds := []int{50, 80, 90, 100}
	var allAlerts []ThresholdAlert

	// Get current time for comparison
	now := time.Now()
	fifteenMinutesAgo := now.Add(-15 * time.Minute)

	for _, threshold := range thresholds {
		alerts, err := getTasksJustCrossedThreshold(db, float64(threshold), fifteenMinutesAgo)
		if err != nil {
			logger.Errorf("Failed to check %d%% threshold: %v", threshold, err)
			continue
		}
		allAlerts = append(allAlerts, alerts...)
	}

	return allAlerts, nil
}

// getTasksJustCrossedThreshold finds tasks that just crossed a threshold in the last 15 minutes
func getTasksJustCrossedThreshold(db *sql.DB, threshold float64, since time.Time) ([]ThresholdAlert, error) {
	logger := GetGlobalLogger()

	// Simplified query - get tasks with recent activity and let Go handle the threshold logic
	query := `
		WITH recent_entries AS (
			-- Get time entries from the last 15 minutes
			SELECT task_id, duration
			FROM time_entries 
			WHERE modify_time >= $1
			  AND duration > 0
		),
		tasks_with_recent_activity AS (
			-- Get tasks that had recent time entries
			SELECT DISTINCT t.task_id, t.parent_id, t.name
			FROM tasks t
			INNER JOIN recent_entries re ON t.task_id = re.task_id
			WHERE t.name ~ '\[([0-9]+(?:[.,][0-9]+)?h?[-+][0-9]+(?:[.,][0-9]+)?h?|[0-9]+(?:[.,][0-9]+)?h?)\]'  -- Only tasks with estimation patterns
		),
		task_totals AS (
			-- Calculate total time and recent time for these tasks
			SELECT 
				tra.task_id,
				tra.parent_id,
				tra.name,
				COALESCE(SUM(te.duration), 0) as total_duration,
				COALESCE(SUM(CASE WHEN te.modify_time >= $1 THEN te.duration ELSE 0 END), 0) as recent_duration
			FROM tasks_with_recent_activity tra
			LEFT JOIN time_entries te ON tra.task_id = te.task_id
			GROUP BY tra.task_id, tra.parent_id, tra.name
		)
		SELECT 
			task_id,
			parent_id,
			name,
			total_duration,
			recent_duration
		FROM task_totals
		WHERE total_duration > 0
	`

	rows, err := db.Query(query, since)
	if err != nil {
		return nil, fmt.Errorf("could not query threshold crossings: %w", err)
	}
	defer rows.Close()

	var alerts []ThresholdAlert
	var alertTaskIDs []int
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

		// Check if this task just crossed the threshold
		if previousPercentage < threshold && currentPercentage >= threshold {
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
		}
	}

	if len(alerts) > 0 {
		logger.Infof("Found %d tasks that just crossed %.1f%% threshold", len(alerts), threshold)
	}

	return alerts, nil
}

// SendThresholdAlerts sends Slack notifications for threshold crossings
func SendThresholdAlerts(alerts []ThresholdAlert) error {
	if len(alerts) == 0 {
		return nil
	}

	logger := GetGlobalLogger()

	// Group alerts by threshold
	thresholdGroups := make(map[int][]ThresholdAlert)
	for _, alert := range alerts {
		thresholdGroups[alert.ThresholdCrossed] = append(thresholdGroups[alert.ThresholdCrossed], alert)
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

	// Send a message for each threshold that was crossed
	for threshold, thresholdAlerts := range thresholdGroups {
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

		// Format the alert message
		for project, tasks := range projectGroups {
			message := formatThresholdAlertMessage(project, tasks, threshold)

			if err := sendSlackMessage(message); err != nil {
				logger.Errorf("Failed to send threshold alert for %s at %d%%: %v", project, threshold, err)
			} else {
				logger.Infof("Sent threshold alert for %s: %d tasks crossed %d%% threshold", project, len(tasks), threshold)
			}

			// Increased delay for better visual separation between projects
			time.Sleep(1500 * time.Millisecond)
		}
	}

	return nil
}

// formatThresholdAlertMessage formats a threshold crossing alert message
func formatThresholdAlertMessage(project string, tasks []TaskUpdateInfo, threshold int) SlackMessage {
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

	title := fmt.Sprintf("%s %s: Tasks Crossed %d%% Threshold", emoji, urgency, threshold)
	if project != "Other" && project != "" {
		title = fmt.Sprintf("%s %s: %s Tasks Crossed %d%% Threshold", emoji, urgency, project, threshold)
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n", title))
	messageText.WriteString(fmt.Sprintf("⏰ Detected at %s\n\n", time.Now().Format("15:04 on January 2, 2006")))

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
