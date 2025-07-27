package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// NewSlackAPIClient creates a new Slack API client
func NewSlackAPIClient() *SlackAPIClient {
	return &SlackAPIClient{
		botToken: os.Getenv("SLACK_BOT_TOKEN"),
		logger:   GetGlobalLogger(),
	}
}

// SendSlackUpdate sends task updates via webhook
func SendSlackUpdate(taskInfos []TaskUpdateInfo, period string) error {
	logger := GetGlobalLogger()
	if len(taskInfos) == 0 {
		logger.Info("No changes to report")
		return nil
	}

	// Convert to TaskInfo for new formatting
	convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)

	// Send using new simplified messaging system
	return SendTaskMessage(convertedTasks, period)
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

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("error reading response body: %w", readErr)
	}

	var slackResp SlackAPIResponse
	if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
		s.logger.Errorf("Error decoding Slack API response for %s: %v", endpoint, err)
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if !slackResp.OK {
		s.logger.Errorf("Slack API error for %s - Error: %s", endpoint, slackResp.Error)
		return nil, fmt.Errorf("slack API error: %s", slackResp.Error)
	}

	return &slackResp, nil
}

// SendErrorResponse sends an error message to the user
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

// SendNoChangesMessage sends a message when there are no changes
func (s *SlackAPIClient) SendNoChangesMessage(ctx *ConversationContext, period string) error {
	message := fmt.Sprintf("📊 No task changes to report for your %s update! 🎉", period)

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

// SendProgressMessage sends a progress message and returns response for threading
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

	return s.sendSlackAPIRequestWithResponse("chat.postMessage", payload)
}

// UpdateProgress updates progress in existing thread
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

// SendFinalUpdate sends final update with full formatting
func (s *SlackAPIClient) SendFinalUpdate(ctx *ConversationContext, taskInfos []TaskUpdateInfo, period string) error {
	if len(taskInfos) == 0 {
		return s.SendNoChangesMessage(ctx, period)
	}

	// Convert to TaskInfo and format using simplified system
	convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
	message := formatProjectMessage("Update", convertedTasks)

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

// SendThresholdNoResultsMessage sends a message when no tasks are over threshold
func (s *SlackAPIClient) SendThresholdNoResultsMessage(ctx *ConversationContext, threshold float64, period string) error {
	message := fmt.Sprintf("🎯 No tasks found over %.0f%% threshold for %s period", threshold, period)

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message,
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	return s.sendSlackAPIRequest("chat.postMessage", payload)
}

// SendThresholdResults sends the results of a threshold query
func (s *SlackAPIClient) SendThresholdResults(ctx *ConversationContext, taskInfos []TaskUpdateInfo, threshold float64, period string) error {
	if len(taskInfos) == 0 {
		return s.SendThresholdNoResultsMessage(ctx, threshold, period)
	}

	// Convert to TaskInfo and format using simplified system
	convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
	message := formatThresholdMessage("Threshold Alert", convertedTasks, period, threshold)

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

// Helper function to convert TaskUpdateInfo to TaskInfo (reused from smart_router.go)
func convertTaskUpdateInfoToTaskInfo(taskUpdates []TaskUpdateInfo) []TaskInfo {
	var tasks []TaskInfo
	for _, task := range taskUpdates {
		taskInfo := TaskInfo{
			TaskID:         task.TaskID,
			ParentID:       task.ParentID,
			Name:           task.Name,
			EstimationInfo: ParseTaskEstimationWithUsage(task.Name, task.CurrentTime, task.PreviousTime),
			CurrentPeriod:  task.CurrentPeriod,
			CurrentTime:    task.CurrentTime,
			PreviousPeriod: task.PreviousPeriod,
			PreviousTime:   task.PreviousTime,
			DaysWorked:     task.DaysWorked,
			Comments:       task.Comments,
		}
		tasks = append(tasks, taskInfo)
	}
	return tasks
}
