package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type UserPreferences struct {
	UserID          string
	PreferredFormat string // "detailed", "summary", "personal"
	DefaultPeriod   string
	NotifyInChannel bool
	LastInteraction time.Time
}

type SmartRouter struct {
	slackClient *SlackAPIClient
	preferences map[string]*UserPreferences
	mutex       sync.RWMutex
	logger      *Logger
}

func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		slackClient: NewSlackAPIClient(),
		preferences: make(map[string]*UserPreferences),
		logger:      GetGlobalLogger(),
	}
}

// HandleUpdateRequest processes update requests with intelligent routing
func (sr *SmartRouter) HandleUpdateRequest(req *SlackCommandRequest) error {
	sr.mutex.Lock()
	prefs := sr.getUserPreferences(req.UserID)
	sr.mutex.Unlock()

	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
	}

	// Determine period from command or user preference
	period := sr.determinePeriod(req.Text, req.Command, prefs)

	sr.logger.Infof("Processing %s update request from user %s in channel %s", period, req.UserName, req.ChannelName)

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
	sr.slackClient.UpdateProgress(ctx, "📊 Querying database...")
	time.Sleep(500 * time.Millisecond)

	// Get database connection
	db, err := GetDB()
	if err != nil {
		sr.slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	sr.slackClient.UpdateProgress(ctx, "📈 Analyzing time entries...")
	time.Sleep(1 * time.Second)

	// Get task data
	taskInfos, err := getTaskChanges(db, period)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to get %s changes: ```%v```", period, err)
		sr.slackClient.SendErrorResponse(ctx, errorMessage)
		return
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

	// Send final result based on user preferences
	prefs := sr.getUserPreferences(ctx.UserID)

	// Always try to send to the thread first for better UX
	if prefs.NotifyInChannel {
		// Send public message in thread
		err = sr.slackClient.SendFinalUpdate(ctx, taskInfos, period)
	} else {
		// For personal messages, try to send in thread if possible, otherwise fallback
		// Note: Ephemeral messages don't support threading, so we'll send a regular message
		// in the thread but make it clear it's for the specific user
		err = sr.slackClient.SendPersonalUpdateInThread(ctx, taskInfos, period)
	}

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

// HandleConfigRequest allows users to set preferences
func (sr *SmartRouter) HandleConfigRequest(req *SlackCommandRequest) error {
	ctx := &ConversationContext{
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		CommandType: req.Command,
	}

	text := strings.ToLower(strings.TrimSpace(req.Text))

	sr.mutex.Lock()
	prefs := sr.getUserPreferences(req.UserID)

	// Parse configuration commands
	if strings.Contains(text, "public") || strings.Contains(text, "channel") {
		prefs.NotifyInChannel = true
		sr.preferences[req.UserID] = prefs
		sr.mutex.Unlock()
		return sr.slackClient.SendPersonalUpdate(ctx, nil, "config")
	} else if strings.Contains(text, "private") || strings.Contains(text, "personal") {
		prefs.NotifyInChannel = false
		sr.preferences[req.UserID] = prefs
		sr.mutex.Unlock()
		return sr.slackClient.SendPersonalUpdate(ctx, nil, "config")
	}
	sr.mutex.Unlock()

	// Show current preferences
	notificationStyle := "Private (only you)"
	if prefs.NotifyInChannel {
		notificationStyle = "Public (visible to channel)"
	}

	configMessage := fmt.Sprintf(`📋 *Your Current Preferences:*

• *Default Period:* %s
• *Notification Style:* %s
• *Last Updated:* %s

*To change settings, use:*
• /oye config public - Show updates in channel
• /oye config private - Show updates privately
• /oye config daily - Set daily as default
• /oye config weekly - Set weekly as default`,
		prefs.DefaultPeriod,
		notificationStyle,
		prefs.LastInteraction.Format("Jan 2, 2006 3:04 PM"))

	return sr.slackClient.SendErrorResponse(ctx, configMessage)
}

func (sr *SmartRouter) determinePeriod(text, command string, prefs *UserPreferences) string {
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

	// Fall back to user preference or default
	if prefs.DefaultPeriod != "" {
		return prefs.DefaultPeriod
	}

	return "daily" // ultimate fallback
}

func (sr *SmartRouter) getUserPreferences(userID string) *UserPreferences {
	if prefs, exists := sr.preferences[userID]; exists {
		prefs.LastInteraction = time.Now()
		return prefs
	}

	// Default preferences for new users - use channel notifications for better threading support
	return &UserPreferences{
		UserID:          userID,
		PreferredFormat: "detailed",
		DefaultPeriod:   "daily",
		NotifyInChannel: true, // Changed to true for better threading support
		LastInteraction: time.Now(),
	}
}

// SetUserPreference updates a user's preference
func (sr *SmartRouter) SetUserPreference(userID, key, value string) {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	prefs := sr.getUserPreferences(userID)

	switch key {
	case "default_period":
		if value == "daily" || value == "weekly" || value == "monthly" {
			prefs.DefaultPeriod = value
		}
	case "notify_in_channel":
		prefs.NotifyInChannel = value == "true"
	case "preferred_format":
		if value == "detailed" || value == "summary" || value == "personal" {
			prefs.PreferredFormat = value
		}
	}

	prefs.LastInteraction = time.Now()
	sr.preferences[userID] = prefs
}
