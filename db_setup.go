package main

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

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
	// Check for Supabase environment variables
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
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
		// Return empty string to trigger error - all required vars must be set
		return ""
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "require"
	}

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

		// Test the connection
		if err := db.Ping(); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to ping database: %w", err)
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

	logger.Debug("Tasks table already exists")
	return nil
}

func createTasksTable(db *sql.DB) error {
	logger := GetGlobalLogger()

	createTableSQL := `CREATE TABLE tasks (
task_id INTEGER PRIMARY KEY,
parent_id INTEGER NOT NULL,
assigned_by INTEGER NOT NULL,
name TEXT NOT NULL,
level INTEGER NOT NULL,
root_group_id INTEGER NOT NULL
);`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tasks table: %w", err)
	}

	logger.Info("Tasks table created successfully")
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
