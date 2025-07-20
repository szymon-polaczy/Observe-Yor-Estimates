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

	// Parse period from command
	periodInfo := sr.parsePeriodFromText(req.Text, req.Command)

	if req.ProjectName != "" {
		sr.logger.Infof("Processing %s update request for project '%s' from user %s in channel %s",
			periodInfo.DisplayName, req.ProjectName, req.UserName, req.ChannelName)
	} else {
		sr.logger.Infof("Processing %s update request from user %s in channel %s", periodInfo.DisplayName, req.UserName, req.ChannelName)
	}

	// Handle long-running updates with progress tracking
	return sr.HandleLongRunningUpdate(ctx, periodInfo)
}

// HandleProjectAssignmentRequest processes project assignment commands
func (sr *SmartRouter) HandleProjectAssignmentRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID: req.ChannelID,
		UserID:    req.UserID,
	}

	parts := strings.Fields(req.Text)
	if len(parts) == 0 {
		return sr.slackClient.SendErrorResponse(ctx, "Please specify a command. Use `/oye help` for available commands.")
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "assign":
		return sr.handleAssignProject(ctx, parts[1:], req.UserID)
	case "unassign":
		return sr.handleUnassignProject(ctx, parts[1:], req.UserID)
	case "my-projects":
		return sr.handleMyProjects(ctx, req.UserID)
	case "available-projects":
		return sr.handleAvailableProjects(ctx)
	default:
		return sr.slackClient.SendErrorResponse(ctx, "Unknown command. Use `/oye help` for available commands.")
	}
}

func (sr *SmartRouter) handleAssignProject(ctx *ConversationContext, args []string, userID string) error {
	if len(args) == 0 {
		return sr.slackClient.SendErrorResponse(ctx, "Please specify a project name. Usage: `/oye assign \"Project Name\"`")
	}

	projectName := strings.Join(args, " ")
	projectName = strings.Trim(projectName, "\"'")

	db, err := GetDB()
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
	}

	// Find the project
	projects, err := FindProjectsByName(db, projectName)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to find project")
	}

	if len(projects) == 0 {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Project '%s' not found", projectName))
	}

	if len(projects) > 1 {
		projectNames := make([]string, len(projects))
		for i, p := range projects {
			projectNames[i] = p.Name
		}
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Multiple projects found: %s. Please be more specific.", strings.Join(projectNames, ", ")))
	}

	project := projects[0]
	err = AssignUserToProject(db, userID, project.ID)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to assign project")
	}

	message := fmt.Sprintf("✅ Successfully assigned you to project: **%s**", project.Name)

	// Refresh App Home view if possible
	go PublishAppHomeView(userID)

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

	return sr.slackClient.sendSlackAPIRequest("chat.postEphemeral", payload)
}

func (sr *SmartRouter) handleUnassignProject(ctx *ConversationContext, args []string, userID string) error {
	if len(args) == 0 {
		return sr.slackClient.SendErrorResponse(ctx, "Please specify a project name. Usage: `/oye unassign \"Project Name\"`")
	}

	projectName := strings.Join(args, " ")
	projectName = strings.Trim(projectName, "\"'")

	db, err := GetDB()
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
	}

	// Find the project
	projects, err := FindProjectsByName(db, projectName)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to find project")
	}

	if len(projects) == 0 {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Project '%s' not found", projectName))
	}

	if len(projects) > 1 {
		projectNames := make([]string, len(projects))
		for i, p := range projects {
			projectNames[i] = p.Name
		}
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Multiple projects found: %s. Please be more specific.", strings.Join(projectNames, ", ")))
	}

	project := projects[0]
	err = UnassignUserFromProject(db, userID, project.ID)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to unassign project: %v", err))
	}

	message := fmt.Sprintf("✅ Successfully removed you from project: **%s**", project.Name)

	// Refresh App Home view if possible
	go PublishAppHomeView(userID)

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

	return sr.slackClient.sendSlackAPIRequest("chat.postEphemeral", payload)
}

func (sr *SmartRouter) handleMyProjects(ctx *ConversationContext, userID string) error {
	db, err := GetDB()
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
	}

	projects, err := GetUserProjects(db, userID)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to retrieve your projects")
	}

	var message string
	if len(projects) == 0 {
		message = "📋 **Your Assigned Projects:**\n\nYou are not assigned to any projects. You will see all projects in automatic updates.\n\nUse `/oye available-projects` to see all projects, then `/oye assign \"Project Name\"` to assign yourself."
	} else {
		message = "📋 **Your Assigned Projects:**\n\n"
		for _, project := range projects {
			message += fmt.Sprintf("• %s\n", project.Name)
		}
		message += "\nUse `/oye unassign \"Project Name\"` to remove assignments."
	}

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

	return sr.slackClient.sendSlackAPIRequest("chat.postEphemeral", payload)
}

