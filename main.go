package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"

	"net/http"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

type SocketURLResponse struct {
	Ok  bool   `json:"ok"`
	Url string `json:"url"`
}

type TestSlackPayload struct {
	EnvelopeID string `json:"envelope_id"`
}

type Payload struct {
	Blocks []Block `json:"blocks"`
}

type Block struct {
	Type      string     `json:"type"`
	Text      *Text      `json:"text,omitempty"`
	Fields    []Field    `json:"fields,omitempty"`
	Elements  []Element  `json:"elements,omitempty"`
	Accessory *Accessory `json:"accessory,omitempty"`
}

type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type TestSlackPayloadResponse struct {
	EnvelopeID string  `json:"envelope_id"`
	Payload    Payload `json:"payload"`
}

func main() {
	// Initialize logger
	logger := NewLogger()

	// Load environment variables - this is critical, so we panic if it fails
	err := godotenv.Load()
	if err != nil {
		logger.Fatalf("Critical error: Failed to load .env file: %v", err)
	}

	// Validate required environment variables
	if err := validateRequiredEnvVars(); err != nil {
		logger.Fatalf("Critical error: Missing required environment variables: %v", err)
	}

	logger.Info("Starting Observe-Yor-Estimates application")

	// Initialize database connection once at startup
	_, err = GetDB()
	if err != nil {
		logger.Fatalf("Critical error: Failed to initialize database: %v", err)
	}
	logger.Info("Database connection initialized successfully")

	// Check for command line arguments
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "daily-update":
			logger.Info("Running daily update command")
			SendDailySlackUpdate()
			return
		case "sync-time-entries":
			logger.Info("Running time entries sync command")
			if err := SyncTimeEntriesToDatabase(); err != nil {
				logger.Errorf("Time entries sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Time entries sync completed successfully")
			return
		case "sync-tasks":
			logger.Info("Running tasks sync command")
			if err := SyncTasksToDatabase(); err != nil {
				logger.Errorf("Tasks sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Tasks sync completed successfully")
			return
		default:
			logger.Warnf("Unknown command line argument: %s", os.Args[1])
			logger.Info("Available commands: daily-update, sync-time-entries, sync-tasks")
			return
		}
	}

	// Run initial sync - log errors but don't crash the app
	logger.Info("Running initial task sync")
	if err := SyncTasksToDatabase(); err != nil {
		logger.Errorf("Failed initial task sync: %v", err)
		// Continue running - we can retry later via cron
	}

	// Set up cron scheduler
	cronScheduler := cron.New()

	// Get cron schedules from environment variables or use defaults
	taskSyncSchedule := os.Getenv("TASK_SYNC_SCHEDULE")
	if taskSyncSchedule == "" {
		taskSyncSchedule = "*/5 * * * *" // default: every 5 minutes
	}

	timeEntriesSyncSchedule := os.Getenv("TIME_ENTRIES_SYNC_SCHEDULE")
	if timeEntriesSyncSchedule == "" {
		timeEntriesSyncSchedule = "*/10 * * * *" // default: every 10 minutes
	}

	dailyUpdateSchedule := os.Getenv("DAILY_UPDATE_SCHEDULE")
	if dailyUpdateSchedule == "" {
		dailyUpdateSchedule = "0 6 * * *" // default: 6 AM daily
	}

	// Schedule SyncTasksToDatabase to run based on configured schedule
	_, err = cronScheduler.AddFunc(taskSyncSchedule, func() {
		logger.Debug("Running scheduled task sync")
		if err := SyncTasksToDatabase(); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule task sync cron job: %v", err)
	}

	// Schedule SyncTimeEntriesToDatabase to run based on configured schedule
	_, err = cronScheduler.AddFunc(timeEntriesSyncSchedule, func() {
		logger.Debug("Running scheduled time entries sync")
		if err := SyncTimeEntriesToDatabase(); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule time entries sync cron job: %v", err)
	}

	// Schedule daily Slack update to run based on configured schedule
	_, err = cronScheduler.AddFunc(dailyUpdateSchedule, func() {
		logger.Debug("Running scheduled daily Slack update")
		SendDailySlackUpdate()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule daily Slack update: %v", err)
	}

	// Start the cron scheduler
	cronScheduler.Start()
	defer cronScheduler.Stop()

	logger.Info("Cron scheduler started successfully")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Get Slack socket URL with proper error handling
	newSocketURL, err := getSlackSocketURL()
	if err != nil {
		logger.Fatalf("Critical error: Failed to get Slack socket URL: %v", err)
	}

	logger.Infof("Connecting to Slack WebSocket: %s", newSocketURL)

	conn, _, err := websocket.DefaultDialer.Dial(newSocketURL, nil)
	if err != nil {
		logger.Fatalf("Critical error: Failed to establish WebSocket connection: %v", err)
	}
	defer CloseWithErrorLog(conn, "WebSocket connection")

	logger.Info("WebSocket connection established successfully")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("WebSocket handler panicked: %v", r)
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				logger.Errorf("WebSocket read error: %v", err)
				return
			}
			logger.Debugf("Received WebSocket message: %s", string(message))

			var testPayload TestSlackPayload

			if err := json.Unmarshal(message, &testPayload); err != nil {
				logger.Errorf("Failed to unmarshal Slack payload: %v", err)
				continue // Don't crash, just skip this message
			}

			if len(testPayload.EnvelopeID) != 0 {
				response := TestSlackPayloadResponse{
					EnvelopeID: testPayload.EnvelopeID,
					Payload: Payload{
						[]Block{
							Block{
								Type: "section",
								Text: &Text{
									Type: "mrkdwn",
									Text: "**Test text**",
								},
							},
						},
					},
				}

				jsonResponse, err := json.Marshal(response)
				if err != nil {
					logger.Errorf("Failed to marshal response: %v", err)
					continue
				}

				err = conn.WriteMessage(websocket.TextMessage, []byte(jsonResponse))
				if err != nil {
					logger.Errorf("WebSocket write error: %v", err)
					return
				}

				logger.Debugf("Sent WebSocket response: %s", string(jsonResponse))
			}
		}
	}()

	logger.Info("Application is running. Press Ctrl+C to stop.")

	for {
		select {
		case <-interrupt:
			logger.Info("Received interrupt signal, shutting down gracefully...")

			// Stop the cron scheduler first
			logger.Info("Stopping cron scheduler...")
			cronScheduler.Stop()

			// Close the database connection
			logger.Info("Closing database connection...")
			if err := CloseDB(); err != nil {
				logger.Errorf("Error closing database: %v", err)
			} else {
				logger.Info("Database connection closed successfully")
			}

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				logger.Errorf("Error sending close message: %v", err)
			} else {
				logger.Info("Sent WebSocket close message")
			}
			return
		}
	}

}

