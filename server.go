package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Global smart router instance
var globalRouter *SmartRouter

func StartServer(logger *Logger) {
	// Initialize the smart router
	globalRouter = NewSmartRouter()

	// Unified handler for all OYE commands
	http.HandleFunc("/slack/oye", handleUnifiedOYECommand)

	// New App Home routes
	http.HandleFunc("/slack/events", HandleAppHome)
	http.HandleFunc("/slack/interactive", HandleInteractiveComponents)

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

	commandText := strings.ToLower(strings.TrimSpace(req.Text))

	//string replace /oye with ""
	commandText = strings.Replace(commandText, "/oye", "", 1)

	//if the first word is not in the allowed commands, send help
	firstWord := strings.Fields(commandText)[0]
	allowedCommands := []string{"project", "update", "over"}
	if !slices.Contains(allowedCommands, firstWord) {
		sendUnifiedHelp(w, req)
		return
	}

	filteringByProject, projectName, err := confirmProject(w, req, commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(w, err.Error(), "ephemeral")
		return
	}

	filteringByPercentage, percentage, err := confirmPercentage(w, req, commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(w, err.Error(), "ephemeral")
		return
	}

	startTime, endTime, err := confirmPeriod(w, req, commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(w, err.Error(), "ephemeral")
		return
	}

	tasksGroupedByProject := filteredTasksGroupedByProject(startTime, endTime, filteringByProject, projectName, filteringByPercentage, percentage)
}

/* Gets the project name from the command text
 * If the project name is not found, returns an error
 * If the project name is found, but is not in the database, returns an error
 * If the project name is found, and is in the database, returns the project name and nil
 */
func confirmProject(w http.ResponseWriter, req *SlackCommandRequest, commandText string) (bool, string, error) {
	projectName := ""
	projectNameRegex := regexp.MustCompile(`project (.*?) update`)

	matches := projectNameRegex.FindStringSubmatch(commandText)
	if len(matches) > 1 {
		projectName = strings.TrimSpace(matches[1])
	} else {
		return false, "", nil
	}

	if projectName == "" {
		return true, "", fmt.Errorf("Failed to parse project name from command")
	}

	db, err := GetDB()
	if err != nil {
		return true, "", fmt.Errorf("Failed to get database: %v", err)
	}

	_, err = FindProjectsByName(db, projectName)
	if err != nil {
		return true, "", err
	}

	return true, projectName, nil
}

/* Gets the percentage from the command text
 * If the percentage is not found, returns an error
 * If the percentage is found, returns the percentage and nil
 */
func confirmPercentage(w http.ResponseWriter, req *SlackCommandRequest, commandText string) (bool, string, error) {
	percentage := ""
	percentageRegex := regexp.MustCompile(`over (.*?)`)

	matches := percentageRegex.FindStringSubmatch(commandText)
	if len(matches) > 1 {
		percentage = strings.TrimSpace(matches[1])
	} else {
		return false, "", nil
	}

	if percentage == "" {
		return true, "", fmt.Errorf("Failed to parse percentage from command")
	}

	return true, percentage, nil
}

/* Gets the period from the command text
 * If the period is not found, returns an error
 * If the period is found, checks if it is a valid period
 * If the period is found, and is valid, returns the period's start and end times and nil
 * If the period is found, and is invalid, returns an error
 */
func confirmPeriod(w http.ResponseWriter, req *SlackCommandRequest, commandText string) (time.Time, time.Time, error) {
	period := ""
	periodRegex := regexp.MustCompile(`update (.*?)`)

	matches := periodRegex.FindStringSubmatch(commandText)
	if len(matches) > 1 {
		period = strings.TrimSpace(matches[1])
	}

	if period == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("Failed to parse period from command")
	}

	now := time.Now()
	var startTime, endTime time.Time

	// Check for "last x days" pattern
	lastDaysRegex := regexp.MustCompile(`^last (\d+) days?$`)
	if lastDaysRegex.MatchString(period) {
		matches := lastDaysRegex.FindStringSubmatch(period)
		if len(matches) > 1 {
			days := matches[1]
			// Convert to int to validate it's a positive number
			if daysInt, err := strconv.Atoi(days); err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("Invalid number in 'last %s days': not a valid number", days)
			} else if daysInt <= 0 {
				return time.Time{}, time.Time{}, fmt.Errorf("Invalid number in 'last %s days': must be a positive number", days)
			} else {
				// Start: x days ago at 0:00, End: today at 23:59
				startTime = now.AddDate(0, 0, -daysInt).Truncate(24 * time.Hour)
				endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
				return startTime, endTime, nil
			}
		}
	}

	switch period {
	case "today":
		// Today 0:00 to today 23:59
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	case "yesterday":
		// Yesterday 0:00 to yesterday 23:59
		yesterday := now.AddDate(0, 0, -1)
		startTime = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
		endTime = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 999999999, yesterday.Location())
	case "this week":
		// Monday 0:00 to Sunday 23:59 of current week
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		sunday := monday.AddDate(0, 0, 6)
		startTime = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
		endTime = time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 23, 59, 59, 999999999, sunday.Location())
	case "last week":
		// Monday 0:00 to Sunday 23:59 of last week
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		lastMonday := now.AddDate(0, 0, -(weekday-1)-7)
		lastSunday := lastMonday.AddDate(0, 0, 6)
		startTime = time.Date(lastMonday.Year(), lastMonday.Month(), lastMonday.Day(), 0, 0, 0, 0, lastMonday.Location())
		endTime = time.Date(lastSunday.Year(), lastSunday.Month(), lastSunday.Day(), 23, 59, 59, 999999999, lastSunday.Location())
	case "this month":
		// First day of month 0:00 to last day of month 23:59
		firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		lastDay := firstDay.AddDate(0, 1, -1)
		startTime = firstDay
		endTime = time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 999999999, lastDay.Location())
	case "last month":
		// First day of last month 0:00 to last day of last month 23:59
		firstDayLastMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		lastDayLastMonth := firstDayLastMonth.AddDate(0, 1, -1)
		startTime = firstDayLastMonth
		endTime = time.Date(lastDayLastMonth.Year(), lastDayLastMonth.Month(), lastDayLastMonth.Day(), 23, 59, 59, 999999999, lastDayLastMonth.Location())
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid period: %s", period)
	}

	return startTime, endTime, nil
}

func filteredTasksGroupedByProject(startTime time.Time, endTime time.Time, filteringByProject bool, projectName string, filteringByPercentage bool, percentage string) []TaskInfo {
	db, err := GetDB()
	if err != nil {
		return []TaskInfo{}
	}
}

func sendUnifiedHelp(w http.ResponseWriter, req *SlackCommandRequest) {
	helpText := "*🎯 OYE (Observe-Yor-Estimates) Commands*\n\n" +
		"*Time Frame Options:*\n" +
		"• `/oye update [period]` - Update for specific time frame\n" +
		"• `/oye project [project name] update [period]` - Update for specific project and time frame\n" +
		"• `/oye over [percentage] [period]` - Check for tasks over threshold\n" +
		"• `/oye project [project name] over [percentage] update [period]` - Check for tasks over threshold for a specific project\n" +

		"*Available Periods:*\n" +
		"• today\n" +
		"• yesterday\n" +
		"• last week\n" +
		"• this week\n" +
		"• last month\n" +
		"• this month\n" +
		"• last x days\n" +

		"*Tips:*\n" +
		"• Updates are private by default (only you see them)\n" +
		"• Project names with spaces are fine without quotes\n" +
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
