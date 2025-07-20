package main

import (
	"fmt"
	"strconv"
	"strings"
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

// UnifiedUpdateRequest represents a unified update request from any source
type UnifiedUpdateRequest struct {
	Command     string      // "update", "threshold", etc.
	Text        string      // Raw command text for parsing
	ProjectName string      // Optional project name
	UserID      string      // Optional user ID for filtering
	Source      string      // "cli" or "slack"
	PeriodInfo  *PeriodInfo // Optional pre-parsed period info
}

// UnifiedUpdateResult contains the results of processing an update
type UnifiedUpdateResult struct {
	TaskInfos   []TaskUpdateInfo
	PeriodInfo  PeriodInfo
	ProjectName string
	Source      string
	Success     bool
	ErrorMsg    string
}

// ProcessUnifiedUpdate processes update commands from any source (CLI or Slack)
func (sr *SmartRouter) ProcessUnifiedUpdate(req *UnifiedUpdateRequest) *UnifiedUpdateResult {
	result := &UnifiedUpdateResult{
		Source: req.Source,
	}

	// Use pre-parsed period if available, otherwise parse from text
	var periodInfo PeriodInfo
	if req.PeriodInfo != nil {
		periodInfo = *req.PeriodInfo
	} else {
		periodInfo = sr.parsePeriodFromText(req.Text, req.Command)
	}
	result.PeriodInfo = periodInfo

	// Log the request
	if req.ProjectName != "" {
		sr.logger.Infof("Processing %s update request for project '%s' from %s",
			periodInfo.DisplayName, req.ProjectName, req.Source)
	} else {
		sr.logger.Infof("Processing %s update request from %s",
			periodInfo.DisplayName, req.Source)
	}

	// Get database connection
	db, err := GetDB()
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("Database connection failed: %v", err)
		return result
	}

	var taskInfos []TaskUpdateInfo

	if req.ProjectName != "" && req.ProjectName != "all" {
		// Find the project
		projects, err := FindProjectsByName(db, req.ProjectName)
		if err != nil {
			result.ErrorMsg = fmt.Sprintf("Failed to find project: %v", err)
			return result
		}

		if len(projects) == 0 {
			result.ErrorMsg = fmt.Sprintf("Project '%s' not found", req.ProjectName)
			return result
		}

		if len(projects) > 1 {
			projectNames := make([]string, len(projects))
			for i, p := range projects {
				projectNames[i] = p.Name
			}
			result.ErrorMsg = fmt.Sprintf("Multiple projects found matching '%s': %s. Please be more specific.",
				req.ProjectName, strings.Join(projectNames, ", "))
			return result
		}

		project := projects[0]
		result.ProjectName = project.Name

		// Get project-specific task data
		taskInfos, err = getTaskChangesWithProject(db, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
		if err != nil {
			result.ErrorMsg = fmt.Sprintf("Failed to get %s changes for project '%s': %v",
				periodInfo.DisplayName, project.Name, err)
			return result
		}
	} else {
		// Check if user has specific project assignments (only for Slack)
		if req.Source == "slack" && req.UserID != "" {
			userProjects, err := GetUserProjects(db, req.UserID)
			if err != nil {
				sr.logger.Errorf("Failed to get user projects: %v", err)
				// Continue with all projects if we can't get user assignments
			}

			// If user has specific project assignments, filter to only those projects
			if len(userProjects) > 0 {
				var allProjectTasks []TaskUpdateInfo
				for _, project := range userProjects {
					projectTasks, err := getTaskChangesWithProject(db, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
					if err != nil {
						sr.logger.Errorf("Failed to get tasks for project %s: %v", project.Name, err)
						continue
					}
					allProjectTasks = append(allProjectTasks, projectTasks...)
				}
				taskInfos = allProjectTasks
			} else {
				// Get task data for all projects (user has no specific assignments)
				taskInfos, err = getTaskChanges(db, periodInfo.Type, periodInfo.Days)
				if err != nil {
					result.ErrorMsg = fmt.Sprintf("Failed to get %s changes: %v", periodInfo.DisplayName, err)
					return result
				}
			}
		} else {
			// CLI or no user filtering - get all tasks
			taskInfos, err = getTaskChanges(db, periodInfo.Type, periodInfo.Days)
			if err != nil {
				result.ErrorMsg = fmt.Sprintf("Failed to get %s changes: %v", periodInfo.DisplayName, err)
				return result
			}
		}
	}

	result.TaskInfos = taskInfos
	result.Success = true
	return result
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
	// Use unified processor
	req := &UnifiedUpdateRequest{
		Command:     "update",
		Text:        "", // Not needed since we're passing PeriodInfo
		ProjectName: ctx.ProjectName,
		UserID:      ctx.UserID,
		Source:      "slack",
		PeriodInfo:  &periodInfo, // Pass the already parsed period
	}

	result := sr.ProcessUnifiedUpdate(req)
	if !result.Success {
		sr.slackClient.SendErrorResponse(ctx, result.ErrorMsg)
		return
	}

	taskInfos := result.TaskInfos

	// Handle the case where there are no tasks
	if len(taskInfos) == 0 {
		err := sr.slackClient.SendNoChangesMessage(ctx, result.PeriodInfo.DisplayName)
		if err != nil {
			sr.logger.Errorf("Failed to send 'no changes' message: %v", err)
			sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", result.PeriodInfo.DisplayName))
		}
		return
	}

	// Send final result as public message in thread
	err := sr.slackClient.SendFinalUpdate(ctx, taskInfos, result.PeriodInfo.DisplayName)

	if err != nil {
		sr.logger.Errorf("Failed to send final %s report via Slack API: %v", result.PeriodInfo.DisplayName, err)

		// Try webhook fallback with proper splitting
		sr.logger.Info("Attempting webhook fallback with message splitting...")

		// Convert TaskUpdateInfo to TaskInfo for new formatting
		convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)

		// Test if single message would work (use new simplified messaging)
		err = SendTaskMessage(convertedTasks, result.PeriodInfo.DisplayName)
		if err == nil {
			return // Successfully sent
		}

		// Fallback error handling
		sr.logger.Errorf("Task message failed: %v", err)
		sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", result.PeriodInfo.DisplayName))
		return
	}

	sr.logger.Infof("Completed %s update for user %s", result.PeriodInfo.DisplayName, ctx.UserID)
}

// parsePeriodFromText parses natural language time periods
func (sr *SmartRouter) parsePeriodFromText(text, command string) PeriodInfo {
	text = strings.ToLower(strings.TrimSpace(text))

	// Look for "last X days" pattern first
	words := strings.Fields(text)
	for i, word := range words {
		if word == "last" && i+2 < len(words) {
			if words[i+2] == "days" || words[i+2] == "day" {
				if days, err := strconv.Atoi(words[i+1]); err == nil && days >= 1 && days <= 60 {
					return PeriodInfo{
						Type:        "last_x_days",
						Days:        days,
						DisplayName: fmt.Sprintf("Last %d Days", days),
					}
				}
			}
		}
	}

	// Check for specific period keywords (order matters - check more specific patterns first)
	switch {
	case strings.Contains(text, "today"):
		return PeriodInfo{Type: "today", Days: 0, DisplayName: "Today"}
	case strings.Contains(text, "yesterday"):
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	case strings.Contains(text, "this week"):
		return PeriodInfo{Type: "this_week", Days: 0, DisplayName: "This Week"}
	case strings.Contains(text, "last week"):
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	case strings.Contains(text, "this month"):
		return PeriodInfo{Type: "this_month", Days: 0, DisplayName: "This Month"}
	case strings.Contains(text, "last month"):
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	case strings.Contains(text, "weekly"):
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	case strings.Contains(text, "monthly"):
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

		// Convert TaskUpdateInfo to TaskInfo for new formatting
		convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)

		// Send threshold message using new simplified messaging
		err = SendThresholdMessage(convertedTasks, periodInfo.DisplayName, threshold)
		if err == nil {
			return // Successfully sent
		}

		// Fallback error handling
		sr.logger.Errorf("Threshold message failed: %v", err)
		sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send threshold report"))
		return
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
