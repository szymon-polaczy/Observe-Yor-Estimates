package main

import (
	"context"
	"database/sql"
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

func StartServer(logger *Logger) {
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
func handleUnifiedOYECommand(responseWriter http.ResponseWriter, request *http.Request) {
	logger := GetGlobalLogger()

	if request.Method != http.MethodPost {
		http.Error(responseWriter, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSlackCommand(request)
	if err != nil {
		logger.Errorf("Failed to parse slash command: %v", err)
		http.Error(responseWriter, "Bad request", http.StatusBadRequest)
		return
	}

	if err := verifySlackRequest(req); err != nil {
		logger.Errorf("Failed to verify Slack request: %v", err)
		http.Error(responseWriter, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Infof("Received /oye command from user %s: %s", req.UserName, req.Text)

	commandText := strings.ToLower(strings.TrimSpace(req.Text))

	//string replace /oye with ""
	commandText = strings.Replace(commandText, "/oye", "", 1)

	//if the first word is not in the allowed commands, send help
	firstWord := strings.Fields(commandText)[0]
	allowedCommands := []string{"project", "for", "over"}
	if !slices.Contains(allowedCommands, firstWord) {
		sendUnifiedHelp(responseWriter)
		return
	}

	filteringByProject, projectName, err := confirmProject(commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(responseWriter, err.Error(), "ephemeral")
		return
	}

	filteringByPercentage, percentage, err := confirmPercentage(commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(responseWriter, err.Error(), "ephemeral")
		return
	}

	startTime, endTime, err := confirmPeriod(commandText)
	if err != nil {
		logger.Errorf(err.Error())
		sendImmediateResponse(responseWriter, err.Error(), "ephemeral")
		return
	}

	filteredTasks := getFilteredTasks(startTime, endTime, filteringByProject, []string{projectName}, filteringByPercentage, percentage)
	if len(filteredTasks) == 0 {
		sendImmediateResponse(responseWriter, "No tasks found", "ephemeral")
		return
	}

	filteredTasks = addCommentsToTasks(filteredTasks, startTime, endTime)
	filteredTasksGroupedByProject := groupTasksByProject(filteredTasks)

	sendTasksGroupedByProject(req, filteredTasksGroupedByProject)
}

/* Gets the project name from the command text
 * If the project name is not found, returns an error
 * If the project name is found, but is not in the database, returns an error
 * If the project name is found, and is in the database, returns the project name and nil
 */
func confirmProject(commandText string) (bool, string, error) {
	projectName := ""
	projectNameRegex := regexp.MustCompile(`project (.*?) (for|over)`)

	matches := projectNameRegex.FindStringSubmatch(commandText)
	if len(matches) >= 1 {
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
func confirmPercentage(commandText string) (bool, string, error) {
	percentage := ""
	percentageRegex := regexp.MustCompile(`over (.*?) for`)

	matches := percentageRegex.FindStringSubmatch(commandText)
	if len(matches) >= 1 {
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
func confirmPeriod(commandText string) (time.Time, time.Time, error) {
	logger := GetGlobalLogger()
	period := ""
	periodRegex := regexp.MustCompile(`for (.*)`)

	matches := periodRegex.FindStringSubmatch(commandText)
	if len(matches) >= 1 {
		period = strings.TrimSpace(matches[1])
	}

	if period == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("Failed to parse period from command")
	}

	logger.Infof("Parsing period: '%s'", period)
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
				logger.Infof("Period '%s' resolved to: %s to %s", period, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))
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

	logger.Infof("Period '%s' resolved to: %s to %s", period, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))
	return startTime, endTime, nil
}

// fully AI generated
func getFilteredTasks(startTime time.Time, endTime time.Time, filteringByProject bool, projectNames []string, filteringByPercentage bool, percentage string) []TaskInfo {
	logger := GetGlobalLogger()
	logger.Infof("getFilteredTasks called with: startTime=%s, endTime=%s, filteringByProject=%t, projectNames='%s', filteringByPercentage=%t, percentage='%s'",
		startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"), filteringByProject, projectNames, filteringByPercentage, percentage)

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection: %v", err)
		return []TaskInfo{}
	}

	// Convert time range to date strings for SQL query (time_entries uses TEXT date field)
	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")
	logger.Infof("Searching for tasks between dates: %s and %s", startDateStr, endDateStr)

	// Simplified query that should work with the actual database structure
	var query string
	var args []interface{}

	if filteringByProject && len(projectNames) > 0 {
		for i, projectName := range projectNames {
			projectNames[i] = strings.ToLower(projectName)
		}

		// When filtering by project, join with projects table
		query = `
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE 
				WHEN te.date >= $1 AND te.date <= $2
				THEN te.duration 
				ELSE 0 
			END), 0) as current_period_duration,
			COALESCE(SUM(te.duration), 0) as total_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		LEFT JOIN projects p ON t.project_id = p.id
		WHERE LOWER(p.name) IN ($3)
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(CASE 
			WHEN te.date >= $4 AND te.date <= $5
			THEN te.duration 
			ELSE 0 
		END), 0) > 0
		ORDER BY t.name;`
		args = []interface{}{startDateStr, endDateStr, strings.Join(projectNames, ","), startDateStr, endDateStr}
	} else {
		// When not filtering by project, get all tasks with time entries in the period
		query = `
		SELECT 
			t.task_id,
			t.parent_id,
			t.name,
			COALESCE(SUM(CASE 
				WHEN te.date >= $1 AND te.date <= $2
				THEN te.duration 
				ELSE 0 
			END), 0) as current_period_duration,
			COALESCE(SUM(te.duration), 0) as total_duration
		FROM tasks t
		LEFT JOIN time_entries te ON t.task_id = te.task_id
		GROUP BY t.task_id, t.parent_id, t.name
		HAVING COALESCE(SUM(CASE 
			WHEN te.date >= $3 AND te.date <= $4
			THEN te.duration 
			ELSE 0 
		END), 0) > 0
		ORDER BY t.name;`
		args = []interface{}{startDateStr, endDateStr, startDateStr, endDateStr}
	}

	logger.Infof("Query: %s", query)
	logger.Infof("Args: %v", args)

	logger.Infof("Executing query with args: %v", args)
	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Errorf("Database query failed: %v", err)
		logger.Errorf("Query was: %s", query)
		return []TaskInfo{}
	}
	defer rows.Close()

	var allTasks []TaskInfo
	var percentageThreshold float64
	taskCount := 0

	// Parse percentage threshold if filtering by percentage
	if filteringByPercentage {
		if percentageFloat, err := strconv.ParseFloat(strings.TrimSuffix(percentage, "%"), 64); err == nil {
			percentageThreshold = percentageFloat
			logger.Infof("Filtering by percentage threshold: %.1f%%", percentageThreshold)
		} else {
			logger.Errorf("Invalid percentage format: %s", percentage)
			return []TaskInfo{} // Invalid percentage format
		}
	}

	for rows.Next() {
		taskCount++
		var task TaskInfo
		var currentDuration, totalDuration int

		err := rows.Scan(
			&task.TaskID,
			&task.ParentID,
			&task.Name,
			&currentDuration,
			&totalDuration,
		)
		if err != nil {
			logger.Errorf("Failed to scan task row %d: %v", taskCount, err)
			continue
		}

		logger.Infof("Found task %d: ID=%d, Name='%s', CurrentDuration=%d seconds", taskCount, task.TaskID, task.Name, currentDuration)

		// Format durations using existing formatDuration function (takes seconds)
		task.CurrentTime = formatDuration(currentDuration)
		task.TotalDuration = formatDuration(totalDuration)

		// Parse estimation from task name and calculate usage
		estimationInfo := ParseTaskEstimationWithUsage(task.Name, task.CurrentTime, "0h 0m")
		task.EstimationInfo = estimationInfo

		// If filtering by percentage, apply the logic
		if filteringByPercentage {
			// Skip tasks without valid estimations
			if estimationInfo.ErrorMessage != "" {
				logger.Infof("Skipping task %d (no valid estimation): %s", task.TaskID, estimationInfo.ErrorMessage)
				continue
			}

			// Skip tasks below threshold
			if estimationInfo.Percentage < percentageThreshold {
				logger.Infof("Skipping task %d (below threshold): %.1f%% < %.1f%%", task.TaskID, estimationInfo.Percentage, percentageThreshold)
				continue
			}
		}

		allTasks = append(allTasks, task)
	}

	logger.Infof("Query returned %d total tasks, filtered to %d tasks", taskCount, len(allTasks))
	return allTasks
}

// fully AI generated
func addCommentsToTasks(tasks []TaskInfo, startTime time.Time, endTime time.Time) []TaskInfo {
	logger := GetGlobalLogger()
	if len(tasks) == 0 {
		logger.Info("No tasks to add comments to")
		return tasks
	}

	logger.Infof("Adding comments to %d tasks", len(tasks))

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection for comments: %v", err)
		return tasks
	}

	// Collect all task IDs
	taskIDs := make([]string, 0, len(tasks))
	taskMap := make(map[int]*TaskInfo)

	for i := range tasks {
		taskIDs = append(taskIDs, strconv.Itoa(tasks[i].TaskID))
		taskMap[tasks[i].TaskID] = &tasks[i]
	}

	logger.Infof("Querying comments for task IDs: %v", taskIDs)

	// Query for comments from time_entries descriptions using PostgreSQL placeholders
	placeholders := make([]string, len(taskIDs))
	for i := range taskIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	placeholderStr := strings.Join(placeholders, ",")

	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")

	query := fmt.Sprintf(`
		SELECT task_id, description, date
		FROM time_entries 
		WHERE task_id IN (%s) 
		AND date >= '%s' AND date <= '%s'
		AND description IS NOT NULL 
		AND description != ''
		ORDER BY task_id, date DESC`, placeholderStr)

	// Convert taskIDs to []interface{} for query args
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}

	args = append(args, startDateStr, endDateStr)

	logger.Infof("Executing comment query: %s", query)
	logger.Infof("Query args: %v", args)

	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Errorf("Failed to query comments: %v", err)
		return tasks
	}
	defer rows.Close()

	// Map to collect comments by task_id
	taskComments := make(map[int][]string)
	commentCount := 0

	for rows.Next() {
		var taskID int
		var description sql.NullString

		if err := rows.Scan(&taskID, &description); err != nil {
			logger.Errorf("Failed to scan comment row: %v", err)
			continue
		}

		if description.Valid && description.String != "" {
			logger.Infof("Found comment for task %d: %s", taskID, description.String)
			// Split descriptions by "; " as done in existing code
			comments := strings.Split(description.String, "; ")
			taskComments[taskID] = append(taskComments[taskID], comments...)
			commentCount++
		}
	}

	logger.Infof("Found %d comment entries for %d tasks", commentCount, len(taskComments))

	// Add comments to tasks and remove duplicates
	for taskID, comments := range taskComments {
		if task, exists := taskMap[taskID]; exists {
			// Remove duplicates and empty strings
			uniqueComments := make([]string, 0)
			seen := make(map[string]bool)

			for _, comment := range comments {
				comment = strings.TrimSpace(comment)
				if comment != "" && !seen[comment] {
					uniqueComments = append(uniqueComments, comment)
					seen[comment] = true
				}
			}

			logger.Infof("Adding %d unique comments to task %d (%s)", len(uniqueComments), taskID, task.Name)
			task.Comments = uniqueComments
		}
	}

	logger.Infof("Completed adding comments to tasks")
	return tasks
}

// fully AI generated
func sendTasksGroupedByProject(req *SlackCommandRequest, projectGroups map[string][]TaskInfo) {
	logger := GetGlobalLogger()
	logger.Infof("Starting sendTasksGroupedByProject with %d tasks", len(projectGroups))

	if len(projectGroups) == 0 {
		logger.Info("No tasks to send, returning early")
		return
	}

	// Check for required Slack configuration
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackBotToken == "" {
		logger.Error("SLACK_BOT_TOKEN not configured in environment")
		return
	}

	if req.ResponseURL == "" {
		logger.Error("No response URL provided, cannot send initial message")
		return
	}

	logger.Infof("Using Slack Bot API for threaded messages in channel: %s", req.ChannelID)

	// Get threshold values from environment variables
	midPoint := DEFAULT_MID_POINT
	if envMidPoint := os.Getenv("MID_POINT"); envMidPoint != "" {
		if parsed, err := strconv.ParseFloat(envMidPoint, 64); err == nil {
			midPoint = parsed
		}
	}

	highPoint := DEFAULT_HIGH_POINT
	if envHighPoint := os.Getenv("HIGH_POINT"); envHighPoint != "" {
		if parsed, err := strconv.ParseFloat(envHighPoint, 64); err == nil {
			highPoint = parsed
		}
	}

	logger.Infof("Using thresholds - MID_POINT: %.1f, HIGH_POINT: %.1f", midPoint, highPoint)

	// Send initial thread message using response URL
	initialMessage := map[string]interface{}{
		"response_type": "in_channel",
		"text":          "📊 Update in thread",
	}

	initialPayloadBytes, err := json.Marshal(initialMessage)
	if err != nil {
		logger.Errorf("Failed to marshal initial message payload: %v", err)
		return
	}

	logger.Info("Sending initial thread message")
	initialResp, err := http.Post(req.ResponseURL, "application/json", strings.NewReader(string(initialPayloadBytes)))
	if err != nil {
		logger.Errorf("Failed to send initial thread message: %v", err)
		return
	}
	initialResp.Body.Close()

	if initialResp.StatusCode != http.StatusOK {
		logger.Errorf("Initial thread message response status: %d", initialResp.StatusCode)
		return
	}

	logger.Info("Initial thread message sent successfully")

	// Wait a moment for the message to be processed
	time.Sleep(500 * time.Millisecond)

	// Get the timestamp of the initial message by calling Slack API
	// We'll use the conversations.history API to get the latest message timestamp
	historyURL := "https://slack.com/api/conversations.history"
	historyParams := fmt.Sprintf("channel=%s&limit=1", req.ChannelID)
	historyReq, _ := http.NewRequest("GET", historyURL+"?"+historyParams, nil)
	historyReq.Header.Set("Authorization", "Bearer "+slackBotToken)
	historyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	historyResp, err := client.Do(historyReq)
	if err != nil {
		logger.Errorf("Failed to get conversation history: %v", err)
		return
	}
	defer historyResp.Body.Close()

	var historyData map[string]interface{}
	if err := json.NewDecoder(historyResp.Body).Decode(&historyData); err != nil {
		logger.Errorf("Failed to decode history response: %v", err)
		return
	}

	// Extract timestamp from the latest message
	var threadTimestamp string
	if messages, ok := historyData["messages"].([]interface{}); ok && len(messages) > 0 {
		if latestMsg, ok := messages[0].(map[string]interface{}); ok {
			if ts, ok := latestMsg["ts"].(string); ok {
				threadTimestamp = ts
				logger.Infof("Got thread timestamp: %s", threadTimestamp)
			}
		}
	}

	if threadTimestamp == "" {
		logger.Error("Failed to get thread timestamp, sending messages without threading")
		// Could fallback to non-threaded messages here if needed
		return
	}

	// Process each project
	for projectName, projectTasks := range projectGroups {
		logger.Infof("Processing project '%s' with %d tasks", projectName, len(projectTasks))

		// Send project header message as threaded reply
		projectHeaderPayload := map[string]interface{}{
			"channel":   req.ChannelID,
			"text":      fmt.Sprintf("%s **%s**", EMOJI_FOLDER, projectName),
			"thread_ts": threadTimestamp,
		}

		headerPayloadBytes, err := json.Marshal(projectHeaderPayload)
		if err != nil {
			logger.Errorf("Failed to marshal project header payload: %v", err)
			continue
		}

		logger.Infof("Sending project header for '%s'", projectName)
		headerReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(headerPayloadBytes)))
		headerReq.Header.Set("Authorization", "Bearer "+slackBotToken)
		headerReq.Header.Set("Content-Type", "application/json")

		headerResp, err := client.Do(headerReq)
		if err != nil {
			logger.Errorf("Failed to send project header for '%s': %v", projectName, err)
			continue
		}
		headerResp.Body.Close()

		if headerResp.StatusCode != http.StatusOK {
			logger.Errorf("Project header response status %d for '%s'", headerResp.StatusCode, projectName)
		} else {
			logger.Infof("Successfully sent project header for '%s'", projectName)
		}

		// Wait 200ms before next message
		time.Sleep(200 * time.Millisecond)

		// Create task blocks
		var blocks []map[string]interface{}
		currentBlockCount := 0
		currentCharCount := 0

		for _, task := range projectTasks {
			logger.Infof("Processing task %d (%s) with %d comments", task.TaskID, task.Name, len(task.Comments))

			// Build task text - EstimationInfo.Text already contains percentage and emoji
			taskText := fmt.Sprintf("*%s*", task.Name)

			// Add time spent on the task
			taskText += fmt.Sprintf("\nTime spent: %s | Total time: %s", task.CurrentTime, task.TotalDuration)

			// Add estimation info if available
			if task.EstimationInfo.Text != "" {
				taskText += fmt.Sprintf(" | %s", task.EstimationInfo.Text)
			}

			// Add comments as unordered list
			if len(task.Comments) > 0 {
				logger.Infof("Adding %d comments to task %d", len(task.Comments), task.TaskID)
				taskText += "\n"
				for i, comment := range task.Comments {
					if comment != "" {
						// Limit comment length to avoid overwhelming
						if len(comment) > 100 {
							comment = comment[:97] + "..."
						}
						taskText += fmt.Sprintf("• %s\n", comment)
						logger.Infof("Comment %d for task %d: %s", i+1, task.TaskID, comment)
					}
				}
			} else {
				logger.Infof("No comments found for task %d", task.TaskID)
			}

			// Create task block
			taskBlock := map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": taskText,
				},
			}

			// Estimate character count for this block
			blockBytes, _ := json.Marshal(taskBlock)
			blockCharCount := len(blockBytes)

			// Check if adding this block would exceed limits
			if currentBlockCount+1 > MAX_SLACK_BLOCKS || currentCharCount+blockCharCount > MAX_MESSAGE_CHARS_BUFFER {
				// Send current blocks if we have any
				if len(blocks) > 0 {
					logger.Infof("Sending message chunk for project '%s' with %d blocks (%d chars)", projectName, len(blocks), currentCharCount)

					messagePayload := map[string]interface{}{
						"channel":   req.ChannelID,
						"blocks":    blocks,
						"thread_ts": threadTimestamp,
					}

					payloadBytes, err := json.Marshal(messagePayload)
					if err != nil {
						logger.Errorf("Failed to marshal message payload: %v", err)
						break
					}

					msgReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(payloadBytes)))
					msgReq.Header.Set("Authorization", "Bearer "+slackBotToken)
					msgReq.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(msgReq)
					if err != nil {
						logger.Errorf("Failed to send message chunk for project '%s': %v", projectName, err)
						break
					}
					resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						logger.Errorf("Message chunk response status %d for project '%s'", resp.StatusCode, projectName)
					} else {
						logger.Infof("Successfully sent message chunk for project '%s'", projectName)
					}

					// Wait 200ms before next message
					time.Sleep(200 * time.Millisecond)

					// Reset for next chunk
					blocks = []map[string]interface{}{}
					currentBlockCount = 0
					currentCharCount = 0
				}
			}

			// Add block to current batch
			blocks = append(blocks, taskBlock)
			currentBlockCount++
			currentCharCount += blockCharCount
		}

		// Send remaining blocks if any
		if len(blocks) > 0 {
			logger.Infof("Sending final message chunk for project '%s' with %d blocks (%d chars)", projectName, len(blocks), currentCharCount)

			messagePayload := map[string]interface{}{
				"channel":   req.ChannelID,
				"blocks":    blocks,
				"thread_ts": threadTimestamp,
			}

			payloadBytes, err := json.Marshal(messagePayload)
			if err != nil {
				logger.Errorf("Failed to marshal final message payload: %v", err)
				continue
			}

			finalReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(payloadBytes)))
			finalReq.Header.Set("Authorization", "Bearer "+slackBotToken)
			finalReq.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(finalReq)
			if err != nil {
				logger.Errorf("Failed to send final message chunk for project '%s': %v", projectName, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				logger.Errorf("Final message chunk response status %d for project '%s'", resp.StatusCode, projectName)
			} else {
				logger.Infof("Successfully sent final message chunk for project '%s'", projectName)
			}

			// Wait 200ms before next project
			time.Sleep(200 * time.Millisecond)
		}

		logger.Infof("Completed processing project '%s'", projectName)
	}

	logger.Info("Completed sendTasksGroupedByProject")
}

