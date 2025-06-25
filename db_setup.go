package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// Global database connection
var (
	globalDB *sql.DB
	dbMutex  sync.RWMutex
	dbOnce   sync.Once
	initErr  error
)

// getDBConnectionString returns the PostgreSQL connection string from environment variables
func getDBConnectionString() string {
	logger := GetGlobalLogger()

	// Check for Supabase environment variables
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		logger.Debug("Using DATABASE_URL environment variable for database connection")
		return dbURL
	}

	// Fallback to individual components
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	if host == "" || user == "" || password == "" || dbname == "" {
		logger.Error("Database configuration missing! Required environment variables:")
		logger.Error("  Either set DATABASE_URL")
		logger.Error("  Or set all of: DB_HOST, DB_USER, DB_PASSWORD, DB_NAME")
		logger.Error("  Optional: DB_PORT (defaults to 5432), DB_SSLMODE (defaults to require)")
		// Return empty string to trigger error - all required vars must be set
		return ""
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "require"
	}

	logger.Debug("Using individual DB_* environment variables for database connection")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

// isNetlifyRuntime checks if we're running in a Netlify serverless function (runtime)
func isNetlifyRuntime() bool {
	// Netlify runtime sets these environment variables
	return os.Getenv("LAMBDA_TASK_ROOT") != "" ||
		os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" ||
		os.Getenv("NETLIFY_DEV") != ""
}

// validateDatabaseWriteAccess tests if the database is writable
func validateDatabaseWriteAccess() error {
	logger := GetGlobalLogger()

	db, err := GetDB()
	if err != nil {
		// Check if this is an IPv6 connectivity issue
		if strings.Contains(err.Error(), "network is unreachable") && strings.Contains(err.Error(), "dial tcp [") {
			return fmt.Errorf("failed to connect to database: IPv6 connectivity issue detected. The database hostname only resolves to IPv6 addresses but your system cannot reach IPv6 networks. This is a common issue with some network configurations. Consider: 1) Enabling IPv6 connectivity on your system/network, 2) Using a different database that supports IPv4, or 3) Using a VPN/proxy that supports IPv6. Original error: %w", err)
		}
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Try to create a test table to verify write access
	testQuery := `CREATE TABLE IF NOT EXISTS write_test (id SERIAL PRIMARY KEY, test_value TEXT)`
	_, err = db.Exec(testQuery)
	if err != nil {
		return fmt.Errorf("database write test failed: %w", err)
	}

	// Clean up test table
	_, err = db.Exec(`DROP TABLE IF EXISTS write_test`)
	if err != nil {
		logger.Warnf("Failed to clean up write test table: %v", err)
	}

	logger.Debug("Database write access validated successfully")
	return nil
}

// GetDB returns a shared connection to the PostgreSQL database, creating it once if needed
func GetDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		logger := GetGlobalLogger()
		connStr := getDBConnectionString()
		if connStr == "" {
			initErr = fmt.Errorf("database connection string not configured - please set DATABASE_URL or individual DB_* environment variables")
			return
		}

		logger.Debugf("Initializing database connection to PostgreSQL")

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			initErr = fmt.Errorf("failed to open database connection: %w", err)
			return
		}

		// Set connection pool settings with timeouts optimized for serverless
		// Optimized connection pool settings for batch operations
		db.SetConnMaxLifetime(time.Minute * 5)  // Increased for better performance
		db.SetMaxOpenConns(25)                  // Increased for concurrent batch operations
		db.SetMaxIdleConns(10)                  // Increased to keep connections alive
		db.SetConnMaxIdleTime(90 * time.Second) // Close idle connections after 90s

		// Test the connection with shorter timeout for faster failure detection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			db.Close()
			// Provide more helpful error messages for common connectivity issues
			if strings.Contains(err.Error(), "network is unreachable") && strings.Contains(err.Error(), "dial tcp [") {
				initErr = fmt.Errorf("failed to ping database: IPv6 connectivity issue detected. The database hostname only resolves to IPv6 addresses but your system cannot reach IPv6 networks. This suggests a network configuration issue. Original error: %w", err)
			} else {
				initErr = fmt.Errorf("failed to ping database (connection timeout after 10s): %w", err)
			}
			return
		}

		if err := migrateTasksTable(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to migrate tasks table: %w", err)
			return
		}

		if err := migrateTaskHistoryTable(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to migrate task_history table: %w", err)
			return
		}

		if err := migrateTimeEntriesTable(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to migrate time_entries table: %w", err)
			return
		}

		if err := migrateUsersTable(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to migrate users table: %w", err)
			return
		}

		if err := migrateProjectsTable(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to migrate projects table: %w", err)
			return
		}

		logger.Info("Database connection established and tables migrated successfully")

		dbMutex.Lock()
		globalDB = db
		dbMutex.Unlock()
	})

	if initErr != nil {
		return nil, initErr
	}

	dbMutex.RLock()
	defer dbMutex.RUnlock()

	if globalDB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	return globalDB, nil
}

