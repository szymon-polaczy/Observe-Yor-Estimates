package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SmartRouter struct {
	slackClient *SlackAPIClient
	logger      *Logger
}

func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		slackClient: NewSlackAPIClient(),
		logger:      GetGlobalLogger(),
	}
}

// HandleUpdateRequest processes update requests with intelligent routing
func (sr *SmartRouter) HandleUpdateRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
		ProjectName: req.ProjectName, // Add project context
	}

	// Determine period from command
	period := sr.determinePeriod(req.Text, req.Command)

	if req.ProjectName != "" {
		sr.logger.Infof("Processing %s update request for project '%s' from user %s in channel %s", 
			period, req.ProjectName, req.UserName, req.ChannelName)
	} else {
		sr.logger.Infof("Processing %s update request from user %s in channel %s", period, req.UserName, req.ChannelName)
	}

	// Handle long-running updates with progress tracking
	return sr.HandleLongRunningUpdate(ctx, period)
}

// HandleLongRunningUpdate shows progress and delivers final result
func (sr *SmartRouter) HandleLongRunningUpdate(ctx *ConversationContext, period string) error {
	// Send initial progress message
	progressResp, err := sr.slackClient.SendProgressMessage(ctx,
		fmt.Sprintf("🔄 Generating your %s update...", period))
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to start update process")
	}

	// Update context with message timestamp for threading
	if progressResp != nil {
		ctx.ThreadTS = progressResp.Timestamp
	}

	// Process in background
	go func() {
		sr.processUpdateWithProgress(ctx, period)
	}()

	return nil
}

func (sr *SmartRouter) processUpdateWithProgress(ctx *ConversationContext, period string) {
	// Show progress updates
	if ctx.ProjectName != "" {
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📊 Querying database for project '%s'...", ctx.ProjectName))
	} else {
		sr.slackClient.UpdateProgress(ctx, "📊 Querying database...")
	}
	time.Sleep(500 * time.Millisecond)

	// Get database connection
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	var taskInfos []TaskUpdateInfo
	
	if ctx.ProjectName != "" && ctx.ProjectName != "all" {
		// Project-specific query
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("🔍 Finding project '%s'...", ctx.ProjectName))
		time.Sleep(500 * time.Millisecond)
		
		// Find the project
		projects, err := FindProjectsByName(db, ctx.ProjectName)
		if err != nil {
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to find project: %v", err))
			return
		}
		
		if len(projects) == 0 {
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Project '%s' not found. Use `/oye help` to see available commands.", ctx.ProjectName))
			return
		}
		
		if len(projects) > 1 {
			// Multiple matches found, suggest more specific search
			projectNames := make([]string, len(projects))
			for i, p := range projects {
				projectNames[i] = p.Name
			}
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Multiple projects found matching '%s': %s. Please be more specific.", ctx.ProjectName, strings.Join(projectNames, ", ")))
			return
		}
		
		project := projects[0]
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📈 Analyzing time entries for '%s'...", project.Name))
		time.Sleep(1 * time.Second)
		
		// Get project-specific task data
		taskInfos, err = getTaskChangesWithProject(db, period, &project.TimeCampTaskID)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to get %s changes for project '%s': ```%v```", period, project.Name, err)
			sr.slackClient.SendErrorResponse(ctx, errorMessage)
			return
		}
	} else {
		// All projects query
		sr.slackClient.UpdateProgress(ctx, "📈 Analyzing time entries...")
		time.Sleep(1 * time.Second)
		
		// Get task data for all projects
		taskInfos, err = getTaskChanges(db, period)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to get %s changes: ```%v```", period, err)
			sr.slackClient.SendErrorResponse(ctx, errorMessage)
			return
		}
	}

	sr.slackClient.UpdateProgress(ctx, "✍️ Formatting report...")
	time.Sleep(500 * time.Millisecond)

	// Handle the case where there are no tasks
	if len(taskInfos) == 0 {
		err = sr.slackClient.SendNoChangesMessage(ctx, period)
		if err != nil {
			sr.logger.Errorf("Failed to send 'no changes' message: %v", err)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", period))
		}
		return
	}

	// Send final result as public message in thread
	err = sr.slackClient.SendFinalUpdate(ctx, taskInfos, period)

	if err != nil {
		sr.logger.Errorf("Failed to send final %s report via Slack API: %v", period, err)

		// Try webhook fallback
		message := sr.slackClient.formatContextualMessage(taskInfos, period, ctx.UserID)
		if webhookErr := sendSlackMessage(message); webhookErr != nil {
			sr.logger.Errorf("Webhook fallback also failed: %v", webhookErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", period))
			return
		}
		sr.logger.Info("Successfully sent report via webhook fallback")
	}

	sr.logger.Infof("Completed %s update for user %s", period, ctx.UserID)
}

