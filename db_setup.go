package main

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// Global database connection
var (
	globalDB *sql.DB
	dbMutex  sync.RWMutex
	dbOnce   sync.Once
	initErr  error
)

// getDBPath returns the database path from environment variable or default
func getDBPath() string {
	if path := os.Getenv("DATABASE_PATH"); path != "" {
		return path
	}
	return "./oye.db" // default path
}

// GetDB returns a shared connection to the SQLite database, creating it once if needed
func GetDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		logger := NewLogger()
		dbPath := getDBPath()
		logger.Debugf("Initializing database connection to: %s", dbPath)

		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			initErr = fmt.Errorf("failed to open database at %s: %w", dbPath, err)
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
	logger := NewLogger()

	// Check if table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks';")
	var name string
	err := row.Scan(&name)
	if err == sql.ErrNoRows {
		// Table does not exist, create it
		logger.Info("Tasks table does not exist, creating it")
		return createTasksTable(db)
	} else if err != nil {
		return fmt.Errorf("error checking if tasks table exists: %w", err)
	}

	logger.Debug("Tasks table already exists")
	// Table exists, for now we don't do migration logic,
	// but we could add checks for schema changes in the future.
	return nil
}

func createTasksTable(db *sql.DB) error {
	logger := NewLogger()

	createTableSQL := `CREATE TABLE tasks (
task_id INTEGER PRIMARY KEY,
parent_id INT NOT NULL,
assigned_by INT NOT NULL,
name STRING NOT NULL,
level INT NOT NULL,
root_group_id INT NOT NULL
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
	logger := NewLogger()

	// Check if table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='task_history';")
	var name string
	err := row.Scan(&name)
	if err == sql.ErrNoRows {
		// Table does not exist, create it
		logger.Info("Task history table does not exist, creating it")
		return createTaskHistoryTable(db)
	} else if err != nil {
		return fmt.Errorf("error checking if task_history table exists: %w", err)
	}

	logger.Debug("Task history table already exists")
	// Table exists, for now we don't do migration logic,
	// but we could add checks for schema changes in the future.
	return nil
}

func createTaskHistoryTable(db *sql.DB) error {
	logger := NewLogger()

	createTableSQL := `CREATE TABLE task_history (
id INTEGER PRIMARY KEY AUTOINCREMENT,
task_id INTEGER NOT NULL,
name STRING NOT NULL,
timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
change_type STRING NOT NULL,
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
	logger := NewLogger()

	// Check if table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='time_entries';")
	var name string
	err := row.Scan(&name)
	if err == sql.ErrNoRows {
		// Table does not exist, create it
		logger.Info("Time entries table does not exist, creating it")
		return createTimeEntriesTable(db)
	} else if err != nil {
		return fmt.Errorf("error checking if time_entries table exists: %w", err)
	}

	logger.Debug("Time entries table already exists")
	// Table exists, for now we don't do migration logic,
	// but we could add checks for schema changes in the future.
	return nil
}

func createTimeEntriesTable(db *sql.DB) error {
	logger := NewLogger()

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