// sendTasksGroupedByProjectToUser sends personalized task updates to a specific user via direct message
func sendTasksGroupedByProjectToUser(userID string, projectGroups map[string][]TaskInfo) {
	logger := GetGlobalLogger()
	logger.Infof("Starting sendTasksGroupedByProjectToUser for user %s with %d project groups", userID, len(projectGroups))

	if len(projectGroups) == 0 {
		logger.Infof("No tasks to send to user %s, returning early", userID)
		return
	}

	// Check for required Slack configuration
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackBotToken == "" {
		logger.Error("SLACK_BOT_TOKEN not configured in environment")
		return
	}

	logger.Infof("Sending direct message to user %s", userID)

	// Get threshold values from environment variables
	midPoint := DEFAULT_MID_POINT
	if envMidPoint := os.Getenv("MID_POINT"); envMidPoint != "" {
		if parsed, err := strconv.ParseFloat(envMidPoint, 64); err == nil {
			midPoint = parsed
		}
	}

	highPoint := DEFAULT_HIGH_POINT
	if envHighPoint := os.Getenv("HIGH_POINT"); envHighPoint != "" {
		if parsed, err := strconv.ParseFloat(envHighPoint, 64); err == nil {
			highPoint = parsed
		}
	}

	logger.Infof("Using thresholds - MID_POINT: %.1f, HIGH_POINT: %.1f", midPoint, highPoint)

	client := &http.Client{}

	// Process each project
	for projectName, projectTasks := range projectGroups {
		logger.Infof("Processing project '%s' with %d tasks for user %s", projectName, len(projectTasks), userID)

		// Send project header message as direct message
		projectHeaderPayload := map[string]interface{}{
			"channel": userID, // For DMs, channel is the user ID
			"text":    fmt.Sprintf("%s **%s**", EMOJI_FOLDER, projectName),
		}

		headerPayloadBytes, err := json.Marshal(projectHeaderPayload)
		if err != nil {
			logger.Errorf("Failed to marshal project header payload: %v", err)
			continue
		}

		logger.Infof("Sending project header for '%s' to user %s", projectName, userID)
		headerReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(headerPayloadBytes)))
		headerReq.Header.Set("Authorization", "Bearer "+slackBotToken)
		headerReq.Header.Set("Content-Type", "application/json")

		headerResp, err := client.Do(headerReq)
		if err != nil {
			logger.Errorf("Failed to send project header for '%s' to user %s: %v", projectName, userID, err)
			continue
		}
		headerResp.Body.Close()

		if headerResp.StatusCode != http.StatusOK {
			logger.Errorf("Project header response status %d for '%s' to user %s", headerResp.StatusCode, projectName, userID)
		} else {
			logger.Infof("Successfully sent project header for '%s' to user %s", projectName, userID)
		}

		// Wait 200ms before next message
		time.Sleep(200 * time.Millisecond)

		// Create task blocks
		var blocks []map[string]interface{}
		currentBlockCount := 0
		currentCharCount := 0

		for _, task := range projectTasks {
			logger.Infof("Processing task %d (%s) with %d comments for user %s", task.TaskID, task.Name, len(task.Comments), userID)

			// Build task text - EstimationInfo.Text already contains percentage and emoji
			taskText := fmt.Sprintf("*%s*", task.Name)

			// Add time spent on the task
			taskText += fmt.Sprintf("\nTime spent: %s | Total time: %s", task.CurrentTime, task.TotalDuration)

			// Add estimation info if available
			if task.EstimationInfo.Text != "" {
				taskText += fmt.Sprintf(" | %s", task.EstimationInfo.Text)
			}

			// Add comments as unordered list
			if len(task.Comments) > 0 {
				logger.Infof("Adding %d comments to task %d for user %s", len(task.Comments), task.TaskID, userID)
				taskText += "\n"
				for i, comment := range task.Comments {
					if comment != "" {
						// Limit comment length to avoid overwhelming
						if len(comment) > 100 {
							comment = comment[:97] + "..."
						}
						taskText += fmt.Sprintf("• %s\n", comment)
						logger.Infof("Comment %d for task %d: %s", i+1, task.TaskID, comment)
					}
				}
			} else {
				logger.Infof("No comments found for task %d", task.TaskID)
			}

			// Create task block
			taskBlock := map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": taskText,
				},
			}

			// Estimate character count for this block
			blockBytes, _ := json.Marshal(taskBlock)
			blockCharCount := len(blockBytes)

			// Check if adding this block would exceed limits
			if currentBlockCount+1 > MAX_SLACK_BLOCKS || currentCharCount+blockCharCount > MAX_MESSAGE_CHARS_BUFFER {
				// Send current blocks if we have any
				if len(blocks) > 0 {
					logger.Infof("Sending message chunk for project '%s' to user %s with %d blocks (%d chars)", projectName, userID, len(blocks), currentCharCount)

					messagePayload := map[string]interface{}{
						"channel": userID,
						"blocks":  blocks,
					}

					payloadBytes, err := json.Marshal(messagePayload)
					if err != nil {
						logger.Errorf("Failed to marshal message payload: %v", err)
						break
					}

					msgReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(payloadBytes)))
					msgReq.Header.Set("Authorization", "Bearer "+slackBotToken)
					msgReq.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(msgReq)
					if err != nil {
						logger.Errorf("Failed to send message chunk for project '%s' to user %s: %v", projectName, userID, err)
						break
					}
					resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						logger.Errorf("Message chunk response status %d for project '%s' to user %s", resp.StatusCode, projectName, userID)
					} else {
						logger.Infof("Successfully sent message chunk for project '%s' to user %s", projectName, userID)
					}

					// Wait 200ms before next message
					time.Sleep(200 * time.Millisecond)

					// Reset for next chunk
					blocks = []map[string]interface{}{}
					currentBlockCount = 0
					currentCharCount = 0
				}
			}

			// Add block to current batch
			blocks = append(blocks, taskBlock)
			currentBlockCount++
			currentCharCount += blockCharCount
		}

		// Send remaining blocks if any
		if len(blocks) > 0 {
			logger.Infof("Sending final message chunk for project '%s' to user %s with %d blocks (%d chars)", projectName, userID, len(blocks), currentCharCount)

			messagePayload := map[string]interface{}{
				"channel": userID,
				"blocks":  blocks,
			}

			payloadBytes, err := json.Marshal(messagePayload)
			if err != nil {
				logger.Errorf("Failed to marshal final message payload: %v", err)
				continue
			}

			finalReq, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", strings.NewReader(string(payloadBytes)))
			finalReq.Header.Set("Authorization", "Bearer "+slackBotToken)
			finalReq.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(finalReq)
			if err != nil {
				logger.Errorf("Failed to send final message chunk for project '%s' to user %s: %v", projectName, userID, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				logger.Errorf("Final message chunk response status %d for project '%s' to user %s", resp.StatusCode, projectName, userID)
			} else {
				logger.Infof("Successfully sent final message chunk for project '%s' to user %s", projectName, userID)
			}

			// Wait 200ms before next project
			time.Sleep(200 * time.Millisecond)
		}

		logger.Infof("Completed processing project '%s' for user %s", projectName, userID)
	}

	logger.Infof("Completed sendTasksGroupedByProjectToUser for user %s", userID)
}

/* Displays help text for the OYE command */
func sendUnifiedHelp(responseWriter http.ResponseWriter) {
	helpText := "*🎯 OYE (Observe-Yor-Estimates) Commands*\n\n" +
		"*Time Frame Options:*\n" +
		"• `/oye for [period]` - Update for specific time frame\n" +
		"• `/oye project [project name] for [period]` - Update for specific project and time frame\n" +
		"• `/oye over [percentage] for [period]` - Check for tasks over threshold\n" +
		"• `/oye project [project name] over [percentage] for [period]` - Check for tasks over threshold for a specific project\n" +

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

	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(response)
}

/* Parses the form data from a Slack slash command */
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

/* Verifies that the request is from Slack */
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

/* Sends an immediate response to Slack */
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
