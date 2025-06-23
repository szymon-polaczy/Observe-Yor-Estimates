package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type SlackAPIClient struct {
	botToken string
	logger   *Logger
}

type ConversationContext struct {
	ChannelID   string
	UserID      string
	ThreadTS    string
	CommandType string
}

type SlackAPIResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"ts,omitempty"`
	Channel   string `json:"channel,omitempty"`
}

type Option struct {
	Text  *Text  `json:"text"`
	Value string `json:"value"`
}

func NewSlackAPIClient() *SlackAPIClient {
	return &SlackAPIClient{
		botToken: os.Getenv("SLACK_BOT_TOKEN"),
		logger:   GetGlobalLogger(),
	}
}

// NewSlackAPIClientFromEnv creates a client using CLI environment variables
func NewSlackAPIClientFromEnv() *SlackAPIClient {
	client := NewSlackAPIClient()

	// When called from CLI, create context from environment variables
	if channelID := os.Getenv("CHANNEL_ID"); channelID != "" {
		client.logger.Debugf("CLI mode: Using channel %s", channelID)
	}

	return client
}

// GetContextFromEnv creates conversation context from environment variables (for CLI usage)
func GetContextFromEnv() *ConversationContext {
	return &ConversationContext{
		ChannelID:   os.Getenv("CHANNEL_ID"),
		UserID:      os.Getenv("USER_ID"),
		ThreadTS:    os.Getenv("THREAD_TS"),
		CommandType: os.Getenv("COMMAND_TYPE"),
	}
}

// Send message directly to the channel where user asked
func (s *SlackAPIClient) SendContextualUpdate(ctx *ConversationContext, taskInfos []TaskUpdateInfo, period string) error {
	message := s.formatContextualMessage(taskInfos, period, ctx.UserID)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message.Text,
		"blocks":  message.Blocks,
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

// Send ephemeral message only visible to the requesting user
func (s *SlackAPIClient) SendPersonalUpdate(ctx *ConversationContext, taskInfos []TaskUpdateInfo, period string) error {
	message := s.formatPersonalMessage(taskInfos, period, ctx.UserID)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"user":    ctx.UserID,
		"text":    message.Text,
		"blocks":  message.Blocks,
	}

	return s.sendSlackAPIRequest("chat.postEphemeral", payload)
}

// SendPersonalUpdateInThread sends a personal update in thread since ephemeral messages don't support threading
func (s *SlackAPIClient) SendPersonalUpdateInThread(ctx *ConversationContext, taskInfos []TaskUpdateInfo, period string) error {
	// Create a message that's addressed to the specific user but sent as a regular message in thread
	message := s.formatPersonalThreadMessage(taskInfos, period, ctx.UserID)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message.Text,
		"blocks":  message.Blocks,
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

// Send progress message and return response for threading
func (s *SlackAPIClient) SendProgressMessage(ctx *ConversationContext, message string) (*SlackAPIResponse, error) {
	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message,
		"blocks": []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: message},
			},
		},
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	resp, err := s.sendSlackAPIRequestWithResponse("chat.postMessage", payload)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Update progress in existing thread
func (s *SlackAPIClient) UpdateProgress(ctx *ConversationContext, message string) error {
	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message,
		"blocks": []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("_%s_", message)},
			},
		},
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

// Send final update with full formatting
func (s *SlackAPIClient) SendFinalUpdate(ctx *ConversationContext, taskInfos []TaskUpdateInfo, period string) error {
	if len(taskInfos) == 0 {
		return s.SendNoChangesMessage(ctx, period)
	}

	// Check if we need to split by project due to size limits
	projectGroups := groupTasksByProject(taskInfos)

	// If we have many projects or tasks, send multiple messages (one per project)
	if len(projectGroups) > 15 || len(taskInfos) > 25 {
		return s.sendProjectSplitMessages(ctx, projectGroups, period, taskInfos)
	}

	// Otherwise, send as single message
	message := s.formatContextualMessage(taskInfos, period, ctx.UserID)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message.Text,
		"blocks":  message.Blocks,
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

// sendProjectSplitMessages sends separate messages for each project to avoid limits
func (s *SlackAPIClient) sendProjectSplitMessages(ctx *ConversationContext, projectGroups map[string][]TaskUpdateInfo, period string, allTasks []TaskUpdateInfo) error {
	// Send header message first
	headerMessage := s.formatReportHeaderMessage(period, ctx.UserID, len(allTasks), len(projectGroups))

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    headerMessage.Text,
		"blocks":  headerMessage.Blocks,
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	err := s.sendSlackAPIRequest("chat.postMessage", payload)
	if err != nil {
		return fmt.Errorf("failed to send header message: %w", err)
	}

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

	// Send one message per project
	for i, project := range projectNames {
		tasks := projectGroups[project]

		projectMessage := s.formatSingleProjectMessage(project, tasks, period, i+1, len(projectNames))

		payload := map[string]interface{}{
			"channel": ctx.ChannelID,
			"text":    projectMessage.Text,
			"blocks":  projectMessage.Blocks,
		}

		if ctx.ThreadTS != "" {
			payload["thread_ts"] = ctx.ThreadTS
		}

		err := s.sendSlackAPIRequest("chat.postMessage", payload)
		if err != nil {
			s.logger.Errorf("Failed to send project message for %s: %v", project, err)
			// Continue with other projects even if one fails
		}

		// Small delay between messages to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// formatReportHeaderMessage creates the header message for split reports
func (s *SlackAPIClient) formatReportHeaderMessage(period, userID string, totalTasks, totalProjects int) SlackMessage {
	headerText := fmt.Sprintf("📊 %s Task Update for <@%s>", strings.Title(period), userID)

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: fmt.Sprintf("%s Task Update", strings.Title(period))},
		},
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*\n📋 **%d tasks** across **%d projects**\n_Report split by project for better readability_", headerText, totalTasks, totalProjects)},
		},
		{Type: "divider"},
	}

	return SlackMessage{
		Text:   fmt.Sprintf("%s - %d tasks across %d projects", headerText, totalTasks, totalProjects),
		Blocks: blocks,
	}
}

// formatSingleProjectMessage creates a message for a single project
func (s *SlackAPIClient) formatSingleProjectMessage(project string, tasks []TaskUpdateInfo, period string, projectNum, totalProjects int) SlackMessage {
	var projectTitle string
	if project == "Other" {
		projectTitle = "📋 Other Tasks"
	} else {
		projectTitle = fmt.Sprintf("📁 %s Project", project)
	}

	headerText := fmt.Sprintf("%s (%d/%d)", projectTitle, projectNum, totalProjects)

	blocks := []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*\n_%d tasks in this project_", headerText, len(tasks))},
		},
	}

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n\n", headerText))

	// Use detailed format
	for _, task := range tasks {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)

		// Build text version
		appendTaskTextMessage(&messageText, task)
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

func (s *SlackAPIClient) SendNoChangesMessage(ctx *ConversationContext, period string) error {
	message := fmt.Sprintf("📊 No task changes to report for your %s update, <@%s>! 🎉", period, ctx.UserID)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message,
		"blocks": []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: message},
			},
		},
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

func (s *SlackAPIClient) SendErrorResponse(ctx *ConversationContext, errorMsg string) error {
	message := fmt.Sprintf("❌ %s", errorMsg)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"user":    ctx.UserID,
		"text":    message,
		"blocks": []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: message},
			},
		},
	}

	return s.sendSlackAPIRequest("chat.postEphemeral", payload)
}

func (s *SlackAPIClient) formatContextualMessage(taskInfos []TaskUpdateInfo, period string, userID string) SlackMessage {
	headerText := fmt.Sprintf("📊 %s Task Update for <@%s>", strings.Title(period), userID)

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n\n", headerText))

	blocks := []Block{
		{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: fmt.Sprintf("%s Task Update", strings.Title(period))},
		},
		{
			Type: "context",
			Elements: []Element{
				{Type: "mrkdwn", Text: fmt.Sprintf("Requested by <@%s> at %s", userID, time.Now().Format("3:04 PM"))},
			},
		},
		{Type: "divider"},
	}

	// Use individual task blocks
	for _, task := range taskInfos {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)

		// Build text version
		appendTaskTextMessage(&messageText, task)
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

func (s *SlackAPIClient) formatPersonalMessage(taskInfos []TaskUpdateInfo, period string, userID string) SlackMessage {
	headerText := fmt.Sprintf("📊 Your %s task update", period)

	// Simplified blocks for ephemeral messages - avoid unsupported block types
	blocks := []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*\n_This update is only visible to you_", headerText)},
		},
		{Type: "divider"},
	}

	// Add task information using only supported block types for ephemeral messages
	for _, task := range taskInfos {
		// Use simplified task blocks for ephemeral messages
		taskBlock := s.formatSimpleTaskBlock(task)
		blocks = append(blocks, taskBlock...)
	}

	// Don't add action buttons for ephemeral messages as they're not supported
	// Instead, add a simple text section with instructions
	if len(taskInfos) > 0 {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: "_💡 Tip: Use `/oye config public` to share updates with the channel_"},
		})
	}

	return SlackMessage{
		Text:   headerText,
		Blocks: blocks,
	}
}

// formatSimpleTaskBlock creates simplified task blocks compatible with ephemeral messages
func (s *SlackAPIClient) formatSimpleTaskBlock(task TaskUpdateInfo) []Block {
	// Sanitize task name to prevent Slack block issues
	taskName := sanitizeSlackText(task.Name)

	// Create a simple text-based representation for ephemeral messages
	var taskInfo strings.Builder
	taskInfo.WriteString(fmt.Sprintf("*%s*\n", taskName))
	taskInfo.WriteString(fmt.Sprintf("• %s: %s\n", task.CurrentPeriod, task.CurrentTime))
	taskInfo.WriteString(fmt.Sprintf("• %s: %s\n", task.PreviousPeriod, task.PreviousTime))

	if task.DaysWorked > 0 {
		taskInfo.WriteString(fmt.Sprintf("• Days Worked: %d\n", task.DaysWorked))
	}

	if task.EstimationInfo != "" {
		estimationInfo := sanitizeSlackText(task.EstimationInfo)
		taskInfo.WriteString(fmt.Sprintf("• %s\n", estimationInfo))
	}

	if len(task.Comments) > 0 {
		taskInfo.WriteString("• Recent Comments:\n")
		// Limit comments for ephemeral messages
		commentCount := len(task.Comments)
		if commentCount > 2 {
			commentCount = 2
		}
		for i := 0; i < commentCount; i++ {
			comment := sanitizeSlackText(task.Comments[i])
			if len(comment) > 100 {
				comment = comment[:97] + "..."
			}
			taskInfo.WriteString(fmt.Sprintf("  - %s\n", comment))
		}
		if len(task.Comments) > 2 {
			taskInfo.WriteString(fmt.Sprintf("  - ... and %d more comments\n", len(task.Comments)-2))
		}
	}

	return []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: taskInfo.String()},
		},
		{Type: "divider"},
	}
}

// formatPersonalThreadMessage creates a message for personal updates that can be sent in thread
func (s *SlackAPIClient) formatPersonalThreadMessage(taskInfos []TaskUpdateInfo, period string, userID string) SlackMessage {
	headerText := fmt.Sprintf("📊 %s Task Update for <@%s> (Personal Report)", strings.Title(period), userID)

	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("*%s*\n\n", headerText))

	blocks := []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("*%s*\n_This is <@%s>'s personal update_", headerText, userID)},
		},
		{Type: "divider"},
	}

	// Use individual task blocks
	for _, task := range taskInfos {
		taskBlock := formatSingleTaskBlock(task)
		blocks = append(blocks, taskBlock)

		// Also build text version
		appendTaskTextMessage(&messageText, task)
	}

	return SlackMessage{
		Text:   messageText.String(),
		Blocks: blocks,
	}
}

func (s *SlackAPIClient) sendSlackAPIRequest(endpoint string, payload map[string]interface{}) error {
	_, err := s.sendSlackAPIRequestWithResponse(endpoint, payload)
	return err
}

func (s *SlackAPIClient) sendSlackAPIRequestWithResponse(endpoint string, payload map[string]interface{}) (*SlackAPIResponse, error) {
	if s.botToken == "" {
		s.logger.Warn("SLACK_BOT_TOKEN not configured, cannot send direct API requests")
		return nil, fmt.Errorf("slack bot token not configured")
	}

	url := fmt.Sprintf("https://slack.com/api/%s", endpoint)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling payload: %w", err)
	}

	// Log the JSON payload for debugging (first 500 chars to avoid spam)
	jsonStr := string(jsonData)
	if len(jsonStr) > 500 {
		s.logger.Debugf("Sending %s payload (truncated): %s...", endpoint, jsonStr[:500])
	} else {
		s.logger.Debugf("Sending %s payload: %s", endpoint, jsonStr)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.botToken))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read the raw response body for detailed logging
	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("error reading response body: %w", readErr)
	}

	var slackResp SlackAPIResponse
	if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
		s.logger.Errorf("Error decoding Slack API response for %s: %v", endpoint, err)
		s.logger.Errorf("Raw response body: %s", string(bodyBytes))
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if !slackResp.OK {
		s.logger.Errorf("Slack API error for %s - Error: %s, Full response: %s", endpoint, slackResp.Error, string(bodyBytes))
		return nil, fmt.Errorf("slack API error: %s", slackResp.Error)
	}

	s.logger.Debugf("Successfully sent %s request to Slack API", endpoint)
	return &slackResp, nil
}
