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

// setupHTTPRoutes sets up the HTTP routes for the application
func setupHTTPRoutes() {
	// Slack slash command routes
	http.HandleFunc("/slack/daily-update", handleDailyUpdateCommand)
	http.HandleFunc("/slack/weekly-update", handleWeeklyUpdateCommand)
	http.HandleFunc("/slack/monthly-update", handleMonthlyUpdateCommand)

	// Manual trigger routes (for admin/testing)
	http.HandleFunc("/api/daily-update", handleAPITrigger("daily-update"))
	http.HandleFunc("/api/weekly-update", handleAPITrigger("weekly-update"))
	http.HandleFunc("/api/monthly-update", handleAPITrigger("monthly-update"))
	http.HandleFunc("/api/sync-tasks", handleAPITrigger("sync-tasks"))
	http.HandleFunc("/api/sync-time-entries", handleAPITrigger("sync-time-entries"))
	http.HandleFunc("/api/full-sync", handleAPITrigger("full-sync"))

	// Health check and status routes
	http.HandleFunc("/health", handleHealthCheck)
	http.HandleFunc("/status", handleStatusCheck)
	http.HandleFunc("/", handleRootRoute)
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
	logger := NewLogger()

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
	logger := NewLogger()

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
	logger := NewLogger()

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
	logger := NewLogger()

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

// handleAPITrigger returns a handler function for API-triggered commands
func handleAPITrigger(command string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := NewLogger()

		// Only allow POST requests for triggers
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Execute the command based on the type
		switch command {
		case "daily-update":
			go func() {
				logger.Info("API triggered daily update")
				SendDailySlackUpdate()
			}()

		case "weekly-update":
			go func() {
				logger.Info("API triggered weekly update")
				SendWeeklySlackUpdate()
			}()

		case "monthly-update":
			go func() {
				logger.Info("API triggered monthly update")
				SendMonthlySlackUpdate()
			}()

		case "sync-tasks":
			go func() {
				logger.Info("API triggered task sync")
				if err := SyncTasksToDatabase(); err != nil {
					logger.Errorf("Task sync failed: %v", err)
				} else {
					logger.Info("Task sync completed successfully")
				}
			}()

		case "sync-time-entries":
			go func() {
				logger.Info("API triggered time entries sync")
				if err := SyncTimeEntriesToDatabase(); err != nil {
					logger.Errorf("Time entries sync failed: %v", err)
				} else {
					logger.Info("Time entries sync completed successfully")
				}
			}()

		case "full-sync":
			go func() {
				logger.Info("API triggered full sync")
				if err := FullSyncAll(); err != nil {
					logger.Errorf("Full sync failed: %v", err)
				} else {
					logger.Info("Full sync completed successfully")
				}
			}()

		default:
			http.Error(w, "Unknown command", http.StatusBadRequest)
			return
		}

		// Send immediate response
		response := map[string]interface{}{
			"status":  "triggered",
			"command": command,
			"message": fmt.Sprintf("%s has been triggered and is running in the background", command),
		}

		json.NewEncoder(w).Encode(response)
	}
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

// handleStatusCheck handles the /status endpoint
func handleStatusCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	db, err := GetDB()
	dbStatus := "healthy"
	if err != nil {
		dbStatus = "unhealthy: " + err.Error()
	} else if db == nil {
		dbStatus = "unhealthy: database connection is nil"
	}

	response := map[string]interface{}{
		"status":      "running",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"version":     "1.0.0",
		"database":    dbStatus,
		"environment": "go-server",
	}

	json.NewEncoder(w).Encode(response)
}

// handleRootRoute handles requests to the root path
func handleRootRoute(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"service": "Observe-Yor-Estimates",
		"version": "1.0.0",
		"status":  "running",
		"endpoints": map[string]string{
			"/health":                "Health check endpoint",
			"/status":                "Detailed status information",
			"/slack/daily-update":    "Slack daily update command",
			"/slack/weekly-update":   "Slack weekly update command",
			"/slack/monthly-update":  "Slack monthly update command",
			"/api/daily-update":      "API trigger for daily update",
			"/api/weekly-update":     "API trigger for weekly update",
			"/api/monthly-update":    "API trigger for monthly update",
			"/api/sync-tasks":        "API trigger for task sync",
			"/api/sync-time-entries": "API trigger for time entries sync",
			"/api/full-sync":         "API trigger for full sync",
		},
		"documentation": "See README.md for more information",
	}

	json.NewEncoder(w).Encode(response)
}
