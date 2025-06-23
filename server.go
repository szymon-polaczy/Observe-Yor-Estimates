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
	http.HandleFunc("/slack/update", handleUpdateCommand)
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

// handleUpdateCommand handles all update-related slash commands
func handleUpdateCommand(w http.ResponseWriter, r *http.Request) {
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

	// Extract period from command
	var period string
	command := req.Command
	if strings.HasSuffix(command, "-update") {
		period = strings.TrimSuffix(command[1:], "-update")
	} else {
		logger.Errorf("Unknown update command: %s", command)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	logger.Infof("Received %s update command from user %s in channel %s", period, req.UserName, req.ChannelName)

	// Send immediate acknowledgment
	sendImmediateResponse(w, fmt.Sprintf("⏳ Generating %s update...", period), "ephemeral")

	// Process the command asynchronously
	go func() {
		SendSlackUpdate(period, req.ResponseURL, false)
	}()
}

// handleFullSyncCommand handles the /full-sync slash command from Slack
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
		logger.Errorf("Failed to verify slack request: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /full-sync command from user %s in channel %s", req.UserName, req.ChannelName)

	// Quick database connectivity check before starting
	_, dbErr := GetDB()
	if dbErr != nil {
		logger.Errorf("Database connection failed: %v", dbErr)
		sendImmediateResponse(w, fmt.Sprintf("❌ Cannot start full sync: Database connection failed - %v", dbErr), "ephemeral")
		return
	}

	// Send immediate acknowledgment
	sendImmediateResponse(w, "⏳ Starting full data synchronization... This will run in the background and you'll be notified when complete.", "ephemeral")

	// Run full sync asynchronously with timeout protection
	go func() {
		// Create a timeout context for the full sync operation
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		done := make(chan error, 1)

		// Run the sync in another goroutine to enable timeout
		go func() {
			done <- FullSyncAll()
		}()

		select {
		case err := <-done:
			if err != nil {
				logger.Errorf("Full sync failed: %v", err)
				sendDelayedResponse(req.ResponseURL, SlackMessage{
					Text: fmt.Sprintf("❌ Full sync failed: %v", err),
				})
			} else {
				logger.Info("Full sync completed successfully via slash command")
				sendDelayedResponse(req.ResponseURL, SlackMessage{
					Text: "✅ Full data synchronization completed successfully!",
				})
			}
		case <-ctx.Done():
			logger.Error("Full sync timed out after 20 seconds")
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "⚠️ Full sync timed out. This may indicate database connectivity issues or the operation is taking longer than expected. Please check the logs and try again.",
			})
		}
	}()
}

// handleHealthCheck handles a simple health check endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	// Test database connectivity
	_, err := GetDB()
	if err != nil {
		logger.Errorf("Health check failed - Database connection error: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Database connection failed: %v", err)))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK - Database connected"))
}