func getSlackSocketURL() (string, error) {
	logger := NewLogger()

	// Get Slack API URL from environment variable or use default
	slackURL := os.Getenv("SLACK_API_URL")
	if slackURL == "" {
		slackURL = "https://slack.com/api/apps.connections.open"
	}

	// Validate Slack token exists
	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		return "", fmt.Errorf("SLACK_TOKEN environment variable not set")
	}

	request, err := http.NewRequest("POST", slackURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Add("Authorization", "Bearer "+slackToken)
	request.Header.Add("Content-type", "application/x-www-form-urlencoded")

	logger.Debugf("Requesting Slack socket URL from: %s", slackURL)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("HTTP request to Slack API failed: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			logger.Errorf("Error closing response body: %v", closeErr)
		}
	}()

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("Slack API returned status %d: %s", response.StatusCode, string(body))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var socketURLResponse SocketURLResponse

	if err := json.Unmarshal(body, &socketURLResponse); err != nil {
		return "", fmt.Errorf("failed to parse JSON response from Slack: %w", err)
	}

	if !socketURLResponse.Ok {
		return "", fmt.Errorf("Slack API returned error: response marked as not OK")
	}

	if socketURLResponse.Url == "" {
		return "", fmt.Errorf("Slack API returned empty socket URL")
	}

	logger.Debugf("Successfully obtained Slack socket URL")
	return socketURLResponse.Url, nil
}