// CloseDB closes the global database connection
func CloseDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if globalDB != nil {
		err := globalDB.Close()
		globalDB = nil
		return err
	}
	return nil
}

// migrateTasksTable ensures the tasks table exists and matches the desired schema.
func migrateTasksTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if table exists (PostgreSQL way)
	row := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'tasks');")
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if tasks table exists: %w", err)
	}

	if !exists {
		// Table does not exist, create it
		logger.Info("Tasks table does not exist, creating it")
		return createTasksTable(db)
	}

	logger.Debug("Tasks table already exists, checking for columns")

	// Check if archived column exists and add it if missing
	if err := ensureArchivedColumn(db); err != nil {
		return err
	}

	// Check if project_id column exists and add it if missing
	return ensureProjectIdColumn(db)
}

func createTasksTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE tasks (
task_id INTEGER PRIMARY KEY,
parent_id INTEGER NOT NULL,
assigned_by INTEGER NOT NULL,
name TEXT NOT NULL,
level INTEGER NOT NULL,
root_group_id INTEGER NOT NULL,
archived INTEGER DEFAULT 0
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tasks table: %w", err)
	}

	logger.Info("Tasks table created successfully")
	return nil
}

// ensureArchivedColumn checks if the archived column exists in the tasks table and adds it if missing
func ensureArchivedColumn(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if archived column exists
	checkColumnSQL := `SELECT column_name 
		FROM information_schema.columns 
		WHERE table_schema = 'public' 
		AND table_name = 'tasks' 
		AND column_name = 'archived';`

	var columnName string
	err := db.QueryRow(checkColumnSQL).Scan(&columnName)

	if err == sql.ErrNoRows {
		// Column doesn't exist, add it
		logger.Info("Adding archived column to tasks table")
		alterTableSQL := `ALTER TABLE tasks ADD COLUMN archived INTEGER DEFAULT 0;`
		_, err := db.Exec(alterTableSQL)
		if err != nil {
			return fmt.Errorf("failed to add archived column to tasks table: %w", err)
		}
		logger.Info("Archived column added successfully to tasks table")
	} else if err != nil {
		return fmt.Errorf("error checking for archived column: %w", err)
	} else {
		logger.Debug("Archived column already exists in tasks table")
	}

	return nil
}

// ensureProjectIdColumn checks if the project_id column exists in the tasks table and adds it if missing
func ensureProjectIdColumn(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if project_id column exists
	checkColumnSQL := `SELECT column_name 
		FROM information_schema.columns 
		WHERE table_schema = 'public' 
		AND table_name = 'tasks' 
		AND column_name = 'project_id';`

	var columnName string
	err := db.QueryRow(checkColumnSQL).Scan(&columnName)

	if err == sql.ErrNoRows {
		// Column doesn't exist, add it
		logger.Info("Adding project_id column to tasks table")
		alterTableSQL := `ALTER TABLE tasks ADD COLUMN project_id INTEGER REFERENCES projects(id);`
		_, err := db.Exec(alterTableSQL)
		if err != nil {
			return fmt.Errorf("failed to add project_id column to tasks table: %w", err)
		}
		logger.Info("Project_id column added successfully to tasks table")
	} else if err != nil {
		return fmt.Errorf("error checking for project_id column: %w", err)
	} else {
		logger.Debug("Project_id column already exists in tasks table")
	}

	return nil
}

