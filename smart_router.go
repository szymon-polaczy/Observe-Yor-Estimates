package main

import (
	"database/sql"
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

type UnifiedUpdateRequest struct {
	Command     string
	Text        string
	ProjectName string
	UserID      string
	Source      string
	PeriodInfo  *PeriodInfo
}

type UnifiedUpdateResult struct {
	TaskInfos   []TaskUpdateInfo
	PeriodInfo  PeriodInfo
	ProjectName string
	Source      string
	Success     bool
	ErrorMsg    string
}

func (sr *SmartRouter) logRequest(projectName, displayName, source string) {
	if projectName != "" {
		sr.logger.Infof("Processing %s update request for project '%s' from %s", displayName, projectName, source)
	} else {
		sr.logger.Infof("Processing %s update request from %s", displayName, source)
	}
}

func (sr *SmartRouter) findProject(db *sql.DB, projectName string) (*Project, error) {
	projects, err := FindProjectsByName(db, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to find project: %v", err)
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("project '%s' not found", projectName)
	}

	if len(projects) > 1 {
		projectNames := make([]string, len(projects))
		for i, p := range projects {
			projectNames[i] = p.Name
		}
		return nil, fmt.Errorf("multiple projects found matching '%s': %s. Please be more specific",
			projectName, strings.Join(projectNames, ", "))
	}

	return &projects[0], nil
}

func (sr *SmartRouter) getTaskData(db *sql.DB, req *UnifiedUpdateRequest, periodInfo PeriodInfo) ([]TaskUpdateInfo, string, error) {
	var taskInfos []TaskUpdateInfo
	var projectName string

	if req.ProjectName != "" && req.ProjectName != "all" {
		project, err := sr.findProject(db, req.ProjectName)
		if err != nil {
			return nil, "", err
		}

		projectName = project.Name
		taskInfos, err = getTaskChangesWithProject(db, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get %s changes for project '%s': %v",
				periodInfo.DisplayName, project.Name, err)
		}
	} else {
		var err error
		if req.Source == "slack" && req.UserID != "" {
			taskInfos, err = sr.getUserFilteredTasks(db, req.UserID, periodInfo)
		} else {
			taskInfos, err = getTaskChanges(db, periodInfo.Type, periodInfo.Days)
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to get %s changes: %v", periodInfo.DisplayName, err)
		}
	}

	return taskInfos, projectName, nil
}

func (sr *SmartRouter) getUserFilteredTasks(db *sql.DB, userID string, periodInfo PeriodInfo) ([]TaskUpdateInfo, error) {
	userProjects, err := GetUserProjects(db, userID)
	if err != nil {
		sr.logger.Errorf("Failed to get user projects: %v", err)
		return getTaskChanges(db, periodInfo.Type, periodInfo.Days)
	}

	if len(userProjects) == 0 {
		return getTaskChanges(db, periodInfo.Type, periodInfo.Days)
	}

	var allProjectTasks []TaskUpdateInfo
	for _, project := range userProjects {
		projectTasks, err := getTaskChangesWithProject(db, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
		if err != nil {
			sr.logger.Errorf("Failed to get tasks for project %s: %v", project.Name, err)
			continue
		}
		allProjectTasks = append(allProjectTasks, projectTasks...)
	}
	return allProjectTasks, nil
}

func (sr *SmartRouter) getThresholdTasks(db *sql.DB, userID, projectName string, threshold float64, periodInfo PeriodInfo) ([]TaskUpdateInfo, error) {
	if projectName != "" && projectName != "all" {
		project, err := sr.findProject(db, projectName)
		if err != nil {
			return nil, err
		}

		taskInfos, err := GetTasksOverThresholdWithProject(db, threshold, periodInfo.Type, periodInfo.Days, &project.TimeCampTaskID)
		if err != nil {
			return nil, err
		}
		return convertTaskInfoToTaskUpdateInfo(taskInfos), nil
	}

	userProjects, err := GetUserProjects(db, userID)
	if err != nil {
		sr.logger.Errorf("Failed to get user projects: %v", err)
	}

	if len(userProjects) > 0 {
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
		return allProjectTasks, nil
	}

	allTaskInfos, err := GetTasksOverThresholdWithProject(db, threshold, periodInfo.Type, periodInfo.Days, nil)
	if err != nil {
		return nil, err
	}
	return convertTaskInfoToTaskUpdateInfo(allTaskInfos), nil
}

func (sr *SmartRouter) ProcessUnifiedUpdate(req *UnifiedUpdateRequest) *UnifiedUpdateResult {
	result := &UnifiedUpdateResult{Source: req.Source}

	var periodInfo PeriodInfo
	if req.PeriodInfo != nil {
		periodInfo = *req.PeriodInfo
	} else {
		periodInfo = sr.parsePeriodFromText(req.Text, req.Command)
	}
	result.PeriodInfo = periodInfo

	sr.logRequest(req.ProjectName, periodInfo.DisplayName, req.Source)

	db, err := GetDB()
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("Database connection failed: %v", err)
		return result
	}

	taskInfos, projectName, err := sr.getTaskData(db, req, periodInfo)
	if err != nil {
		result.ErrorMsg = err.Error()
		return result
	}

	result.TaskInfos = taskInfos
	result.ProjectName = projectName
	result.Success = true
	return result
}