// HandleFullSyncRequest processes full sync requests
func (sr *SmartRouter) HandleFullSyncRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
	}

	sr.logger.Infof("Processing full sync request from user %s", req.UserName)

	// Send initial progress message
	progressResp, err := sr.slackClient.SendProgressMessage(ctx, "🚀 Starting full data synchronization...")
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to start sync process")
	}

	// Update context for threading
	if progressResp != nil {
		ctx.ThreadTS = progressResp.Timestamp
	}

	// Process in background
	go func() {
		sr.processFullSyncWithProgress(ctx)
	}()

	return nil
}

func (sr *SmartRouter) processFullSyncWithProgress(ctx *ConversationContext) {
	startTime := time.Now()

	// Show detailed progress
	sr.slackClient.UpdateProgress(ctx, "📊 Syncing tasks from TimeCamp...")
	time.Sleep(1 * time.Second)

	sr.slackClient.UpdateProgress(ctx, "⏱️ Syncing time entries...")
	time.Sleep(2 * time.Second)

	// Perform the actual sync
	if err := FullSyncAll(); err != nil {
		sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Full sync failed: %v", err))
		return
	}

	// Send completion message
	duration := time.Since(startTime)
	message := fmt.Sprintf("✅ Full data synchronization completed successfully! (took %v)", duration.Round(time.Second))

	payload := map[string]interface{}{
		"channel": ctx.ChannelID,
		"text":    message,
		"blocks": []Block{
			{
				Type: "header",
				Text: &Text{Type: "plain_text", Text: "✅ Full Sync Complete"},
			},
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Full synchronization completed successfully*\n\n• All tasks synced from TimeCamp\n• Time entries synced (last 6 months)\n• Database is now up to date\n\n*Duration:* %v\n*Completed at:* %s",
						duration.Round(time.Second),
						time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	if ctx.ThreadTS != "" {
		payload["thread_ts"] = ctx.ThreadTS
	}

	sr.slackClient.sendSlackAPIRequest("chat.postMessage", payload)
	sr.logger.Infof("Completed full sync for user %s in %v", ctx.UserID, duration)
}

func (sr *SmartRouter) determinePeriod(text, command string) string {
	// Check explicit text
	text = strings.ToLower(strings.TrimSpace(text))
	for _, period := range []string{"daily", "weekly", "monthly"} {
		if strings.Contains(text, period) {
			return period
		}
	}

	// Check command name
	if strings.Contains(command, "daily") {
		return "daily"
	}
	if strings.Contains(command, "weekly") {
		return "weekly"
	}
	if strings.Contains(command, "monthly") {
		return "monthly"
	}

	return "daily" // default fallback
}

// HandleThresholdRequest processes threshold percentage requests like "over 50 daily"
func (sr *SmartRouter) HandleThresholdRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
		ProjectName: req.ProjectName, // Add project context
	}

	sr.logger.Infof("Processing threshold request from user %s: %s", req.UserName, req.Text)

	// Parse the threshold and period from the text
	threshold, period, err := sr.parseThresholdCommand(req.Text)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("❌ Invalid command format. Use: `/oye over <percentage> <period>`\nExample: `/oye over 50 daily`\nError: %v", err))
	}

	// Send initial progress message
	var progressMsg string
	if req.ProjectName != "" && req.ProjectName != "all" {
		progressMsg = fmt.Sprintf("🔍 Searching for %s project tasks over %.0f%% threshold for %s period...", req.ProjectName, threshold, period)
	} else {
		progressMsg = fmt.Sprintf("🔍 Searching for tasks over %.0f%% threshold for %s period...", threshold, period)
	}
	
	progressResp, err := sr.slackClient.SendProgressMessage(ctx, progressMsg)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to start threshold check")
	}

	// Update context with message timestamp for threading
	if progressResp != nil {
		ctx.ThreadTS = progressResp.Timestamp
	}

	// Process in background
	go func() {
		sr.processThresholdWithProgress(ctx, threshold, period)
	}()

	return nil
}