func (sr *SmartRouter) handleAvailableProjects(ctx *ConversationContext) error {
	db, err := GetDB()
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
	}

	projects, err := GetAllProjects(db)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to retrieve projects")
	}

	if len(projects) == 0 {
		message := "📁 **Available Projects:**\n\nNo projects found in the database."
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
		return sr.slackClient.sendSlackAPIRequest("chat.postEphemeral", payload)
	}

	// Split projects into chunks to respect Slack's message limits
	projectChunks := sr.splitProjectsIntoChunks(projects)

	for i, chunk := range projectChunks {
		var message string
		var blocks []Block

		if i == 0 {
			// First message - include header
			message = fmt.Sprintf("📁 **Available Projects** (%d total):\n\n", len(projects))
		} else {
			// Subsequent messages - continuation
			message = fmt.Sprintf("📁 **Available Projects** (continued - part %d of %d):\n\n", i+1, len(projectChunks))
		}

		// Add projects in this chunk
		for _, project := range chunk {
			message += fmt.Sprintf("• %s\n", project.Name)
		}

		// Add footer to last message
		if i == len(projectChunks)-1 {
			message += "\n💡 Use `/oye assign \"Project Name\"` to assign yourself to a project."
		}

		blocks = []Block{
			{
				Type: "section",
				Text: &Text{Type: "mrkdwn", Text: message},
			},
		}

		// Add part indicator for multiple parts
		if len(projectChunks) > 1 {
			blocks = append(blocks, Block{
				Type: "context",
				Elements: []Element{
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("Part %d of %d", i+1, len(projectChunks)),
					},
				},
			})
		}

		payload := map[string]interface{}{
			"channel": ctx.ChannelID,
			"user":    ctx.UserID,
			"text":    message,
			"blocks":  blocks,
		}

		if err := sr.slackClient.sendSlackAPIRequest("chat.postEphemeral", payload); err != nil {
			return err
		}
	}

	return nil
}

// splitProjectsIntoChunks splits projects into chunks that fit within Slack's message limits
func (sr *SmartRouter) splitProjectsIntoChunks(projects []Project) [][]Project {
	const maxMessageSize = 1000 // Conservative size to account for JSON overhead
	var chunks [][]Project
	var currentChunk []Project
	var currentSize int

	for _, project := range projects {
		// Estimate the size this project would add (project name + bullet + newline)
		projectSize := len(project.Name) + 3 // "• " + "\n"

		// Check if adding this project would exceed the size limit
		if currentSize+projectSize > maxMessageSize && len(currentChunk) > 0 {
			// Start a new chunk
			chunks = append(chunks, currentChunk)
			currentChunk = []Project{project}
			currentSize = projectSize + 150 // Add buffer for headers
		} else {
			// Add to current chunk
			currentChunk = append(currentChunk, project)
			currentSize += projectSize
		}
	}

	// Add the last chunk if it has projects
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	// If no chunks were created (empty project list), return empty slice
	if len(chunks) == 0 {
		chunks = append(chunks, []Project{})
	}

	return chunks
}

// HandleLongRunningUpdate shows progress and delivers final result
func (sr *SmartRouter) HandleLongRunningUpdate(ctx *ConversationContext, periodInfo PeriodInfo) error {
	// Send initial progress message
	progressResp, err := sr.slackClient.SendProgressMessage(ctx,
		fmt.Sprintf("🔄 Generating your %s update...", periodInfo.DisplayName))
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to start update process")
	}

	// Update context with message timestamp for threading
	if progressResp != nil {
		ctx.ThreadTS = progressResp.Timestamp
	}

	// Process in background
	go func() {
		sr.processUpdateWithProgress(ctx, periodInfo)
	}()

	return nil
}