func (sr *SmartRouter) HandleUpdateRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
		ProjectName: req.ProjectName,
	}

	periodInfo := sr.parsePeriodFromText(req.Text, req.Command)
	sr.logRequest(req.ProjectName, periodInfo.DisplayName, "slack")

	go sr.processUpdateWithProgress(ctx, periodInfo, req.ResponseURL)
	return nil
}

func (sr *SmartRouter) HandleThresholdRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
		ProjectName: req.ProjectName,
	}

	threshold, periodInfo, err := sr.parseThresholdCommand(req.Text)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("❌ Invalid command format. Use: `/oye over <percentage> <period>`\nExample: `/oye over 50 daily`\nError: %v", err))
	}

	progressMsg := sr.formatThresholdProgressMsg(req.ProjectName, threshold, periodInfo.DisplayName)
	progressResp, err := sr.slackClient.SendProgressMessage(ctx, progressMsg)
	if err != nil {
		return sr.slackClient.SendErrorResponse(ctx, "Failed to start threshold check")
	}

	if progressResp != nil {
		ctx.ThreadTS = progressResp.Timestamp
	}

	go sr.processThresholdWithProgress(ctx, threshold, periodInfo)
	return nil
}

func (sr *SmartRouter) formatThresholdProgressMsg(projectName string, threshold float64, displayName string) string {
	if projectName != "" && projectName != "all" {
		return fmt.Sprintf("🔍 Searching for %s project tasks over %.0f%% threshold for %s period...",
			projectName, threshold, displayName)
	}
	return fmt.Sprintf("🔍 Searching for tasks over %.0f%% threshold for %s period...",
		threshold, displayName)
}

func (sr *SmartRouter) processUpdateWithProgress(ctx *ConversationContext, periodInfo PeriodInfo, responseURL string) {
	req := &UnifiedUpdateRequest{
		Command:     "update",
		ProjectName: ctx.ProjectName,
		UserID:      ctx.UserID,
		Source:      "slack",
		PeriodInfo:  &periodInfo,
	}

	result := sr.ProcessUnifiedUpdate(req)
	if !result.Success {
		sr.slackClient.SendErrorResponse(ctx, result.ErrorMsg)
		return
	}

	if len(result.TaskInfos) == 0 {
		sr.slackClient.SendNoChangesMessage(ctx, result.PeriodInfo.DisplayName)
		return
	}

	err := sr.slackClient.SendFinalUpdate(ctx, result.TaskInfos, result.PeriodInfo.DisplayName)
	if err != nil {
		sr.logger.Errorf("Failed to send final report: %v", err)
		sr.sendFallbackMessage(result.TaskInfos, result.PeriodInfo.DisplayName, responseURL, ctx)
	}

	sr.logger.Infof("Completed %s update for user %s", result.PeriodInfo.DisplayName, ctx.UserID)
}

