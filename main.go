package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	var logger *Logger
	if getOutputJSON() {
		appLogger = NewLoggerForJSON()
		logger = appLogger
	} else {
		logger = NewLogger()
	}

	err := godotenv.Load()
	if err != nil {
		if isNetlifyBuild() {
			logger.Info("No .env file found (normal for Netlify builds)")
		} else {
			logger.Warnf("Warning: Failed to load .env file: %v", err)
		}
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
	switch command {
	case "--help", "-h", "help":
		showHelp()
	case "--version", "version":
		fmt.Println("Observe-Yor-Estimates v1.0.0")
	case "--build-test", "build-test":
		fmt.Println("Build test successful - binary is working correctly")
	case "--init-db", "init-db":
		_, err := GetDB()
		if err != nil {
			logger.Fatalf("Failed to initialize database: %v", err)
		}
		logger.Info("Database initialized successfully")
	case "update":
		if len(args) < 2 {
			logger.Error("Error: update command requires a period (daily, weekly, or monthly)")
			return
		}
		period := args[1]
		if period != "daily" && period != "weekly" && period != "monthly" {
			logger.Errorf("Error: invalid period '%s'. Must be one of daily, weekly, or monthly.", period)
			return
		}
		logger.Infof("Running %s update command", period)

		// Check if we have Slack API context (from Netlify function)
		if os.Getenv("SLACK_BOT_TOKEN") != "" && os.Getenv("CHANNEL_ID") != "" {
			logger.Info("Using direct Slack API for context-aware response")
			handleDirectSlackUpdate(period)
		} else {
			// Fallback to original behavior
			responseURL := getResponseURL()
			outputJSON := getOutputJSON()
			SendSlackUpdate(period, responseURL, outputJSON)
		}
	case "sync-time-entries":
		logger.Info("Running time entries sync command")
		if err := SyncTimeEntriesToDatabase("", ""); err != nil {
			logger.Errorf("Time entries sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Time entries sync completed successfully")
	case "sync-tasks":
		logger.Info("Running tasks sync command")
		if err := SyncTasksToDatabaseFull(); err != nil {
			logger.Errorf("Tasks sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Tasks sync completed successfully")
	case "full-sync":
		logger.Info("Running full synchronization command")
		responseURL := getResponseURL()
		outputJSON := getOutputJSON()
		if outputJSON {
			SendFullSyncJSON()
		} else if responseURL != "" {
			SendFullSyncWithResponseURL(responseURL)
		} else {
			if err := FullSyncAll(); err != nil {
				logger.Errorf("Full sync failed: %v", err)
				os.Exit(1)
			}
			logger.Info("Full synchronization completed successfully")
		}
	case "full-sync-tasks-only":
		logger.Info("Running full tasks synchronization only")
		if err := SyncTasksToDatabaseFull(); err != nil {
			logger.Errorf("Full tasks sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full tasks synchronization completed successfully")
	case "full-sync-entries-only":
		logger.Info("Running full time entries synchronization only")
		if err := FullSyncTimeEntriesToDatabase(); err != nil {
			logger.Errorf("Full time entries sync failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Full time entries synchronization completed successfully")
	case "threshold-check":
		logger.Info("Running threshold monitoring check")
		if err := RunThresholdMonitoring(); err != nil {
			logger.Errorf("Threshold monitoring failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Threshold monitoring completed successfully")
	case "process-orphaned":
		logger.Info("Processing orphaned time entries")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}

		// Show current count before processing
		if count, err := GetOrphanedTimeEntriesCount(db); err != nil {
			logger.Errorf("Failed to get orphaned entries count: %v", err)
			os.Exit(1)
		} else {
			logger.Infof("Found %d orphaned time entries to process", count)
			if count == 0 {
				logger.Info("No orphaned time entries to process")
				return
			}
		}

		if err := ProcessOrphanedTimeEntries(db); err != nil {
			logger.Errorf("Failed to process orphaned time entries: %v", err)
			os.Exit(1)
		}

		// Show remaining count after processing
		if count, err := GetOrphanedTimeEntriesCount(db); err != nil {
			logger.Warnf("Failed to get remaining orphaned entries count: %v", err)
		} else {
			logger.Infof("Remaining orphaned time entries: %d", count)
		}
		logger.Info("Orphaned time entries processing completed successfully")
	case "cleanup-orphaned":
		if len(args) < 2 {
			logger.Error("Error: cleanup-orphaned command requires number of days (e.g., cleanup-orphaned 30)")
			return
		}

		var days int
		if _, err := fmt.Sscanf(args[1], "%d", &days); err != nil {
			logger.Errorf("Error: invalid number of days '%s'", args[1])
			return
		}

		if days < 1 {
			logger.Error("Error: number of days must be at least 1")
			return
		}

		logger.Infof("Cleaning up orphaned time entries older than %d days", days)
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}

		if err := CleanupOldOrphanedEntries(db, days); err != nil {
			logger.Errorf("Failed to cleanup orphaned entries: %v", err)
			os.Exit(1)
		}
		logger.Info("Orphaned entries cleanup completed successfully")

	case "sync-users":
		logger.Info("Syncing users from TimeCamp")
		if err := SyncUsersFromTimeCamp(); err != nil {
			logger.Errorf("Error syncing users: %v", err)
			os.Exit(1)
		}
		logger.Info("Users synced successfully")
	case "list-users":
		logger.Info("Listing users from database")
		if err := ListUsers(); err != nil {
			logger.Errorf("Error listing users: %v", err)
			os.Exit(1)
		}
	case "active-users":
		logger.Info("Getting active user IDs from time entries")
		userIDs, err := GetActiveUserIDs()
		if err != nil {
			logger.Errorf("Error getting active user IDs: %v", err)
			os.Exit(1)
		}
		fmt.Println("Active user IDs from time entries:")
		for _, userID := range userIDs {
			fmt.Printf("- %d\n", userID)
		}
	case "add-user":
		if len(args) < 4 {
			logger.Error("Error: add-user command requires: <user_id> <username> <display_name>")
			return
		}
		var userID int
		if _, err := fmt.Sscanf(args[1], "%d", &userID); err != nil {
			logger.Errorf("Error: invalid user ID '%s'", args[1])
			return
		}
		username := args[2]
		displayName := args[3]
		logger.Infof("Adding user: %d - %s (%s)", userID, displayName, username)
		if err := AddUser(userID, username, displayName); err != nil {
			logger.Errorf("Error adding user: %v", err)
			os.Exit(1)
		}
		logger.Info("User added successfully")
	case "sync-projects":
		logger.Info("Syncing projects table from task hierarchy")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}
		if err := SyncProjectsFromTasks(db); err != nil {
			logger.Errorf("Error syncing projects: %v", err)
			os.Exit(1)
		}
		logger.Info("Projects synced successfully")
	case "list-projects":
		logger.Info("Listing all projects from database")
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}
		projects, err := GetAllProjects(db)
		if err != nil {
			logger.Errorf("Error listing projects: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d projects:\n", len(projects))
		for _, project := range projects {
			fmt.Printf("- %s (TimeCamp Task ID: %d)\n", project.Name, project.TimeCampTaskID)
		}
	case "test-command":
		if len(args) < 2 {
			logger.Error("Error: test-command requires a command string (e.g., test-command \"/oye all this month\")")
			return
		}
		testCommand := args[1]
		logger.Infof("Testing local OYE command: %s", testCommand)

		// Remove "/oye " prefix if present
		testCommand = strings.TrimPrefix(testCommand, "/oye ")
		testCommand = strings.TrimSpace(testCommand)

		// Check for project assignment commands FIRST (before project parsing)
		text := strings.ToLower(strings.TrimSpace(testCommand))
		if strings.HasPrefix(text, "assign ") || strings.HasPrefix(text, "unassign ") ||
			text == "my-projects" || text == "available-projects" {

			fmt.Printf("\n=== OYE Test Command Results ===\n")
			fmt.Printf("Original command: %s\n", testCommand)
			fmt.Printf("Command type: Project assignment command\n")
			fmt.Printf("=====================================\n\n")

			// Create mock request for project assignment commands
			req := &SlackCommandRequest{
				UserID: "TEST_USER",
				Text:   testCommand,
			}

			// Test the project assignment handler
			router := NewSmartRouter()
			if err := router.HandleProjectAssignmentRequest(req); err != nil {
				logger.Errorf("Failed to handle project assignment request: %v", err)
				os.Exit(1)
			}

			fmt.Println("=== Project assignment test completed ===")
			return
		}

		// Parse project name from command if present (only after checking for management commands)
		projectName, remainingText := ParseProjectFromCommand(testCommand)

		fmt.Printf("\n=== OYE Test Command Results ===\n")
		fmt.Printf("Original command: %s\n", testCommand)
		if projectName != "" {
			fmt.Printf("Project filter: %s\n", projectName)
		}
		fmt.Printf("Remaining text: %s\n", remainingText)
		fmt.Printf("=====================================\n\n")

		// Get database connection
		db, err := GetDB()
		if err != nil {
			logger.Errorf("Failed to get database connection: %v", err)
			os.Exit(1)
		}

		// Parse the time period
		router := NewSmartRouter()
		periodInfo := router.parsePeriodFromText(remainingText, "")

		fmt.Printf("Parsed period: %s (type: %s, days: %d)\n\n", periodInfo.DisplayName, periodInfo.Type, periodInfo.Days)

		// Handle project-specific query
		var taskInfos []TaskUpdateInfo
		if projectName != "" && projectName != "all" {
			// Find the project
			projects, err := FindProjectsByName(db, projectName)
			if err != nil {
				logger.Errorf("Failed to find project: %v", err)
				os.Exit(1)
			}

			if len(projects) == 0 {
				fmt.Printf("❌ Project '%s' not found.\n", projectName)
				fmt.Println("\nAvailable projects:")
				allProjects, err := GetAllProjects(db)
				if err == nil {
					for _, p := range allProjects {
						fmt.Printf("- %s\n", p.Name)
					}
				}
				return
			}

			if len(projects) > 1 {
				fmt.Printf("⚠️ Multiple projects found for '%s':\n", projectName)
				for _, p := range projects {
					fmt.Printf("- %s\n", p.Name)
				}
				fmt.Println("\nPlease be more specific.")
				return
			}

			project := projects[0]
			fmt.Printf("Found project: %s (TimeCamp Task ID: %d)\n", project.Name, project.TimeCampTaskID)

			// Get project-specific task data
			taskInfos, err = getTaskChangesWithProject(db, periodInfo.Type, &project.TimeCampTaskID)
			if err != nil {
				logger.Errorf("Failed to get %s changes for project '%s': %v", periodInfo.DisplayName, project.Name, err)
				os.Exit(1)
			}
		} else {
			// Get task data for all projects
			taskInfos, err = getTaskChanges(db, periodInfo.Type)
			if err != nil {
				logger.Errorf("Failed to get %s changes: %v", periodInfo.DisplayName, err)
				os.Exit(1)
			}
		}

		fmt.Printf("\n📊 Results for %s:\n", periodInfo.DisplayName)
		fmt.Printf("Found %d tasks with time entries\n\n", len(taskInfos))

		if len(taskInfos) == 0 {
			fmt.Println("No tasks found with time entries for the specified period.")
		} else {
			for i, task := range taskInfos {
				fmt.Printf("%d. %s\n", i+1, task.Name)
				fmt.Printf("   %s: %s | %s: %s\n", task.CurrentPeriod, task.CurrentTime, task.PreviousPeriod, task.PreviousTime)
				if task.EstimationInfo != "" {
					fmt.Printf("   %s\n", task.EstimationInfo)
				}
				if len(task.Comments) > 0 {
					fmt.Printf("   Comments: %d\n", len(task.Comments))
				}
				fmt.Println()
			}
		}

		fmt.Println("=== Test command completed ===")

		// Additional debugging: Check project task relationships
		if projectName != "" && projectName != "all" {
			fmt.Printf("\n=== Debug: Project Task Relationships ===\n")
			projects, err := FindProjectsByName(db, projectName)
			if err == nil && len(projects) > 0 {
				project := projects[0]
				fmt.Printf("Project: %s (TimeCamp Task ID: %d)\n", project.Name, project.TimeCampTaskID)

				// Check how many tasks are linked to this project
				var taskCount int
				err = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id = (
					SELECT id FROM projects WHERE timecamp_task_id = $1
				)`, project.TimeCampTaskID).Scan(&taskCount)
				if err == nil {
					fmt.Printf("Tasks linked to this project: %d\n", taskCount)
				}

				// Show sample tasks linked to this project
				rows, err := db.Query(`SELECT task_id, name FROM tasks WHERE project_id = (
					SELECT id FROM projects WHERE timecamp_task_id = $1
				) LIMIT 10`, project.TimeCampTaskID)
				if err == nil {
					defer rows.Close()
					fmt.Println("Sample tasks linked to this project:")
					for rows.Next() {
						var taskID int
						var taskName string
						if err := rows.Scan(&taskID, &taskName); err == nil {
							fmt.Printf("- Task %d: %s\n", taskID, taskName)
						}
					}
				}

				// Also check if there are tasks with "Filestage" in the name but not linked to the project
				fmt.Println("\nTasks with 'Filestage' in name (regardless of project):")
				rows2, err := db.Query(`SELECT task_id, name, project_id FROM tasks WHERE LOWER(name) LIKE LOWER('%filestage%') LIMIT 20`)
				if err == nil {
					defer rows2.Close()
					for rows2.Next() {
						var taskID int
						var taskName string
						var projectID sql.NullInt64
						if err := rows2.Scan(&taskID, &taskName, &projectID); err == nil {
							projectStr := "NULL"
							if projectID.Valid {
								projectStr = fmt.Sprintf("%d", projectID.Int64)
							}
							fmt.Printf("- Task %d: %s (project_id: %s)\n", taskID, taskName, projectStr)
						}
					}
				}
			}
		}
	case "test-message-limits":
		logger.Info("Testing Slack message limits with mock data")

		// Generate test tasks with various sizes
		testTasks := generateTestTasks(50) // Generate 50 tasks to test limits

		fmt.Printf("\n=== Message Limits Test ===\n")
		fmt.Printf("Generated %d test tasks\n", len(testTasks))

		// Test formatProjectMessageWithComments
		fmt.Println("\n--- Testing formatProjectMessageWithComments ---")
		messages := formatProjectMessageWithComments("Test Project", testTasks, "monthly")

		fmt.Printf("Generated %d messages\n", len(messages))

		for i, message := range messages {
			validation := validateSlackMessage(message)
			status := "✅ VALID"
			if !validation.IsValid {
				status = "❌ INVALID"
			}

			fmt.Printf("Message %d: %s\n", i+1, status)
			fmt.Printf("  - Blocks: %d/%d\n", validation.BlockCount, MaxSlackBlocks)
			fmt.Printf("  - Characters: %d/%d\n", validation.CharacterCount, MaxSlackMessageChars)
			if !validation.IsValid {
				fmt.Printf("  - Error: %s\n", validation.ErrorMessage)
			}
		}

		// Test individual message validation
		fmt.Println("\n--- Testing Large Single Message ---")
		largeTestTasks := generateTestTasks(100) // Even more tasks
		largeMessage := formatProjectMessage("Large Project", largeTestTasks, "monthly")
		validation := validateSlackMessage(largeMessage)

		status := "✅ VALID"
		if !validation.IsValid {
			status = "❌ INVALID"
		}

		fmt.Printf("Large message validation: %s\n", status)
		fmt.Printf("  - Blocks: %d/%d\n", validation.BlockCount, MaxSlackBlocks)
		fmt.Printf("  - Characters: %d/%d\n", validation.CharacterCount, MaxSlackMessageChars)
		if !validation.IsValid {
			fmt.Printf("  - Error: %s\n", validation.ErrorMessage)
		}

		// Test comment overflow
		fmt.Println("\n--- Testing Comment Overflow ---")
		tasksWithComments := generateTestTasksWithComments(10, 20) // 10 tasks with 20 comments each
		commentMessages := formatProjectMessageWithComments("Comment Heavy Project", tasksWithComments, "daily")

		// Test extreme comment overflow
		fmt.Println("\n--- Testing Extreme Comment Overflow ---")
		extremeTasks := generateTestTasksWithComments(3, 50) // 3 tasks with 50 comments each
		extremeMessages := formatProjectMessageWithComments("Extreme Project", extremeTasks, "daily")

		fmt.Printf("Generated %d messages for comment-heavy tasks\n", len(commentMessages))

		for i, message := range commentMessages {
			validation := validateSlackMessage(message)
			status := "✅ VALID"
			if !validation.IsValid {
				status = "❌ INVALID"
			}

			fmt.Printf("Comment Message %d: %s\n", i+1, status)
			fmt.Printf("  - Blocks: %d/%d\n", validation.BlockCount, MaxSlackBlocks)
			fmt.Printf("  - Characters: %d/%d\n", validation.CharacterCount, MaxSlackMessageChars)
			if !validation.IsValid {
				fmt.Printf("  - Error: %s\n", validation.ErrorMessage)
			}
		}

		fmt.Printf("\nGenerated %d messages for extreme comment tasks\n", len(extremeMessages))

		for i, message := range extremeMessages {
			validation := validateSlackMessage(message)
			status := "✅ VALID"
			if !validation.IsValid {
				status = "❌ INVALID"
			}

			fmt.Printf("Extreme Message %d: %s\n", i+1, status)
			fmt.Printf("  - Blocks: %d/%d\n", validation.BlockCount, MaxSlackBlocks)
			fmt.Printf("  - Characters: %d/%d\n", validation.CharacterCount, MaxSlackMessageChars)
			if !validation.IsValid {
				fmt.Printf("  - Error: %s\n", validation.ErrorMessage)
			}
		}

		fmt.Println("\n=== Message Limits Test Completed ===")

		// Summary
		allValid := true
		for _, messages := range [][]SlackMessage{messages, commentMessages, extremeMessages, {largeMessage}} {
			for _, message := range messages {
				if !validateSlackMessage(message).IsValid {
					allValid = false
					break
				}
			}
		}

		if allValid {
			fmt.Println("🎉 All messages passed validation!")
		} else {
			fmt.Println("⚠️  Some messages failed validation - review the fixes needed")
		}
	default:
		logger.Warnf("Unknown command line argument: %s", command)
		showHelp()
	}
}

func setupCronJobs(logger *Logger) {
	cronScheduler := cron.New()

	addCronJob(cronScheduler, "TASK_SYNC_SCHEDULE", "0 */3 * * *", "task sync", logger, func() {
		if err := SyncTasksToDatabaseIncremental(); err != nil {
			logger.Errorf("Scheduled task sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "TIME_ENTRIES_SYNC_SCHEDULE", "*/10 * * * *", "time entries sync", logger, func() {
		if err := SyncTimeEntriesToDatabase("", ""); err != nil {
			logger.Errorf("Scheduled time entries sync failed: %v", err)
		}
	})

	addCronJob(cronScheduler, "DAILY_UPDATE_SCHEDULE", "0 6 * * *", "daily Slack update", logger, func() {
		SendSlackUpdate("daily", "", false)
	})

	// Add threshold monitoring cron job (every 15 minutes)
	addCronJob(cronScheduler, "THRESHOLD_MONITORING_SCHEDULE", "*/15 * * * *", "threshold monitoring", logger, func() {
		if err := RunThresholdMonitoring(); err != nil {
			logger.Errorf("Threshold monitoring failed: %v", err)
		}
	})

	// Add orphaned time entries processing cron job (every hour)
	addCronJob(cronScheduler, "ORPHANED_PROCESSING_SCHEDULE", "0 * * * *", "orphaned time entries processing", logger, func() {
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
	fmt.Println("  update <period>          - Send Slack update for a period (daily, weekly, monthly)")
	fmt.Println("  sync-time-entries        - Sync recent time entries (last day)")
	fmt.Println("  sync-tasks               - Full sync of all tasks (manual operation)")
	fmt.Println("  full-sync                - Full sync of all tasks and time entries")
	fmt.Println("  full-sync-tasks-only     - Full sync of tasks only")
	fmt.Println("  full-sync-entries-only   - Full sync of time entries only")
	fmt.Println("  threshold-check          - Manual threshold monitoring check")
	fmt.Println("  process-orphaned         - Process orphaned time entries")
	fmt.Println("  cleanup-orphaned <days>   - Clean up orphaned time entries older than specified days")
	fmt.Println("")
	fmt.Println("User management commands:")
	fmt.Println("  sync-users               - Sync users from TimeCamp to the database")
	fmt.Println("  list-users               - Show all users in the database")
	fmt.Println("  active-users             - Show user IDs that have time entries")
	fmt.Println("  add-user <id> <user> <name> - Add a specific user to the database")
	fmt.Println("")
	fmt.Println("Project management commands:")
	fmt.Println("  sync-projects            - Sync projects table from task hierarchy")
	fmt.Println("  list-projects            - Show all projects in the database")
	fmt.Println("")
	fmt.Println("Testing commands:")
	fmt.Println("  test-command <command>   - Test OYE commands locally (e.g., test-command \"/oye Filestage.io this month\")")
	fmt.Println("")
	fmt.Println("  --version, version         - Show application version")
	fmt.Println("  --help, -h, help         - Show help message")
	fmt.Println("\nSync Behavior:")
	fmt.Println("  • Cron jobs use incremental sync every 3 hours (only process changed tasks)")
	fmt.Println("  • Manual commands use full sync (process all tasks)")
	fmt.Println("  • Uses TimeCamp API minimal option for optimized performance")
	fmt.Println("\nThreshold Monitoring:")
	fmt.Println("  • Automatic alerts for tasks crossing 50%, 80%, 90%, and 100% thresholds")
	fmt.Println("  • Runs every 15 minutes via cron job")
	fmt.Println("  • Manual Slack commands: /oye over <percentage> <period>")
	fmt.Println("\nSlack Integration:")
	fmt.Println("  Set up /oye command in Slack to point to /slack/oye endpoint")
	fmt.Println("  Requires SLACK_BOT_TOKEN environment variable for direct responses")
}

func getResponseURL() string {
	// First check command line arguments
	for i, arg := range os.Args {
		if (arg == "--response-url" || arg == "-r") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	// If not found in args, check environment variable (set by Netlify functions)
	return os.Getenv("RESPONSE_URL")
}

func getOutputJSON() bool {
	// First check command line arguments
	for _, arg := range os.Args {
		if arg == "--json" {
			return true
		}
	}
	// If not found in args, check environment variable (set by Netlify functions)
	return os.Getenv("OUTPUT_JSON") == "true"
}

func handleDirectSlackUpdate(period string) {
	logger := GetGlobalLogger()
	logger.Infof("Handling direct Slack update for period: %s", period)

	// Create Slack API client and context from environment
	slackClient := NewSlackAPIClientFromEnv()
	ctx := GetContextFromEnv()

	// Get database connection
	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to open database connection: %v", err)
		slackClient.SendErrorResponse(ctx, "Database connection failed")
		return
	}

	// Get task data
	taskInfos, err := getTaskChanges(db, period)
	if err != nil {
		logger.Errorf("Failed to get %s task changes: %v", period, err)
		slackClient.SendErrorResponse(ctx, fmt.Sprintf("Failed to get %s changes", period))
		return
	}

	// Send direct response to Slack
	if len(taskInfos) == 0 {
		slackClient.SendNoChangesMessage(ctx, period)
	} else {
		// Default to personal update for now - can be enhanced with user preferences
		slackClient.SendPersonalUpdate(ctx, taskInfos, period)
	}

	logger.Infof("Direct Slack update completed for period: %s", period)
}

// generateTestTasks creates mock TaskUpdateInfo for testing message limits
func generateTestTasks(count int) []TaskUpdateInfo {
	tasks := make([]TaskUpdateInfo, count)

	for i := 0; i < count; i++ {
		tasks[i] = TaskUpdateInfo{
			TaskID:           1000 + i,
			ParentID:         100 + (i % 10),
			Name:             fmt.Sprintf("Test Task %d - This is a sample task name for testing message limits and various formatting scenarios", i+1),
			EstimationInfo:   fmt.Sprintf("Estimation: %d-%d hours | 🟢 %.1f%% (on track)", (i%5)+1, (i%5)+5, float64((i*7)%100)),
			EstimationStatus: "",
			CurrentPeriod:    "This Month",
			CurrentTime:      fmt.Sprintf("%dh %dm", (i*3)%24, (i*7)%60),
			PreviousPeriod:   "Previous Month",
			PreviousTime:     fmt.Sprintf("%dh %dm", (i*2)%15, (i*5)%60),
			DaysWorked:       (i % 10) + 1,
			Comments:         generateTestComments(i % 5), // 0-4 comments per task
			UserBreakdown: map[int]UserTimeContribution{
				100 + (i % 3): {
					UserID:       100 + (i % 3),
					CurrentTime:  fmt.Sprintf("%dh %dm", (i*2)%12, (i*3)%60),
					PreviousTime: fmt.Sprintf("%dh %dm", (i)%8, (i*2)%60),
				},
			},
		}
	}

	return tasks
}

// generateTestTasksWithComments creates mock TaskUpdateInfo with many comments for testing
func generateTestTasksWithComments(taskCount, commentsPerTask int) []TaskUpdateInfo {
	tasks := make([]TaskUpdateInfo, taskCount)

	for i := 0; i < taskCount; i++ {
		tasks[i] = TaskUpdateInfo{
			TaskID:           2000 + i,
			ParentID:         200 + (i % 5),
			Name:             fmt.Sprintf("Comment Heavy Task %d - Testing comment overflow handling", i+1),
			EstimationInfo:   fmt.Sprintf("Estimation: %d hours | 🟠 %.1f%% (high usage)", (i%3)+3, float64((i*11)%120)),
			EstimationStatus: "",
			CurrentPeriod:    "Today",
			CurrentTime:      fmt.Sprintf("%dh %dm", (i*4)%15, (i*9)%60),
			PreviousPeriod:   "Before Today",
			PreviousTime:     fmt.Sprintf("%dh %dm", (i*3)%10, (i*6)%60),
			DaysWorked:       (i % 7) + 1,
			Comments:         generateTestComments(commentsPerTask),
			UserBreakdown: map[int]UserTimeContribution{
				200 + (i % 2): {
					UserID:       200 + (i % 2),
					CurrentTime:  fmt.Sprintf("%dh %dm", (i*3)%8, (i*4)%60),
					PreviousTime: fmt.Sprintf("%dh %dm", (i*2)%6, (i*3)%60),
				},
			},
		}
	}

	return tasks
}

// generateTestComments creates mock comments for testing
func generateTestComments(count int) []string {
	if count == 0 {
		return []string{}
	}

	comments := make([]string, count)
	sampleComments := []string{
		"This is a test comment for validating message formatting and character limits in Slack messages.",
		"Working on implementing the new feature as discussed in the previous meeting with the team.",
		"Found an issue with the database connection that needs to be resolved before proceeding further.",
		"Updated the documentation to reflect the latest changes and improvements made to the system.",
		"Completed the code review and submitted the pull request for team review and feedback.",
		"Need to schedule a follow-up meeting to discuss the next steps and project timeline.",
		"Investigating the performance issues reported by users and working on optimization solutions.",
		"Added new unit tests to improve code coverage and ensure better quality assurance.",
		"Refactored the legacy code to use modern patterns and improve maintainability.",
		"Coordinating with the design team to finalize the user interface specifications.",
	}

	for i := 0; i < count; i++ {
		commentIndex := i % len(sampleComments)
		comments[i] = fmt.Sprintf("%s (Comment #%d)", sampleComments[commentIndex], i+1)
	}

	return comments
}
