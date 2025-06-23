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
)

type TaskUpdateInfo struct {
	Name             string
	EstimationInfo   string
	EstimationStatus string
	CurrentPeriod    string
	CurrentTime      string
	PreviousPeriod   string
	PreviousTime     string
	DaysWorked       int
	Comments         []string
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

	message := formatSlackMessage(taskInfos, period)

	if asJSON {
		outputJSON(message)
		return
	}

	if responseURL != "" {
		if err := sendDelayedResponseShared(responseURL, message); err != nil {
			logger.Errorf("Failed to send delayed response: %v", err)
		}
		return
	}

	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		logger.Warn("SLACK_WEBHOOK_URL not configured. Update would contain:")
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

func formatSlackMessage(taskInfos []TaskUpdateInfo, period string) SlackMessage {
	var title string
	var date string
	switch period {
	case "daily":
		title = "📊 Daily Task Update"
		date = time.Now().Format("January 2, 2006")
	case "weekly":
		title = "📈 Weekly Task Summary"
		date = time.Now().Format("January 2, 2006")
	case "monthly":
		title = "📅 Monthly Task Summary"
		date = time.Now().Format("January 2006")
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s* - %s\n\n", title, date))

	// Header blocks
	blocks := []Block{
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

	// Group tasks by project for better organization and block efficiency
	projectGroups := groupTasksByProject(taskInfos)

	// If we have many projects, use project grouping; otherwise use individual task blocks
	const maxBlocksPerMessage = 50
	const headerBlocks = 3
	const maxProjectGroups = maxBlocksPerMessage - headerBlocks - 2 // -2 for safety and footer

	if len(projectGroups) > maxProjectGroups || len(taskInfos) > 25 {
		// Use project grouping for better space efficiency
		blocks = append(blocks, formatProjectGroupedBlocks(projectGroups, &messageText, period)...)
	} else {
		// Use individual task blocks (1 block per task instead of 3)
		for _, task := range taskInfos {
			taskBlock := formatSingleTaskBlock(task)
			blocks = append(blocks, taskBlock)

			// Also build text version
			appendTaskTextMessage(&messageText, task)
		}
	}

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

	// Time information
	taskInfo.WriteString(fmt.Sprintf("• %s: **%s** | %s: **%s**",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime))

	if task.DaysWorked > 0 {
		taskInfo.WriteString(fmt.Sprintf(" | Days: **%d**", task.DaysWorked))
	}
	taskInfo.WriteString("\n")

	// Estimation info
	if task.EstimationInfo != "" {
		estimationInfo := sanitizeSlackText(task.EstimationInfo)
		taskInfo.WriteString(fmt.Sprintf("• %s\n", estimationInfo))
	}

	// Comments (limit to save space)
	if len(task.Comments) > 0 {
		taskInfo.WriteString("• Recent: ")
		if len(task.Comments) == 1 {
			comment := sanitizeSlackText(task.Comments[0])
			if len(comment) > 80 {
				comment = comment[:77] + "..."
			}
			taskInfo.WriteString(fmt.Sprintf("_%s_", comment))
		} else {
			taskInfo.WriteString(fmt.Sprintf("_%d comments_", len(task.Comments)))
		}
		taskInfo.WriteString("\n")
	}

	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: taskInfo.String()},
	}
}

// groupTasksByProject groups tasks by their project identifier (e.g., WP3DX, BNBS, etc.)
func groupTasksByProject(tasks []TaskUpdateInfo) map[string][]TaskUpdateInfo {
	projects := make(map[string][]TaskUpdateInfo)

	for _, task := range tasks {
		project := extractProjectCode(task.Name)
		projects[project] = append(projects[project], task)
	}

	return projects
}

// extractProjectCode extracts project code from task name (e.g., "WP3DX" from "[WP3DX-652] Task name")
func extractProjectCode(taskName string) string {
	// Look for pattern like [ABC-123] or [ABCD-123]
	re := regexp.MustCompile(`\[([A-Z]+)[-\d]`)
	matches := re.FindStringSubmatch(taskName)

	if len(matches) > 1 {
		return matches[1]
	}

	// If no project code found, group under "Other"
	return "Other"
}

// formatProjectGroupedBlocks formats tasks grouped by project
func formatProjectGroupedBlocks(projectGroups map[string][]TaskUpdateInfo, messageText *strings.Builder, period string) []Block {
	var blocks []Block

	// Sort project names for consistent output
	var projectNames []string
	for project := range projectGroups {
		projectNames = append(projectNames, project)
	}

	// Sort projects, but put "Other" last
	sort.Slice(projectNames, func(i, j int) bool {
		if projectNames[i] == "Other" {
			return false
		}
		if projectNames[j] == "Other" {
			return true
		}
		return projectNames[i] < projectNames[j]
	})

	for _, project := range projectNames {
		tasks := projectGroups[project]

		var projectInfo strings.Builder
		if project == "Other" {
			projectInfo.WriteString("*📋 Other Tasks*\n\n")
		} else {
			projectInfo.WriteString(fmt.Sprintf("*📁 %s Project*\n\n", project))
		}

		for _, task := range tasks {
			taskName := sanitizeSlackText(task.Name)
			if len(taskName) > 70 {
				taskName = taskName[:67] + "..."
			}

			projectInfo.WriteString(fmt.Sprintf("• *%s*\n", taskName))
			projectInfo.WriteString(fmt.Sprintf("  %s: %s | %s: %s",
				task.CurrentPeriod, task.CurrentTime,
				task.PreviousPeriod, task.PreviousTime))

			if task.DaysWorked > 0 {
				projectInfo.WriteString(fmt.Sprintf(" | %d days", task.DaysWorked))
			}

			if task.EstimationInfo != "" {
				estimationInfo := sanitizeSlackText(task.EstimationInfo)
				if len(estimationInfo) > 25 {
					estimationInfo = estimationInfo[:22] + "..."
				}
				projectInfo.WriteString(fmt.Sprintf(" | %s", estimationInfo))
			}
			projectInfo.WriteString("\n\n")

			// Also build text version
			appendTaskTextMessage(messageText, task)
		}

		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: projectInfo.String()},
		})
	}

	// Add summary
	totalTasks := 0
	for _, tasks := range projectGroups {
		totalTasks += len(tasks)
	}

	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Element{
			{Type: "mrkdwn", Text: fmt.Sprintf("📊 _Total: %d tasks across %d projects_", totalTasks, len(projectGroups))},
		},
	})

	return blocks
}

