package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

func StartServer(logger *Logger) {
	// Setup handlers in this file
	setupSlackRoutes()

	server := &http.Server{Addr: ":8080"}

	// Goroutine for graceful shutdown
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Fatalf("Server shutdown failed: %v", err)
		}
	}()

	logger.Info("Server is starting on port 8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Could not start server: %v", err)
	}
}

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
	ProjectName string `json:"project_name,omitempty"` // Parsed project name for filtering
	TriggerID   string `json:"trigger_id"`
}

// SlackCommandResponse represents a response to a Slack slash command
type SlackCommandResponse struct {
	ResponseType string  `json:"response_type"`
	Text         string  `json:"text"`
	Blocks       []Block `json:"blocks,omitempty"`
}

// Global smart router instance
var globalRouter *SmartRouter

// setupSlackRoutes sets up the HTTP routes for Slack slash commands
func setupSlackRoutes() {
	// Initialize the smart router
	globalRouter = NewSmartRouter()

	// Unified handler for all OYE commands
	http.HandleFunc("/slack/oye", handleUnifiedOYECommand)
	http.HandleFunc("/health", handleHealthCheck)

	// New App Home routes
	http.HandleFunc("/slack/events", HandleAppHome)
	http.HandleFunc("/slack/interactive", HandleInteractiveComponents)
}

// handleUnifiedOYECommand handles the new unified /oye command
func handleUnifiedOYECommand(w http.ResponseWriter, r *http.Request) {
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

	logger.Infof("Received /oye command from user %s: %s", req.UserName, req.Text)

	text := strings.ToLower(strings.TrimSpace(req.Text))

	// Check for project assignment commands FIRST (before project parsing)
	if strings.HasPrefix(text, "assign ") || strings.HasPrefix(text, "unassign ") ||
		text == "my-projects" || text == "available-projects" {
		if err := globalRouter.HandleProjectAssignmentRequest(req); err != nil {
			logger.Errorf("Failed to handle project assignment request: %v", err)
			sendImmediateResponse(w, "❌ Failed to process project assignment request", "ephemeral")
		} else {
			sendImmediateResponse(w, "✅ Processing your project assignment...", "ephemeral")
		}
		return
	}

	// Parse project name from command if present (only after checking for management commands)
	projectName, remainingText := ParseProjectFromCommand(req.Text)

	// Update the request with parsed project info
	if projectName != "" && projectName != "all" {
		req.ProjectName = projectName
		req.Text = remainingText                            // Update text to remaining command after project name
		text = strings.ToLower(strings.TrimSpace(req.Text)) // Update text variable too
	}

	// Route to appropriate handler based on command content
	if text == "" || text == "help" {
		// If no remaining text after project name, treat as update request
		if projectName != "" && projectName != "all" {
			// Project-specific update request with default period
			if err := globalRouter.HandleUpdateRequest(req); err != nil {
				logger.Errorf("Failed to handle project update request: %v", err)
				sendImmediateResponse(w, "❌ Failed to process project update request", "ephemeral")
			} else {
				sendImmediateResponse(w, fmt.Sprintf("⏳ Generating %s project update...", projectName), "ephemeral")
			}
			return
		}

		sendUnifiedHelp(w, req)
		return
	}

	if strings.Contains(text, "sync") || text == "full-sync" {
		// Handle full sync request
		if err := globalRouter.HandleFullSyncRequest(req); err != nil {
			logger.Errorf("Failed to handle full sync request: %v", err)
			sendImmediateResponse(w, "❌ Failed to process sync request", "ephemeral")
		} else {
			sendImmediateResponse(w, "⏳ Full sync started! I'll update you with progress...", "ephemeral")
		}
		return
	}

	if strings.Contains(text, "over ") {
		// Handle threshold percentage queries like "over 50 daily"
		if err := globalRouter.HandleThresholdRequest(req); err != nil {
			logger.Errorf("Failed to handle threshold request: %v", err)
			sendImmediateResponse(w, "❌ Failed to process threshold request", "ephemeral")
		} else {
			sendImmediateResponse(w, "⏳ Checking for tasks over threshold...", "ephemeral")
		}
		return
	}

	// Default to update request (daily, weekly, monthly, or user's default)
	if err := globalRouter.HandleUpdateRequest(req); err != nil {
		logger.Errorf("Failed to handle update request: %v", err)
		sendImmediateResponse(w, "❌ Failed to process update request", "ephemeral")
	} else {
		sendImmediateResponse(w, "⏳ Generating your update! I'll show progress as I work...", "ephemeral")
	}
}

func sendUnifiedHelp(w http.ResponseWriter, req *SlackCommandRequest) {
	helpText := "*🎯 OYE (Observe-Yor-Estimates) Commands*\n\n" +
		"*Time Frame Options:*\n" +
		"• `/oye` or `/oye daily` - Yesterday's tasks (default)\n" +
		"• `/oye today` - Today's tasks\n" +
		"• `/oye yesterday` - Yesterday's tasks\n" +
		"• `/oye weekly` or `/oye last week` - Last week's tasks\n" +
		"• `/oye this week` - Current week's tasks\n" +
		"• `/oye monthly` or `/oye last month` - Last month's tasks\n" +
		"• `/oye this month` - Current month's tasks\n" +
		"• `/oye last 7 days` - Custom range (1-60 days)\n\n" +
		"*Project Management:*\n" +
		"• `/oye assign \"Project Name\"` - Assign yourself to a project\n" +
		"• `/oye unassign \"Project Name\"` - Remove yourself from a project\n" +
		"• `/oye my-projects` - View your assigned projects\n" +
		"• `/oye available-projects` - View all available projects\n\n" +
		"*Project Filtering:*\n" +
		"• `/oye \"project name\" daily` - Daily update for specific project\n" +
		"• `/oye marketing last week` - Weekly update for project (fuzzy match)\n" +
		"• `/oye all this month` - Monthly update for all projects\n" +
		"• `/oye \"3dconnexion\" over 90 last 30 days` - Project-specific thresholds\n\n" +
		"*Threshold Monitoring:*\n" +
		"• `/oye over 50 today` - Tasks over 50% of estimation\n" +
		"• `/oye over 80 this week` - Tasks over 80% of estimation\n" +
		"• `/oye over 100 last month` - Tasks over budget\n\n" +
		"*Data Management:*\n" +
		"• `/oye sync` - Full data synchronization\n\n" +
		"*Tips:*\n" +
		"• Updates are private by default (only you see them)\n" +
		"• Use \"public\" in any command to share with channel\n" +
		"• Quote project names with spaces: `/oye \"My Project\" today`\n" +
		"• Project names support fuzzy matching\n" +
		"• Custom ranges: `/oye last 14 days` (1-60 days supported)\n" +
		"• When you assign projects, automatic updates show only your projects\n" +
		"• Click the OYE app in sidebar to see your project settings page"

	response := SlackCommandResponse{
		ResponseType: "ephemeral",
		Text:         helpText,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

// handleHealthCheck handles a simple health check endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	// Quick health check without blocking on database
	_, err := GetDB()
	if err != nil {
		logger.Warnf("Health check - Database not yet ready: %v", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK - Initializing (database connecting...)"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK - Database connected"))
}
