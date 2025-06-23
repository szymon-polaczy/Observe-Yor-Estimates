package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Standalone job processor that can be run separately from the main server
// This can run on a different service, server, or as a separate Netlify function

func processJobsMain() {
	// This function can be called from main.go to run the job processor
	runJobProcessor()
}

// runJobProcessor runs the standalone job processor server
func runJobProcessor() {
	logger := NewLogger()
	logger.Info("Starting standalone job processor")

	// Setup environment
	err := validateRequiredEnvVars()
	if err != nil {
		logger.Fatalf("Missing required environment variables: %v", err)
	}

	// Setup HTTP handler
	http.HandleFunc("/process-job", handleStandaloneJobProcessor)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Job processor healthy"))
	})

	port := os.Getenv("JOB_PROCESSOR_PORT")
	if port == "" {
		port = "8081"
	}

	logger.Infof("Job processor listening on port %s", port)
	logger.Fatal(http.ListenAndServe(":"+port, nil))
}

// handleStandaloneJobProcessor handles job processing requests
func handleStandaloneJobProcessor(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Errorf("Failed to read request body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var jobRequest JobRequest
	if err := json.Unmarshal(body, &jobRequest); err != nil {
		logger.Errorf("Failed to decode job request: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	logger.Infof("Received job %s of type %s for %s", jobRequest.JobID, jobRequest.JobType, jobRequest.UserInfo)

	// Respond immediately
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Job %s accepted for processing", jobRequest.JobID)))

	// Process job in background
	go processJobStandalone(jobRequest)
}

// processJobStandalone processes jobs in the standalone processor
func processJobStandalone(job JobRequest) {
	logger := GetGlobalLogger()
	startTime := time.Now()

	logger.Infof("Processing standalone job %s", job.JobID)

	// Send initial progress update
	sendJobStartedMessage(job)

	switch job.JobType {
	case "slack_update":
		processSlackUpdateJob(job)
	case "full_sync":
		processFullSyncJob(job)
	default:
		logger.Errorf("Unknown job type: %s", job.JobType)
		sendJobError(job, "Unknown job type")
		return
	}

	duration := time.Since(startTime)
	logger.Infof("Completed standalone job %s in %v", job.JobID, duration)
}

// processSlackUpdateJob handles Slack update jobs
func processSlackUpdateJob(job JobRequest) {
	logger := GetGlobalLogger()

	period := job.Parameters["period"]
	if period == "" {
		logger.Errorf("Missing period parameter for job %s", job.JobID)
		sendJobError(job, "Missing period parameter")
		return
	}

	logger.Infof("Processing %s update for job %s", period, job.JobID)

	// Send progress update
	sendProgressUpdate(job, fmt.Sprintf("🔄 Generating %s update...", period))

	// Perform the actual work
	SendSlackUpdate(period, job.ResponseURL, false)

	logger.Infof("Completed %s update for job %s", period, job.JobID)
}

// processFullSyncJob handles full sync jobs
func processFullSyncJob(job JobRequest) {
	logger := GetGlobalLogger()
	startTime := time.Now()

	logger.Infof("Processing full sync for job %s", job.JobID)

	// Send progress updates
	sendProgressUpdate(job, "🔄 Starting full synchronization...")

	// Validate database first
	_, err := GetDB()
	if err != nil {
		logger.Errorf("Database connection failed for job %s: %v", job.JobID, err)
		sendJobError(job, fmt.Sprintf("Database connection failed: %v", err))
		return
	}

	// Send more detailed progress
	sendProgressUpdate(job, "📊 Syncing tasks from TimeCamp...")

	// Perform the actual sync
	if err := FullSyncAll(); err != nil {
		logger.Errorf("Full sync failed for job %s: %v", job.JobID, err)
		sendJobError(job, fmt.Sprintf("Full sync failed: %v", err))
		return
	}

	// Send completion message
	duration := time.Since(startTime)
	message := SlackMessage{
		Text: fmt.Sprintf("✅ Full data synchronization completed successfully! (took %v)", duration.Round(time.Second)),
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
					Text: fmt.Sprintf("*Full synchronization completed successfully*\n\n• All tasks synced from TimeCamp\n• Time entries synced (last 6 months)\n• Database is now up to date\n\n*Duration:* %v\n*Completed at:* %s",
						duration.Round(time.Second),
						time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	if err := sendDelayedResponse(job.ResponseURL, message); err != nil {
		logger.Errorf("Failed to send completion message for job %s: %v", job.JobID, err)
	}

	logger.Infof("Completed full sync for job %s in %v", job.JobID, duration)
}

// sendJobStartedMessage sends initial job started message
func sendJobStartedMessage(job JobRequest) {
	if job.ResponseURL == "" {
		return
	}

	var message string
	switch job.JobType {
	case "slack_update":
		period := job.Parameters["period"]
		message = fmt.Sprintf("🚀 Starting %s update generation...", period)
	case "full_sync":
		message = "🚀 Starting full data synchronization..."
	default:
		message = "🚀 Starting job processing..."
	}

	sendProgressUpdate(job, message)
}

// sendProgressUpdate sends a progress update message
func sendProgressUpdate(job JobRequest, message string) {
	if job.ResponseURL == "" {
		return
	}

	slackMessage := SlackMessage{
		Text: message,
	}

	if err := sendDelayedResponse(job.ResponseURL, slackMessage); err != nil {
		logger := GetGlobalLogger()
		logger.Errorf("Failed to send progress update for job %s: %v", job.JobID, err)
	}
}

// sendJobError sends an error message for failed jobs
func sendJobError(job JobRequest, errorMsg string) {
	if job.ResponseURL == "" {
		return
	}

	message := SlackMessage{
		Text: fmt.Sprintf("❌ Job failed: %s", errorMsg),
		Blocks: []Block{
			{
				Type: "section",
				Text: &Text{
					Type: "mrkdwn",
					Text: fmt.Sprintf("❌ *Job Failed*\n\nJob ID: `%s`\nError: %s\n*Time:* %s",
						job.JobID,
						errorMsg,
						time.Now().Format("2006-01-02 15:04:05")),
				},
			},
		},
	}

	if err := sendDelayedResponse(job.ResponseURL, message); err != nil {
		logger := GetGlobalLogger()
		logger.Errorf("Failed to send error message for job %s: %v", job.JobID, err)
	}
}