// migrateTaskHistoryTable ensures the task_history table exists and matches the desired schema.
func migrateTaskHistoryTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if table exists (PostgreSQL way)
	row := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'task_history');")
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if task_history table exists: %w", err)
	}

	if !exists {
		// Table does not exist, create it
		logger.Info("Task history table does not exist, creating it")
		return createTaskHistoryTable(db)
	}

	logger.Debug("Task history table already exists")
	return nil
}

func createTaskHistoryTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE task_history (
id SERIAL PRIMARY KEY,
task_id INTEGER NOT NULL,
name TEXT NOT NULL,
timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
change_type TEXT NOT NULL,
previous_value TEXT,
current_value TEXT,
FOREIGN KEY (task_id) REFERENCES tasks(task_id)
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create task_history table: %w", err)
	}

	logger.Info("Task history table created successfully")
	return nil
}

// migrateTimeEntriesTable ensures the time_entries table exists and matches the desired schema.
func migrateTimeEntriesTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if table exists (PostgreSQL way)
	row := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'time_entries');")
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if time_entries table exists: %w", err)
	}

	if !exists {
		// Table does not exist, create it
		logger.Info("Time entries table does not exist, creating it")
		return createTimeEntriesTable(db)
	}

	logger.Debug("Time entries table already exists")
	return nil
}

func createTimeEntriesTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE time_entries (
id INTEGER PRIMARY KEY,
task_id INTEGER NOT NULL,
user_id INTEGER NOT NULL,
date TEXT NOT NULL,
start_time TEXT,
end_time TEXT,
duration INTEGER NOT NULL,
description TEXT,
billable INTEGER DEFAULT 0,
locked INTEGER DEFAULT 0,
modify_time TEXT,
FOREIGN KEY (task_id) REFERENCES tasks(task_id)
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create time_entries table: %w", err)
	}

	logger.Info("Time entries table created successfully")
	return nil
}

// migrateUsersTable ensures the users table exists and matches the desired schema.
func migrateUsersTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if table exists (PostgreSQL way)
	row := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users');")
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if users table exists: %w", err)
	}

	if !exists {
		// Table does not exist, create it
		logger.Info("Users table does not exist, creating it")
		return createUsersTable(db)
	}

	logger.Debug("Users table already exists")
	return nil
}

func createUsersTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE users (
user_id INTEGER PRIMARY KEY,
username TEXT NOT NULL,
display_name TEXT,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	logger.Info("Users table created successfully")
	return nil
}

// GetUserDisplayName returns the display name for a user ID, falling back to username or user ID
func GetUserDisplayName(db *sql.DB, userID int) string {
	var displayName, username sql.NullString
	
	query := `SELECT username, display_name FROM users WHERE user_id = $1`
	err := db.QueryRow(query, userID).Scan(&username, &displayName)
	
	if err != nil {
		// User not found in database, return user ID format as fallback
		return fmt.Sprintf("user%d", userID)
	}
	
	// Return display_name if available, otherwise username, otherwise user ID
	if displayName.Valid && displayName.String != "" {
		return displayName.String
	}
	if username.Valid && username.String != "" {
		return username.String
	}
	return fmt.Sprintf("user%d", userID)
}

// UpsertUser creates or updates a user record
func UpsertUser(db *sql.DB, userID int, username, displayName string) error {
	query := `
		INSERT INTO users (user_id, username, display_name, updated_at) 
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) 
		DO UPDATE SET 
			username = EXCLUDED.username,
			display_name = EXCLUDED.display_name,
			updated_at = CURRENT_TIMESTAMP`
	
	_, err := db.Exec(query, userID, username, displayName)
	return err
}