func appendTaskTextMessage(builder *strings.Builder, task TaskUpdateInfo) {
	builder.WriteString(fmt.Sprintf("*%s*", task.Name))
	if task.EstimationInfo != "" {
		builder.WriteString(fmt.Sprintf(" | %s", task.EstimationInfo))
	}
	builder.WriteString(fmt.Sprintf("\nTime worked: %s: %s, %s: %s", task.CurrentPeriod, task.CurrentTime, task.PreviousPeriod, task.PreviousTime))
	if task.DaysWorked > 0 {
		builder.WriteString(fmt.Sprintf(", Days worked: %d", task.DaysWorked))
	}
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
	if len(text) > 2000 {
		text = text[:1997] + "..."
	}

	return text
}

func formatTaskBlock(task TaskUpdateInfo) []Block {
	// Sanitize task name to prevent Slack block issues
	taskName := sanitizeSlackText(task.Name)

	var fields []Field
	fields = append(fields, Field{Type: "mrkdwn", Text: fmt.Sprintf("*%s:* %s", task.CurrentPeriod, task.CurrentTime)})
	fields = append(fields, Field{Type: "mrkdwn", Text: fmt.Sprintf("*%s:* %s", task.PreviousPeriod, task.PreviousTime)})

	if task.DaysWorked > 0 {
		fields = append(fields, Field{Type: "mrkdwn", Text: fmt.Sprintf("*Days Worked:*\n%d", task.DaysWorked)})
	}

	if task.EstimationInfo != "" {
		// Sanitize estimation info as well
		estimationInfo := sanitizeSlackText(task.EstimationInfo)
		fields = append(fields, Field{Type: "mrkdwn", Text: fmt.Sprintf("*Estimation:*\n%s", estimationInfo)})
	}

	if len(task.Comments) > 0 {
		// Sanitize and limit comments
		var sanitizedComments []string
		for _, comment := range task.Comments {
			sanitizedComment := sanitizeSlackText(comment)
			if len(sanitizedComment) > 200 { // Limit individual comments
				sanitizedComment = sanitizedComment[:197] + "..."
			}
			sanitizedComments = append(sanitizedComments, sanitizedComment)
		}
		// Limit total number of comments displayed
		if len(sanitizedComments) > 3 {
			sanitizedComments = sanitizedComments[:3]
			sanitizedComments = append(sanitizedComments, fmt.Sprintf("... and %d more comments", len(task.Comments)-3))
		}
		fields = append(fields, Field{Type: "mrkdwn", Text: fmt.Sprintf("*Recent Comments:*\n%s", strings.Join(sanitizedComments, "\n"))})
	}

	return []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*", taskName)},
		},
		{
			Type:   "section",
			Fields: fields,
		},
		{Type: "divider"},
	}
}

func sendNoChangesNotification(period, responseURL string, asJSON bool) error {
	message := SlackMessage{
		Text: fmt.Sprintf("No task changes to report for %s.", period),
	}
	if asJSON {
		outputJSON(message)
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

func outputJSON(message SlackMessage) {
	json.NewEncoder(os.Stdout).Encode(message)
}

func parseEstimation(taskName string) (string, string) {
	logger := GetGlobalLogger()

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
	re := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	matches := re.FindStringSubmatch(estimation)

	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("no estimation pattern found")
	}

	pessimistic, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse pessimistic estimation: %w", err)
	}

	currentSeconds := parseTimeToSeconds(currentTime)
	previousSeconds := parseTimeToSeconds(previousTime)
	totalSeconds := currentSeconds + previousSeconds

	pessimisticSeconds := pessimistic * 3600

	if pessimisticSeconds == 0 {
		return 0, totalSeconds, nil
	}

	percentage := (float64(totalSeconds) / float64(pessimisticSeconds)) * 100
	return percentage, totalSeconds, nil
}