func (sr *SmartRouter) processThresholdWithProgress(ctx *ConversationContext, threshold float64, period string) {
	// Show progress updates
	if ctx.ProjectName != "" && ctx.ProjectName != "all" {
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📊 Querying database for project '%s' tasks with estimations...", ctx.ProjectName))
	} else {
		sr.slackClient.UpdateProgress(ctx, "📊 Querying database for tasks with estimations...")
	}
	time.Sleep(500 * time.Millisecond)

	// Get database connection
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	var taskInfos []TaskUpdateInfo
	
	if ctx.ProjectName != "" && ctx.ProjectName != "all" {
		// Project-specific threshold query
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("🔍 Finding project '%s'...", ctx.ProjectName))
		time.Sleep(500 * time.Millisecond)
		
		// Find the project
		projects, err := FindProjectsByName(db, ctx.ProjectName)
		if err != nil {
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to find project: %v", err))
			return
		}
		
		if len(projects) == 0 {
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Project '%s' not found. Use `/oye help` to see available commands.", ctx.ProjectName))
			return
		}
		
		if len(projects) > 1 {
			var projectNames []string
			for _, proj := range projects {
				projectNames = append(projectNames, proj.Name)
			}
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Multiple projects found for '%s': %s. Please be more specific.", ctx.ProjectName, strings.Join(projectNames, ", ")))
			return
		}
		
		project := projects[0]
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📈 Analyzing %s project tasks over %.0f%% threshold...", project.Name, threshold))
		time.Sleep(1 * time.Second)
		
		// Get project-specific tasks over threshold
		taskInfos, err = GetTasksOverThresholdWithProject(db, threshold, period, &project.TimeCampTaskID)
	} else {
		// All projects threshold query
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📈 Analyzing tasks over %.0f%% threshold...", threshold))
		time.Sleep(1 * time.Second)
		
		// Get all tasks over threshold
		taskInfos, err = GetTasksOverThreshold(db, threshold, period)
	}
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to get tasks over threshold: ```%v```", err)
		sr.slackClient.SendErrorResponse(ctx, errorMessage)
		return
	}

	sr.slackClient.UpdateProgress(ctx, "✍️ Formatting threshold report...")
	time.Sleep(500 * time.Millisecond)

	// Handle the case where there are no tasks
	if len(taskInfos) == 0 {
		err = sr.slackClient.SendThresholdNoResultsMessage(ctx, threshold, period)
		if err != nil {
			sr.logger.Errorf("Failed to send 'no results' message: %v", err)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
		}
		return
	}

	// Send final result
	err = sr.slackClient.SendThresholdResults(ctx, taskInfos, threshold, period)
	if err != nil {
		sr.logger.Errorf("Failed to send threshold results via Slack API: %v", err)

		// Try webhook fallback
		message := sr.slackClient.formatThresholdMessage(taskInfos, threshold, period)
		if webhookErr := sendSlackMessage(message); webhookErr != nil {
			sr.logger.Errorf("Webhook fallback also failed: %v", webhookErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
			return
		}
		sr.logger.Info("Successfully sent threshold report via webhook fallback")
	}

	sr.logger.Infof("Completed threshold check for user %s: %.0f%% threshold, %s period, %d tasks found",
		ctx.UserID, threshold, period, len(taskInfos))
}

// parseThresholdCommand parses commands like "over 50 daily" to extract threshold and period
func (sr *SmartRouter) parseThresholdCommand(text string) (float64, string, error) {
	text = strings.ToLower(strings.TrimSpace(text))

	// Remove "over" from the beginning if present
	text = strings.TrimPrefix(text, "over ")
	text = strings.TrimSpace(text)

	// Split into parts
	parts := strings.Fields(text)
	if len(parts) < 1 {
		return 0, "", fmt.Errorf("missing threshold percentage")
	}

	// Parse threshold percentage
	thresholdStr := parts[0]
	// Remove % sign if present
	thresholdStr = strings.TrimSuffix(thresholdStr, "%")

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid threshold percentage '%s'", parts[0])
	}

	if threshold < 0 || threshold > 1000 {
		return 0, "", fmt.Errorf("threshold percentage must be between 0 and 1000")
	}

	// Determine period
	period := "daily" // default
	if len(parts) >= 2 {
		switch parts[1] {
		case "daily", "day":
			period = "daily"
		case "weekly", "week":
			period = "weekly"
		case "monthly", "month":
			period = "monthly"
		default:
			return 0, "", fmt.Errorf("invalid period '%s'. Use: daily, weekly, or monthly", parts[1])
		}
	}

	return threshold, period, nil
}