// GetAllUserDisplayNames returns a map of user ID to display name for bulk operations
func GetAllUserDisplayNames(db *sql.DB, userIDs []int) map[int]string {
	if len(userIDs) == 0 {
		return make(map[int]string)
	}
	
	result := make(map[int]string)
	
	// Convert int slice to interface slice for pq.Array
	userIDsInterface := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		userIDsInterface[i] = id
	}
	
	query := `SELECT user_id, username, display_name FROM users WHERE user_id = ANY($1)`
	rows, err := db.Query(query, pq.Array(userIDs))
	if err != nil {
		// If query fails, return fallback names
		for _, userID := range userIDs {
			result[userID] = fmt.Sprintf("user%d", userID)
		}
		return result
	}
	defer rows.Close()
	
	foundUsers := make(map[int]bool)
	for rows.Next() {
		var userID int
		var username, displayName sql.NullString
		
		err := rows.Scan(&userID, &username, &displayName)
		if err != nil {
			continue
		}
		
		// Priority: display_name > username > user ID
		if displayName.Valid && displayName.String != "" {
			result[userID] = displayName.String
		} else if username.Valid && username.String != "" {
			result[userID] = username.String
		} else {
			result[userID] = fmt.Sprintf("user%d", userID)
		}
		foundUsers[userID] = true
	}
	
	// Add fallback names for users not found in the database
	for _, userID := range userIDs {
		if !foundUsers[userID] {
			result[userID] = fmt.Sprintf("user%d", userID)
		}
	}
	
	return result
}

// CheckDatabaseHasTasks returns true if the database has any tasks, false otherwise
func CheckDatabaseHasTasks() (bool, error) {
	db, err := GetDB()
	if err != nil {
		return false, fmt.Errorf("failed to get database connection: %w", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count tasks: %w", err)
	}

	return count > 0, nil
}

// CheckDatabaseHasTimeEntries returns true if the database has any time entries, false otherwise
func CheckDatabaseHasTimeEntries() (bool, error) {
	db, err := GetDB()
	if err != nil {
		return false, fmt.Errorf("failed to get database connection: %w", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM time_entries").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count time entries: %w", err)
	}

	return count > 0, nil
}

// migrateProjectsTable ensures the projects table exists and matches the desired schema.
func migrateProjectsTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if table exists (PostgreSQL way)
	row := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'projects');")
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if projects table exists: %w", err)
	}

	if !exists {
		// Table does not exist, create it
		logger.Info("Projects table does not exist, creating it")
		if err := createProjectsTable(db); err != nil {
			return err
		}
		
		// Populate projects from existing task hierarchy
		logger.Info("Populating projects table from existing task hierarchy")
		return populateProjectsFromTasks(db)
	}

	logger.Debug("Projects table already exists")
	return nil
}

func createProjectsTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE projects (
id SERIAL PRIMARY KEY,
name TEXT NOT NULL UNIQUE,
timecamp_task_id INTEGER NOT NULL UNIQUE,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
FOREIGN KEY (timecamp_task_id) REFERENCES tasks(task_id)
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create projects table: %w", err)
	}

	logger.Info("Projects table created successfully")
	return nil
}

// populateProjectsFromTasks extracts project-level tasks and populates the projects table
func populateProjectsFromTasks(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Find all project-level tasks (level 2 in TimeCamp hierarchy)
	// Level 1 = Basecamp3 (root), Level 2 = Projects, Level 3+ = Sub-tasks
	query := `
		SELECT DISTINCT task_id, name
		FROM tasks
		WHERE level = 2  -- Projects are level 2 tasks
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query project-level tasks: %w", err)
	}
	defer rows.Close()

	projectCount := 0
	for rows.Next() {
		var taskID int
		var name string
		
		if err := rows.Scan(&taskID, &name); err != nil {
			logger.Warnf("Failed to scan project row: %v", err)
			continue
		}

		// Insert project into projects table
		insertQuery := `
			INSERT INTO projects (name, timecamp_task_id) 
			VALUES ($1, $2) 
			ON CONFLICT (timecamp_task_id) DO NOTHING
		`
		
		_, err := db.Exec(insertQuery, name, taskID)
		if err != nil {
			logger.Warnf("Failed to insert project %s (task_id: %d): %v", name, taskID, err)
			continue
		}
		
		projectCount++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating project rows: %w", err)
	}

	logger.Infof("Successfully populated %d projects from task hierarchy", projectCount)
	return nil
}