func (sr *SmartRouter) processUpdateWithProgress(ctx *ConversationContext, periodInfo PeriodInfo) {
	// Get database connection
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	var taskInfos []TaskUpdateInfo

	if ctx.ProjectName != "" && ctx.ProjectName != "all" {
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

		// Get project-specific task data
		taskInfos, err = getTaskChangesWithProject(db, periodInfo.Type, &project.TimeCampTaskID)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to get %s changes for project '%s': ```%v```", periodInfo.DisplayName, project.Name, err)
			sr.slackClient.SendErrorResponse(ctx, errorMessage)
			return
		}
	} else {
		// Check if user has specific project assignments
		userProjects, err := GetUserProjects(db, ctx.UserID)
		if err != nil {
			sr.logger.Errorf("Failed to get user projects: %v", err)
			// Continue with all projects if we can't get user assignments
		}

		// If user has specific project assignments, filter to only those projects
		if len(userProjects) > 0 {
			// Get combined task data for all assigned projects
			var allProjectTasks []TaskUpdateInfo
			for _, project := range userProjects {
				projectTasks, err := getTaskChangesWithProject(db, periodInfo.Type, &project.TimeCampTaskID)
				if err != nil {
					sr.logger.Errorf("Failed to get tasks for project %s: %v", project.Name, err)
					continue
				}
				allProjectTasks = append(allProjectTasks, projectTasks...)
			}
			taskInfos = allProjectTasks
		} else {
			// Get task data for all projects (user has no specific assignments)
			taskInfos, err = getTaskChanges(db, periodInfo.Type)
			if err != nil {
				errorMessage := fmt.Sprintf("Failed to get %s changes: ```%v```", periodInfo.DisplayName, err)
				sr.slackClient.SendErrorResponse(ctx, errorMessage)
				return
			}
		}
	}

	// Handle the case where there are no tasks
	if len(taskInfos) == 0 {
		err = sr.slackClient.SendNoChangesMessage(ctx, periodInfo.DisplayName)
		if err != nil {
			sr.logger.Errorf("Failed to send 'no changes' message: %v", err)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", periodInfo.DisplayName))
		}
		return
	}

	// Send final result as public message in thread
	err = sr.slackClient.SendFinalUpdate(ctx, taskInfos, periodInfo.DisplayName)

	if err != nil {
		sr.logger.Errorf("Failed to send final %s report via Slack API: %v", periodInfo.DisplayName, err)

		// Try webhook fallback with proper splitting
		sr.logger.Info("Attempting webhook fallback with message splitting...")

		// Get all tasks for hierarchy mapping (same logic as SendFinalUpdate)
		db, dbErr := GetDB()
		if dbErr != nil {
			sr.logger.Errorf("Webhook fallback failed - database error: %v", dbErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", periodInfo.DisplayName))
			return
		}

		allTasks, taskErr := getAllTasks(db)
		if taskErr != nil {
			sr.logger.Errorf("Webhook fallback failed - tasks error: %v", taskErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", periodInfo.DisplayName))
			return
		}

		// Group tasks by project and send via webhook
		projectGroups := groupTasksByTopParent(taskInfos, allTasks)

		// Convert TaskUpdateInfo to TaskInfo for new formatting
		convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
		
		// Test if single message would work
		testMessage := FormatTaskMessage(convertedTasks, periodInfo.DisplayName, FormatOptions{ShowHeader: true})
		validation := ValidateMessage(testMessage)

		if validation.IsValid && len(projectGroups) <= 15 && len(taskInfos) <= 25 {
			// Single message fits, send it
			if webhookErr := sendSlackMessage(testMessage); webhookErr != nil {
				sr.logger.Errorf("Webhook fallback failed: %v", webhookErr)
				sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", periodInfo.DisplayName))
				return
			}
		} else {
			// Message too large, split by project
			sr.logger.Warnf("Webhook fallback: Message too large, splitting by project")

			// Send simple message via webhook
			message := formatProjectMessageWithComments("All Tasks", taskInfos, periodInfo.DisplayName, 1, 1)

			if webhookErr := sendSlackMessage(message); webhookErr != nil {
				sr.logger.Errorf("Webhook fallback failed: %v", webhookErr)
				sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", periodInfo.DisplayName))
				return
			}
		}

		sr.logger.Info("Successfully sent report via webhook fallback")
	}

	sr.logger.Infof("Completed %s update for user %s", periodInfo.DisplayName, ctx.UserID)
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
	sr.slackClient.UpdateProgress(ctx, "📊 Syncing started...")

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

// PeriodInfo already defined in types.go

// parsePeriodFromText parses natural language time periods
func (sr *SmartRouter) parsePeriodFromText(text, command string) PeriodInfo {
	text = strings.ToLower(strings.TrimSpace(text))

	// Remove project names from text for period parsing
	// This handles cases like "filestage last 7 days"
	words := strings.Fields(text)
	var periodWords []string

	// Look for period-related keywords and numbers
	for i, word := range words {
		// Check for "last X days" pattern
		if word == "last" && i+2 < len(words) && words[i+2] == "days" {
			if days, err := strconv.Atoi(words[i+1]); err == nil && days >= 1 && days <= 60 {
				return PeriodInfo{
					Type:        "last_x_days",
					Days:        days,
					DisplayName: fmt.Sprintf("Last %d Days", days),
				}
			}
		}

		// Check for "last X day" (singular)
		if word == "last" && i+2 < len(words) && words[i+2] == "day" {
			if days, err := strconv.Atoi(words[i+1]); err == nil && days == 1 {
				return PeriodInfo{
					Type:        "yesterday",
					Days:        1,
					DisplayName: "Yesterday",
				}
			}
		}

		// Collect words that might be period-related
		if word == "today" || word == "yesterday" || word == "this" || word == "last" ||
			word == "week" || word == "month" || word == "daily" || word == "weekly" || word == "monthly" ||
			word == "days" || word == "day" {
			periodWords = append(periodWords, word)
		}
	}

	// Join period words to check for multi-word patterns
	periodText := strings.Join(periodWords, " ")

	// Check for specific patterns
	switch {
	case strings.Contains(periodText, "today"):
		return PeriodInfo{Type: "today", Days: 0, DisplayName: "Today"}
	case strings.Contains(periodText, "yesterday"):
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	case strings.Contains(periodText, "this week"):
		return PeriodInfo{Type: "this_week", Days: 0, DisplayName: "This Week"}
	case strings.Contains(periodText, "last week"):
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	case strings.Contains(periodText, "this month"):
		return PeriodInfo{Type: "this_month", Days: 0, DisplayName: "This Month"}
	case strings.Contains(periodText, "last month"):
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	}

	// Check for backwards compatibility with old period names
	if strings.Contains(periodText, "weekly") || strings.Contains(periodText, "week") {
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	}
	if strings.Contains(periodText, "monthly") || strings.Contains(periodText, "month") {
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	}
	if strings.Contains(periodText, "daily") || strings.Contains(periodText, "day") {
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	}

	// Check command name for backwards compatibility
	if strings.Contains(command, "daily") {
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	}
	if strings.Contains(command, "weekly") {
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	}
	if strings.Contains(command, "monthly") {
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	}

	// Default fallback
	return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
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
	threshold, periodInfo, err := sr.parseThresholdCommand(req.Text)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("❌ Invalid command format. Use: `/oye over <percentage> <period>`\nExample: `/oye over 50 daily` or `/oye over 50 last 7 days`\nError: %v", err))
	}

	// Send initial progress message
	var progressMsg string
	if req.ProjectName != "" && req.ProjectName != "all" {
		progressMsg = fmt.Sprintf("🔍 Searching for %s project tasks over %.0f%% threshold for %s period...", req.ProjectName, threshold, periodInfo.DisplayName)
	} else {
		progressMsg = fmt.Sprintf("🔍 Searching for tasks over %.0f%% threshold for %s period...", threshold, periodInfo.DisplayName)
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
		sr.processThresholdWithProgress(ctx, threshold, periodInfo)
	}()

	return nil
}

