package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	logger := NewLogger()

	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Warning: Failed to load .env file: %v", err)
	}

	if err := validateRequiredEnvVars(); err != nil {
		logger.Fatalf("Critical error: Missing required environment variables: %v", err)
	}

	if len(os.Args) > 1 {
		handleCliCommands(os.Args[1:], logger)
		return
	}

	logger.Info("Starting Observe-Yor-Estimates application")

	// Initialize database connection for CLI operations
	logger.Info("Initializing database connection...")
	_, initErr := GetDB()
	if initErr != nil {
		logger.Errorf("Failed to initialize database: %v", initErr)
		return
	}
	logger.Info("Database connection initialized successfully")

	// Start as long-running server with background services
	logger.Info("Starting server mode with background services...")

	// Setup and start cron jobs for periodic syncing
	logger.Info("Setting up scheduled tasks...")
	setupCronJobs(logger)

	// Start HTTP server for Slack commands (this will block)
	logger.Info("Starting HTTP server for Slack integrations...")
	StartServer(logger)
}

func handleCliCommands(args []string, logger *Logger) {
	if len(args) == 0 {
		return
	}
	command := args[0]

	// Support --test-command=<command> format
	if strings.HasPrefix(command, "--test-command=") {
		testCmd := strings.TrimPrefix(command, "--test-command=")
		if err := RunTestCommand(logger, testCmd, "Szymon"); err != nil {
			logger.Errorf("Test command failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Test command completed successfully")
		return
	}

	switch command {
	case "--help", "-h", "help":
		showHelp()
	case "--test-command":
		if len(args) < 2 {
			logger.Errorf("missing command string for --test-command")
			os.Exit(1)
		}
		if err := RunTestCommand(logger, args[1], "Szymon"); err != nil {
			logger.Errorf("Test command failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Test command completed successfully")
	case "full-sync":
		logger.Info("Running full synchronization command")
		if err := FullSyncAll(); err != nil {
			logger.Errorf("Full sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full synchronization completed successfully")
	default:
		logger.Warnf("Unknown command line argument: %s", command)
		showHelp()
	}
}

func setupCronJobs(logger *Logger) {
	cronScheduler := cron.New()

	addCronJob(cronScheduler, "TASK_SYNC_SCHEDULE", "0 */6 * * *", "task sync", logger, func() {
		if err := SyncTasksToDatabase(false); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "TIME_ENTRIES_SYNC_SCHEDULE", "*/30 * * * *", "time entries sync", logger, func() {
		if err := SyncTimeEntriesToDatabaseWithOptions("", "", false); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "SLACK_USER_SYNC_SCHEDULE", "0 5 * * *", "Slack user sync", logger, func() {
		if err := SyncSlackUsersToDatabase(); err != nil {
			logger.Errorf("Scheduled Slack user sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "DAILY_UPDATE_SCHEDULE", "0 6 * * *", "daily Slack update", logger, func() {
		sendDailyUpdate(logger)
	})

	// Add orphaned time entries processing cron job (every 6 hours)
	addCronJob(cronScheduler, "ORPHANED_PROCESSING_SCHEDULE", "0 */6 * * *", "orphaned time entries processing", logger, func() {
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection for orphaned processing: %v", err)
			return
		}

		// Check if there are any orphaned entries to process
		count, err := GetOrphanedTimeEntriesCount(db)
		if err != nil {
			logger.Errorf("Failed to count orphaned entries: %v", err)
			return
		}

		if count > 0 {
			logger.Infof("Found %d orphaned time entries, processing...", count)
			if err := ProcessOrphanedTimeEntries(db); err != nil {
				logger.Errorf("Orphaned time entries processing failed: %v", err)
			} else {
				logger.Debug("Orphaned time entries processing completed successfully")
			}
		}
	})

	cronScheduler.Start()
	logger.Info("Cron scheduler started successfully")
}

func sendDailyUpdate(logger *Logger) {
	commandText := "for yesterday"

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection for daily update: %v", err)
		return
	}

	// Get all Slack users from database (synced earlier by user sync cron job)
	users, err := GetSlackUsersFromDatabase()
	if err != nil {
		logger.Errorf("Failed to get Slack users for daily update: %v", err)
		return
	}

	logger.Infof("Starting optimized daily updates for %d users", len(users))

	// Get time period for filtering tasks
	startTime, endTime, err := confirmPeriod(commandText)
	if err != nil {
		logger.Errorf("Failed to parse period for daily update: %v", err)
		return
	}

	// OPTIMIZATION: Query ALL data once instead of per-user queries
	logger.Info("Fetching ALL yesterday's data in single queries...")
	
	// 1. Get ALL user-project assignments at once
	userProjectMap, err := getAllUserProjectAssignments(db)
	if err != nil {
		logger.Errorf("Failed to get user project assignments: %v", err)
		return
	}
	
	// 2. Get ALL tasks with time entries for yesterday at once  
	allTasksWithTime := getFilteredTasksWithTimeout(startTime, endTime, []string{}, "")
	if len(allTasksWithTime) == 0 {
		logger.Info("No tasks with time entries found for yesterday")
		return
	}
	
	// 3. Add comments to all tasks at once
	allTasksWithTime = addCommentsToTasks(allTasksWithTime, startTime, endTime)
	
	logger.Infof("Successfully fetched data: %d user-project assignments, %d tasks with time", 
		len(userProjectMap), len(allTasksWithTime))

	// Process each user with pre-fetched data
	for _, user := range users {
		logger.Infof("Processing daily update for user %s (%s)", user.ID, user.RealName)

		// Filter tasks for this user based on their project assignments
		userTasks := filterTasksForUser(user.ID, userProjectMap, allTasksWithTime)
		if len(userTasks) == 0 {
			logger.Infof("No tasks found for user %s in the specified period", user.ID)
			continue
		}

		logger.Infof("Found %d tasks for user %s", len(userTasks), user.ID)

		// Group by project and send
		filteredTasksGroupedByProject := groupTasksByProject(userTasks)
		sendTasksGroupedByProjectToUser(user.ID, filteredTasksGroupedByProject)

		// Small delay between users to avoid rate limiting
		time.Sleep(250 * time.Millisecond)

		logger.Infof("Completed daily update for user %s", user.ID)
	}

	logger.Infof("Completed optimized daily updates for all %d users", len(users))
}

func addCronJob(scheduler *cron.Cron, envVar, defaultSchedule, jobName string, logger *Logger, cmd func()) {
	schedule := os.Getenv(envVar)
	if schedule == "" {
		schedule = defaultSchedule
	}
	_, err := scheduler.AddFunc(schedule, func() {
		logger.Debugf("Running scheduled %s", jobName)
		cmd()
	})
	if err != nil {
		logger.Fatalf("Critical error: Failed to schedule %s cron job: %v", jobName, err)
	}
}

func showHelp() {
	fmt.Println("Usage: observe-yor-estimates [command]")
	fmt.Println("\nAvailable commands:")
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  --test-command=\"/oye ...\"  - Run a test slash command locally and DM the result to 'Szymon'")
}

// RunTestCommand simulates handling a Slack slash command locally and sends results via DM to the given Slack user name
func RunTestCommand(logger *Logger, fullCommand string, slackUserName string) error {
	logger.Infof("Running test command: %s", fullCommand)

	// Normalize like the HTTP path
	commandText := strings.ToLower(strings.TrimSpace(fullCommand))
	commandText = strings.Replace(commandText, "/oye", "", 1)

	// Parse inputs using the same helpers as HTTP path
	projectName, err := confirmProject(commandText)
	if err != nil {
		return fmt.Errorf("failed to confirm project: %w", err)
	}

	percentage, err := confirmPercentage(commandText)
	if err != nil {
		return fmt.Errorf("failed to confirm percentage: %w", err)
	}

	// Stricter validation mirroring HTTP handler behavior
	fields := strings.Fields(commandText)
	firstWord := ""
	if len(fields) > 0 {
		firstWord = fields[0]
	}
	if firstWord == "project" && projectName == "" {
		return fmt.Errorf("Missing project name. Use: `/oye project [project name] for [period]` or `/oye project [project name] over [percentage] for [period]`")
	}
	if firstWord == "over" && percentage == "" {
		return fmt.Errorf("Missing percentage value. Use: `/oye over [percentage] for [period]` or `/oye project [project name] over [percentage] for [period]`")
	}

	startTime, endTime, err := confirmPeriod(commandText)
	if err != nil {
		return fmt.Errorf("failed to confirm period: %w", err)
	}

	// Prepare data like the async path
	filteredTasks := getFilteredTasksWithTimeout(startTime, endTime, []string{projectName}, percentage)
	if len(filteredTasks) == 0 {
		logger.Info("No tasks found for test command")
		return nil
	}

	filteredTasks = addCommentsToTasksWithTimeout(filteredTasks, startTime, endTime)
	grouped := groupTasksByProject(filteredTasks)

	// Lookup Slack user ID by name (real or display)
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	userID, err := FindSlackUserIDByName(db, slackUserName)
	if err != nil {
		return fmt.Errorf("failed to find slack user '%s': %w", slackUserName, err)
	}

	// Send via DM path
	sendTasksGroupedByProjectToUser(userID, grouped)
	return nil
}

// OPTIMIZATION HELPER FUNCTIONS FOR DAILY UPDATES

// getAllUserProjectAssignments gets all user-project assignments in a single query
func getAllUserProjectAssignments(db *sql.DB) (map[string][]string, error) {
	query := `
		SELECT upa.slack_user_id, p.name
		FROM user_project_assignments upa
		INNER JOIN projects p ON upa.project_id = p.id
		ORDER BY upa.slack_user_id, p.name
	`
	
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query user project assignments: %w", err)
	}
	defer rows.Close()
	
	userProjectMap := make(map[string][]string)
	for rows.Next() {
		var userID, projectName string
		if err := rows.Scan(&userID, &projectName); err != nil {
			continue // Skip invalid rows
		}
		userProjectMap[userID] = append(userProjectMap[userID], projectName)
	}
	
	return userProjectMap, nil
}



// filterTasksForUser filters tasks for a specific user based on their project assignments
func filterTasksForUser(userID string, userProjectMap map[string][]string, allTasks []TaskInfo) []TaskInfo {
	// Get user's project names (case insensitive)
	userProjects, exists := userProjectMap[userID]
	if !exists {
		return []TaskInfo{} // User has no project assignments
	}
	
	// Convert to lowercase for comparison
	userProjectsLower := make(map[string]bool)
	for _, project := range userProjects {
		userProjectsLower[strings.ToLower(project)] = true
	}
	
	// Get all tasks map for project lookups
	db, err := GetDB()
	if err != nil {
		return []TaskInfo{}
	}
	
	allTasksMap, err := getAllTasks(db)
	if err != nil {
		return []TaskInfo{}
	}
	
	var userTasks []TaskInfo
	for _, task := range allTasks {
		// Find project name for this task
		projectName := getProjectNameForTask(task.TaskID, allTasksMap)
		if projectName == "" {
			continue
		}
		
		// Check if user is assigned to this project
		if userProjectsLower[strings.ToLower(projectName)] {
			// Comments are already added to the task from addCommentsToTasks
			userTasks = append(userTasks, task)
		}
	}
	
	return userTasks
}
