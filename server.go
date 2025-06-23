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

// JobRequest represents a queued job for async processing
type JobRequest struct {
	JobID       string            `json:"job_id"`
	JobType     string            `json:"job_type"`
	Parameters  map[string]string `json:"parameters"`
	ResponseURL string            `json:"response_url"`
	UserInfo    string            `json:"user_info"`
	QueuedAt    time.Time         `json:"queued_at"`
}

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
	http.HandleFunc("/slack/process-job", handleJobProcessor)
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

// generateJobID creates a unique job ID
func generateJobID(jobType string) string {
	return fmt.Sprintf("%s_%d", jobType, time.Now().UnixNano())
}

// queueJob queues a job for processing by calling the job processor endpoint
func queueJob(jobType string, parameters map[string]string, responseURL string, userInfo string) error {
	logger := GetGlobalLogger()

	jobRequest := JobRequest{
		JobID:       generateJobID(jobType),
		JobType:     jobType,
		Parameters:  parameters,
		ResponseURL: responseURL,
		UserInfo:    userInfo,
		QueuedAt:    time.Now(),
	}

	// Get the job processor URL - could be same domain or external service
	processorURL := os.Getenv("JOB_PROCESSOR_URL")
	if processorURL == "" {
		// For local testing, use localhost; in production this would be the full Netlify URL
		processorURL = "http://localhost:8080/slack/process-job"
	}

	jsonData, err := json.Marshal(jobRequest)
	if err != nil {
		return fmt.Errorf("error marshaling job request: %w", err)
	}

	// Make async call to job processor
	go func() {
		resp, err := http.Post(processorURL, "application/json", strings.NewReader(string(jsonData)))
		if err != nil {
			logger.Errorf("Failed to queue job %s: %v", jobRequest.JobID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Errorf("Job processor returned status %d for job %s", resp.StatusCode, jobRequest.JobID)
		} else {
			logger.Infof("Successfully queued job %s", jobRequest.JobID)
		}
	}()

	return nil
}

// handleUpdateCommand handles all update-related slash commands with immediate response
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

	// Send immediate response - this is CRUCIAL for Netlify timeouts
	sendImmediateResponse(w, fmt.Sprintf("⏳ Your %s update is being prepared... I'll send the results here in a moment!", period), "ephemeral")

	// Queue the job for processing
	parameters := map[string]string{
		"period": period,
	}
	userInfo := fmt.Sprintf("%s in #%s", req.UserName, req.ChannelName)

	if err := queueJob("slack_update", parameters, req.ResponseURL, userInfo); err != nil {
		logger.Errorf("Failed to queue update job: %v", err)
		// Send error message since we already responded
		go func() {
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: fmt.Sprintf("❌ Failed to queue %s update job. Please try again.", period),
			})
		}()
	}
}

// handleFullSyncCommand handles the /full-sync slash command with immediate response
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

	// Send immediate response - NO timeout waiting!
	sendImmediateResponse(w, "⏳ Full data synchronization has been queued! This usually takes 30-60 seconds. I'll notify you here when it's complete.", "ephemeral")

	// Queue the job for processing
	parameters := map[string]string{
		"sync_type": "full",
	}
	userInfo := fmt.Sprintf("%s in #%s", req.UserName, req.ChannelName)

	if err := queueJob("full_sync", parameters, req.ResponseURL, userInfo); err != nil {
		logger.Errorf("Failed to queue full sync job: %v", err)
		// Send error message since we already responded
		go func() {
			sendDelayedResponse(req.ResponseURL, SlackMessage{
				Text: "❌ Failed to queue full sync job. Please try again.",
			})
		}()
	}
}

// handleJobProcessor processes queued jobs - this runs separately and can take time
func handleJobProcessor(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var jobRequest JobRequest
	if err := json.NewDecoder(r.Body).Decode(&jobRequest); err != nil {
		logger.Errorf("Failed to decode job request: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	logger.Infof("Processing job %s of type %s for %s", jobRequest.JobID, jobRequest.JobType, jobRequest.UserInfo)

	// Respond immediately to the caller that we received the job
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Job %s queued successfully", jobRequest.JobID)))

	// Process the job asynchronously
	go func() {
		processJob(jobRequest)
	}()
}

// processJob handles the actual job processing
func processJob(job JobRequest) {
	logger := GetGlobalLogger()
	startTime := time.Now()

	logger.Infof("Starting processing of job %s", job.JobID)

	switch job.JobType {
	case "slack_update":
		period := job.Parameters["period"]
		if period == "" {
			logger.Errorf("Missing period parameter for slack_update job %s", job.JobID)
			sendJobErrorResponse(job.ResponseURL, "Invalid job configuration")
			return
		}

		// Send initial status update
		sendDelayedResponse(job.ResponseURL, SlackMessage{
			Text: fmt.Sprintf("🔄 Starting %s update generation...", period),
		})

		// Actually do the work
		SendSlackUpdate(period, job.ResponseURL, false)

	case "full_sync":
		// Send initial status update
		sendDelayedResponse(job.ResponseURL, SlackMessage{
			Text: "🔄 Starting full synchronization...",
		})

		// Actually do the work
		if err := FullSyncAll(); err != nil {
			logger.Errorf("Full sync failed in job %s: %v", job.JobID, err)
			sendDelayedResponse(job.ResponseURL, SlackMessage{
				Text: fmt.Sprintf("❌ Full sync failed: %v", err),
			})
		} else {
			duration := time.Since(startTime)
			logger.Infof("Full sync completed successfully in job %s (took %v)", job.JobID, duration)
			sendDelayedResponse(job.ResponseURL, SlackMessage{
				Text: fmt.Sprintf("✅ Full data synchronization completed successfully! (completed in %v)", duration.Round(time.Second)),
			})
		}

	default:
		logger.Errorf("Unknown job type: %s", job.JobType)
		sendJobErrorResponse(job.ResponseURL, "Unknown job type")
	}

	duration := time.Since(startTime)
	logger.Infof("Completed processing job %s in %v", job.JobID, duration)
}

// sendJobErrorResponse sends an error response for failed jobs
func sendJobErrorResponse(responseURL string, errorMsg string) {
	if responseURL != "" {
		sendDelayedResponse(responseURL, SlackMessage{
			Text: fmt.Sprintf("❌ Job failed: %s", errorMsg),
		})
	}
}

// handleHealthCheck handles a simple health check endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	// Test database connectivity quickly
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
