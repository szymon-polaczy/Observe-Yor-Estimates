package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// SlackCommandRequest represents a Slack slash command request
type SlackCommandRequest struct {
	Token       string `json:"token"`
	TeamID      string `json:"team_id"`
	TeamDomain  string `json:"team_domain"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	Command     string `json:"command"`
	Text        string `json:"text"`
	ResponseURL string `json:"response_url"`
	TriggerID   string `json:"trigger_id"`
}

// SlackCommandResponse represents a response to a Slack slash command
type SlackCommandResponse struct {
	ResponseType string  `json:"response_type"`
	Text         string  `json:"text"`
	Blocks       []Block `json:"blocks,omitempty"`
}

// setupSlackRoutes sets up the HTTP routes for Slack slash commands
func setupSlackRoutes() {
	http.HandleFunc("/slack/daily-update", handleDailyUpdateCommand)
	http.HandleFunc("/slack/weekly-update", handleWeeklyUpdateCommand)
	http.HandleFunc("/slack/monthly-update", handleMonthlyUpdateCommand)
	http.HandleFunc("/slack/full-sync", handleFullSyncCommand)
	http.HandleFunc("/health", handleHealthCheck)
}

// parseSlackCommand parses the form data from a Slack slash command
func parseSlackCommand(r *http.Request) (*SlackCommandRequest, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, fmt.Errorf("failed to parse form data: %w", err)
	}

	return &SlackCommandRequest{
		Token:       r.FormValue("token"),
		TeamID:      r.FormValue("team_id"),
		TeamDomain:  r.FormValue("team_domain"),
		ChannelID:   r.FormValue("channel_id"),
		ChannelName: r.FormValue("channel_name"),
		UserID:      r.FormValue("user_id"),
		UserName:    r.FormValue("user_name"),
		Command:     r.FormValue("command"),
		Text:        r.FormValue("text"),
		ResponseURL: r.FormValue("response_url"),
		TriggerID:   r.FormValue("trigger_id"),
	}, nil
}

// verifySlackRequest verifies that the request is from Slack
func verifySlackRequest(req *SlackCommandRequest) error {
	expectedToken := os.Getenv("SLACK_VERIFICATION_TOKEN")
	if expectedToken == "" {
		// If no verification token is set, skip verification (not recommended for production)
		return nil
	}

	if req.Token != expectedToken {
		return fmt.Errorf("invalid verification token")
	}

	return nil
}

// sendImmediateResponse sends an immediate response to Slack
func sendImmediateResponse(w http.ResponseWriter, message string, responseType string) {
	if responseType == "" {
		responseType = "ephemeral" // Only visible to the user who ran the command
	}

	response := SlackCommandResponse{
		ResponseType: responseType,
		Text:         message,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// sendDelayedResponse sends a delayed response to Slack using the response URL
func sendDelayedResponse(responseURL string, message SlackMessage) error {
	logger := GetGlobalLogger()

	// Convert SlackMessage to SlackCommandResponse format
	response := SlackCommandResponse{
		ResponseType: "in_channel", // Visible to everyone in the channel
		Text:         message.Text,
		Blocks:       message.Blocks,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	resp, err := http.Post(responseURL, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("error sending delayed response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned status %d", resp.StatusCode)
	}

	logger.Debug("Successfully sent delayed response to Slack")
	return nil
}

// handleDailyUpdateCommand handles the /daily-update slash command
func handleDailyUpdateCommand(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSlackCommand(r)
	if err != nil {
		logger.Errorf("Failed to parse slash command: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := verifySlackRequest(req); err != nil {
		logger.Errorf("Failed to verify Slack request: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /daily-update command from user %s in channel %s", req.UserName, req.ChannelName)

	// Send immediate acknowledgment
	sendImmediateResponse(w, "⏳ Generating daily update...", "ephemeral")

	// Process the command asynchronously
	go func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to connect to database",
			})
			return
		}

		taskInfos, err := getTaskTimeChanges(db)
		if err != nil {
			logger.Errorf("Failed to get task time changes: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to retrieve task changes",
			})
			return
		}

		if len(taskInfos) == 0 {
			message := SlackMessage{
				Text: "📊 No task changes to report today",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: "📊 *Daily Task Update*\n\nNo task changes to report today. System is working normally.",
						},
					},
				},
			}
			sendDelayedResponse(req.ResponseURL, message)
			return
		}

		message := formatDailySlackMessage(taskInfos)
		if err := sendDelayedResponse(req.ResponseURL, message); err != nil {
			logger.Errorf("Failed to send delayed response: %v", err)
		} else {
			logger.Info("Successfully sent daily update via slash command")
		}
	}()
}

// handleWeeklyUpdateCommand handles the /weekly-update slash command
func handleWeeklyUpdateCommand(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSlackCommand(r)
	if err != nil {
		logger.Errorf("Failed to parse slash command: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := verifySlackRequest(req); err != nil {
		logger.Errorf("Failed to verify Slack request: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /weekly-update command from user %s in channel %s", req.UserName, req.ChannelName)

	// Send immediate acknowledgment
	sendImmediateResponse(w, "⏳ Generating weekly update...", "ephemeral")

	// Process the command asynchronously
	go func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to connect to database",
			})
			return
		}

		taskInfos, err := getWeeklyTaskChanges(db)
		if err != nil {
			logger.Errorf("Failed to get weekly task changes: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to retrieve weekly task changes",
			})
			return
		}

		if len(taskInfos) == 0 {
			message := SlackMessage{
				Text: "📈 No task changes to report this week",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: "📈 *Weekly Task Summary*\n\nNo task changes to report this week. System is working normally.",
						},
					},
				},
			}
			sendDelayedResponse(req.ResponseURL, message)
			return
		}

		message := formatWeeklySlackMessage(taskInfos)
		if err := sendDelayedResponse(req.ResponseURL, message); err != nil {
			logger.Errorf("Failed to send delayed response: %v", err)
		} else {
			logger.Info("Successfully sent weekly update via slash command")
		}
	}()
}

// handleMonthlyUpdateCommand handles the /monthly-update slash command
func handleMonthlyUpdateCommand(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSlackCommand(r)
	if err != nil {
		logger.Errorf("Failed to parse slash command: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := verifySlackRequest(req); err != nil {
		logger.Errorf("Failed to verify Slack request: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /monthly-update command from user %s in channel %s", req.UserName, req.ChannelName)

	// Send immediate acknowledgment
	sendImmediateResponse(w, "⏳ Generating monthly update...", "ephemeral")

	// Process the command asynchronously
	go func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to connect to database",
			})
			return
		}

		taskInfos, err := getMonthlyTaskChanges(db)
		if err != nil {
			logger.Errorf("Failed to get monthly task changes: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Failed to retrieve monthly task changes",
			})
			return
		}

		if len(taskInfos) == 0 {
			message := SlackMessage{
				Text: "📅 No task changes to report this month",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: "📅 *Monthly Task Summary*\n\nNo task changes to report this month. System is working normally.",
						},
					},
				},
			}
			sendDelayedResponse(req.ResponseURL, message)
			return
		}

		message := formatMonthlySlackMessage(taskInfos)
		if err := sendDelayedResponse(req.ResponseURL, message); err != nil {
			logger.Errorf("Failed to send delayed response: %v", err)
		} else {
			logger.Info("Successfully sent monthly update via slash command")
		}
	}()
}

// handleFullSyncCommand handles the /full-sync slash command
func handleFullSyncCommand(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSlackCommand(r)
	if err != nil {
		logger.Errorf("Failed to parse slash command: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := verifySlackRequest(req); err != nil {
		logger.Errorf("Failed to verify Slack request: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /full-sync command from user %s in channel %s", req.UserName, req.ChannelName)

	// Send immediate acknowledgment
	sendImmediateResponse(w, "⏳ Starting full synchronization... This may take up to 60 seconds.", "ephemeral")

	// Process the command asynchronously
	go func() {
		logger.Info("Starting full sync operation via slash command")

		// Pre-flight checks for Netlify environment
		apiKey := os.Getenv("TIMECAMP_API_KEY")
		if apiKey == "" {
			logger.Error("TIMECAMP_API_KEY environment variable not set")
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Configuration Error: Missing API key",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: "❌ *Configuration Error*\n\n`TIMECAMP_API_KEY` environment variable is not set in Netlify deployment.\n\n*Action Required:*\n• Set `TIMECAMP_API_KEY` in Netlify environment variables\n• Redeploy the application\n• Contact administrator if issue persists",
						},
					},
				},
			})
			return
		}

		// Test database connection
		_, err := GetDB()
		if err != nil {
			logger.Errorf("Database connection failed: %v", err)
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Database Error: Cannot connect to database",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: fmt.Sprintf("❌ *Database Connection Failed*\n\nError: `%v`\n\n*Possible Causes:*\n• Database file missing from deployment\n• Insufficient permissions\n• Storage quota exceeded\n\n*Time:* %s", err, time.Now().Format("2006-01-02 15:04:05")),
						},
					},
				},
			})
			return
		}

		if err := FullSyncAll(); err != nil {
			logger.Errorf("Full sync failed: %v", err)

			// Provide more specific error messages based on error content
			var errorText string
			var troubleshootingText string

			if strings.Contains(err.Error(), "TIMECAMP_API_KEY") {
				errorText = "❌ *API Configuration Error*"
				troubleshootingText = "*Action Required:*\n• Verify `TIMECAMP_API_KEY` is set in Netlify environment variables\n• Ensure the API key is valid and has proper permissions\n• Check TimeCamp account status"
			} else if strings.Contains(err.Error(), "readonly database") || strings.Contains(err.Error(), "attempt to write a readonly database") {
				errorText = "❌ *Database Permission Error*"
				troubleshootingText = "*Netlify Database Issue:*\n• Database file is read-only in Netlify serverless environment\n• Netlify functions run in ephemeral containers with read-only filesystem\n• Full sync operations require persistent storage\n\n*Solutions:*\n• Use external database (PostgreSQL, MySQL) for production\n• Consider database initialization during build process\n• Contact administrator to configure persistent storage"
			} else if strings.Contains(err.Error(), "all task operations failed") {
				errorText = "❌ *Task Sync Failed*"
				troubleshootingText = "*Possible Causes:*\n• Database permission problems (check if read-only)\n• Invalid or expired API key\n• TimeCamp API rate limiting\n• Network connectivity issues\n• TimeCamp service unavailable"
			} else if strings.Contains(err.Error(), "HTTP request") || strings.Contains(err.Error(), "failed after retries") {
				errorText = "❌ *Network Error*"
				troubleshootingText = "*Possible Causes:*\n• TimeCamp API is temporarily unavailable\n• Network connectivity issues in Netlify\n• API rate limiting\n• Firewall or security restrictions"
			} else {
				errorText = "❌ *Full Sync Failed*"
				troubleshootingText = "*Troubleshooting:*\n• Check Netlify function logs for detailed errors\n• Verify all environment variables are set\n• Try again in a few minutes\n• Contact administrator if issue persists"
			}

			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Error: Full synchronization failed",
				Blocks: []Block{
					{
						Type: "section",
						Text: &Text{
							Type: "mrkdwn",
							Text: fmt.Sprintf("%s\n\nError: `%v`\n\n%s\n\n*Time:* %s", errorText, err, troubleshootingText, time.Now().Format("2006-01-02 15:04:05")),
						},
					},
				},
			})
			return
		}

		// Send success message
		message := SlackMessage{
			Text: "✅ Full synchronization completed successfully",
			Blocks: []Block{
				{
					Type: "header",
					Text: &Text{
						Type: "plain_text",
						Text: "✅ Full Sync Complete",
					},
				},
				{
					Type: "section",
					Text: &Text{
						Type: "mrkdwn",
						Text: fmt.Sprintf("*Full synchronization completed successfully*\n\n• All tasks synced from TimeCamp\n• Time entries synced (last 6 months)\n• Database is now up to date\n\n*Completed at:* %s", time.Now().Format("2006-01-02 15:04:05")),
					},
				},
			},
		}

		if err := sendDelayedResponse(req.ResponseURL, message); err != nil {
			logger.Errorf("Failed to send delayed response: %v", err)
		} else {
			logger.Info("Successfully sent full sync completion message via slash command")
		}
	}()
}

// handleHealthCheck provides a simple health check endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