func (sr *SmartRouter) processThresholdWithProgress(ctx *ConversationContext, threshold float64, periodInfo PeriodInfo) {
	// Get database connection
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	var taskInfos []TaskUpdateInfo

	if ctx.ProjectName != "" && ctx.ProjectName != "all" {
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

		// Get project-specific tasks over threshold
		taskInfoList, err := GetTasksOverThresholdWithProject(db, threshold, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
		if err == nil {
			taskInfos = convertTaskInfoToTaskUpdateInfo(taskInfoList)
		}
	} else {
		// All projects threshold query - check if user has project assignments
		sr.slackClient.UpdateProgress(ctx, fmt.Sprintf("📈 Analyzing tasks over %.0f%% threshold...", threshold))

		// Check if user has specific project assignments
		userProjects, err := GetUserProjects(db, ctx.UserID)
		if err != nil {
			sr.logger.Errorf("Failed to get user projects: %v", err)
			// Continue with all projects if we can't get user assignments
		}

		// If user has specific project assignments, filter to only those projects
		if len(userProjects) > 0 {
			// Get combined threshold data for all assigned projects
			var allProjectTasks []TaskUpdateInfo
			for _, project := range userProjects {
				projectTaskInfos, err := GetTasksOverThresholdWithProject(db, threshold, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
				if err != nil {
					sr.logger.Errorf("Failed to get threshold tasks for project %s: %v", project.Name, err)
					continue
				}
				projectTasks := convertTaskInfoToTaskUpdateInfo(projectTaskInfos)
				allProjectTasks = append(allProjectTasks, projectTasks...)
			}
			taskInfos = allProjectTasks
		} else {
			// Get all tasks over threshold (user has no specific assignments)
			allTaskInfos, err := GetTasksOverThreshold(db, threshold, periodInfo.Type, periodInfo.Days)
			if err == nil {
				taskInfos = convertTaskInfoToTaskUpdateInfo(allTaskInfos)
			}
		}
	}
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to get tasks over threshold: ```%v```", err)
		sr.slackClient.SendErrorResponse(ctx, errorMessage)
		return
	}

	// Handle the case where there are no tasks
	if len(taskInfos) == 0 {
		err = sr.slackClient.SendThresholdNoResultsMessage(ctx, threshold, periodInfo.DisplayName)
		if err != nil {
			sr.logger.Errorf("Failed to send 'no results' message: %v", err)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
		}
		return
	}

	// Send final result
	err = sr.slackClient.SendThresholdResults(ctx, taskInfos, threshold, periodInfo.DisplayName)
	if err != nil {
		sr.logger.Errorf("Failed to send threshold results via Slack API: %v", err)

		// Try webhook fallback with proper splitting
		sr.logger.Info("Attempting webhook fallback with message splitting...")

		// Get all tasks for hierarchy mapping (same logic as SendThresholdResults)
		db, dbErr := GetDB()
		if dbErr != nil {
			sr.logger.Errorf("Webhook fallback failed - database error: %v", dbErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
			return
		}

		allTasks, taskErr := getAllTasks(db)
		if taskErr != nil {
			sr.logger.Errorf("Webhook fallback failed - tasks error: %v", taskErr)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
			return
		}

		// Group tasks by project and send via webhook
		projectGroups := groupTasksByTopParent(taskInfos, allTasks)

		// Convert TaskUpdateInfo to TaskInfo for new formatting
		convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
		
		// Test if single message would work
		testMessage := FormatTaskMessage(convertedTasks, periodInfo.DisplayName, FormatOptions{
			ShowHeader: true,
			Threshold: &threshold,
		})
		validation := ValidateMessage(testMessage)

		if validation.IsValid && len(projectGroups) <= 15 && len(taskInfos) <= 25 {
			// Single message fits, send it
			if webhookErr := sendSlackMessage(testMessage); webhookErr != nil {
				sr.logger.Errorf("Webhook fallback failed: %v", webhookErr)
				sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
				return
			}
		} else {
			// Message too large, split by project
			sr.logger.Warnf("Webhook fallback: Message too large, splitting by project")

			// Send simple message via webhook
			message := formatProjectMessageWithComments("Threshold Results", taskInfos, periodInfo.DisplayName, 1, 1)

			if webhookErr := sendSlackMessage(message); webhookErr != nil {
				sr.logger.Errorf("Webhook fallback failed: %v", webhookErr)
				sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
				return
			}
		}

		sr.logger.Info("Successfully sent threshold report via webhook fallback")
	}

	sr.logger.Infof("Completed threshold check for user %s: %.0f%% threshold, %s period, %d tasks found",
		ctx.UserID, threshold, periodInfo.DisplayName, len(taskInfos))
}

// parseThresholdCommand parses commands like "over 50 daily" or "over 50 last 7 days" to extract threshold and period
func (sr *SmartRouter) parseThresholdCommand(text string) (float64, PeriodInfo, error) {
	text = strings.ToLower(strings.TrimSpace(text))

	// Remove "over" from the beginning if present
	text = strings.TrimPrefix(text, "over ")
	text = strings.TrimSpace(text)

	// Split into parts
	parts := strings.Fields(text)
	if len(parts) < 1 {
		return 0, PeriodInfo{}, fmt.Errorf("missing threshold percentage")
	}

	// Parse threshold percentage
	thresholdStr := parts[0]
	// Remove % sign if present
	thresholdStr = strings.TrimSuffix(thresholdStr, "%")

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return 0, PeriodInfo{}, fmt.Errorf("invalid threshold percentage '%s'", parts[0])
	}

	if threshold < 0 || threshold > 1000 {
		return 0, PeriodInfo{}, fmt.Errorf("threshold percentage must be between 0 and 1000")
	}

	// Parse period from remaining text
	if len(parts) >= 2 {
		periodText := strings.Join(parts[1:], " ")
		periodInfo := sr.parsePeriodFromText(periodText, "")
		return threshold, periodInfo, nil
	}

	// Default period
	return threshold, PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}, nil
}

// convertTaskUpdateInfoToTaskInfo defined in slack_client.go