func (sr *SmartRouter) processThresholdWithProgress(ctx *ConversationContext, threshold float64, periodInfo PeriodInfo) {
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	taskInfos, err := sr.getThresholdTasks(db, ctx.UserID, ctx.ProjectName, threshold, periodInfo)
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to get tasks over threshold: %v", err))
		return
	}

	if len(taskInfos) == 0 {
		sr.slackClient.SendThresholdNoResultsMessage(ctx, threshold, periodInfo.DisplayName)
		return
	}

	err = sr.slackClient.SendThresholdResults(ctx, taskInfos, threshold, periodInfo.DisplayName)
	if err != nil {
		sr.logger.Errorf("Failed to send threshold results: %v", err)
		sr.sendThresholdFallback(taskInfos, threshold, periodInfo.DisplayName, ctx)
	}

	sr.logger.Infof("Completed threshold check for user %s: %.0f%% threshold, %s period, %d tasks found",
		ctx.UserID, threshold, periodInfo.DisplayName, len(taskInfos))
}

func (sr *SmartRouter) sendFallbackMessage(taskInfos []TaskUpdateInfo, displayName, responseURL string, ctx *ConversationContext) {
	convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
	err := SendTaskMessageToResponseURL(convertedTasks, displayName, responseURL)
	if err != nil {
		sr.logger.Errorf("Response URL fallback failed: %v", err)
		sr.slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to send %s report", displayName))
	}
}

func (sr *SmartRouter) sendThresholdFallback(taskInfos []TaskUpdateInfo, threshold float64, displayName string, ctx *ConversationContext) {
	convertedTasks := convertTaskUpdateInfoToTaskInfo(taskInfos)
	err := SendThresholdMessage(convertedTasks, displayName, threshold)
	if err != nil {
		sr.logger.Errorf("Threshold message failed: %v", err)
		sr.slackClient.SendErrorResponse(ctx, "Failed to send threshold report")
	}
}

func (sr *SmartRouter) parsePeriodFromText(text, command string) PeriodInfo {
	text = strings.ToLower(strings.TrimSpace(text))

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

	periodMap := map[string]PeriodInfo{
		"today":      {Type: "today", Days: 0, DisplayName: "Today"},
		"yesterday":  {Type: "yesterday", Days: 1, DisplayName: "Yesterday"},
		"this week":  {Type: "this_week", Days: 0, DisplayName: "This Week"},
		"last week":  {Type: "last_week", Days: 7, DisplayName: "Last Week"},
		"this month": {Type: "this_month", Days: 0, DisplayName: "This Month"},
		"last month": {Type: "last_month", Days: 30, DisplayName: "Last Month"},
		"weekly":     {Type: "last_week", Days: 7, DisplayName: "Last Week"},
		"monthly":    {Type: "last_month", Days: 30, DisplayName: "Last Month"},
	}

	for keyword, period := range periodMap {
		if strings.Contains(text, keyword) {
			return period
		}
	}

	return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
}

func (sr *SmartRouter) parseThresholdCommand(text string) (float64, PeriodInfo, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimPrefix(text, "over ")
	text = strings.TrimSpace(text)

	parts := strings.Fields(text)
	if len(parts) < 1 {
		return 0, PeriodInfo{}, fmt.Errorf("missing threshold percentage")
	}

	thresholdStr := strings.TrimSuffix(parts[0], "%")
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return 0, PeriodInfo{}, fmt.Errorf("invalid threshold percentage '%s'", parts[0])
	}

	if threshold < 0 || threshold > 1000 {
		return 0, PeriodInfo{}, fmt.Errorf("threshold percentage must be between 0 and 1000")
	}

	var periodInfo PeriodInfo
	if len(parts) >= 2 {
		periodText := strings.Join(parts[1:], " ")
		periodInfo = sr.parsePeriodFromText(periodText, "")
	} else {
		periodInfo = PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	}

	return threshold, periodInfo, nil
}
