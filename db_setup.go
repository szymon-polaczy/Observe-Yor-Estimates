package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	globalDB *sql.DB
	dbMutex  sync.RWMutex
	dbOnce   sync.Once
	initErr  error
)

func getDBConnectionString() string {
	logger := GetGlobalLogger()

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		logger.Debug("Using DATABASE_URL environment variable")
		return dbURL
	}

	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	if host == "" || user == "" || password == "" || dbname == "" {
		logger.Error("Database configuration missing! Set DATABASE_URL or DB_HOST, DB_USER, DB_PASSWORD, DB_NAME")
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

func validateDatabaseWriteAccess() error {
	logger := GetGlobalLogger()

	db, err := GetDB()
	if err != nil {
		if strings.Contains(err.Error(), "network is unreachable") && strings.Contains(err.Error(), "dial tcp [") {
			return fmt.Errorf("IPv6 connectivity issue detected - check network configuration: %w", err)
		}
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS write_test (id SERIAL PRIMARY KEY)`); err != nil {
		return fmt.Errorf("database write test failed: %w", err)
	}

	if _, err := db.Exec(`DROP TABLE IF EXISTS write_test`); err != nil {
		logger.Warnf("Failed to clean up write test table: %v", err)
	}

	logger.Debug("Database write access validated")
	return nil
}

func GetDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		logger := GetGlobalLogger()
		connStr := getDBConnectionString()
		if connStr == "" {
			initErr = fmt.Errorf("database connection string not configured")
			return
		}

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			initErr = fmt.Errorf("failed to open database connection: %w", err)
			return
		}

		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxIdleTime(90 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			db.Close()
			if strings.Contains(err.Error(), "network is unreachable") && strings.Contains(err.Error(), "dial tcp [") {
				initErr = fmt.Errorf("IPv6 connectivity issue detected: %w", err)
			} else {
				initErr = fmt.Errorf("failed to ping database: %w", err)
			}
			return
		}

		if err := createAllTables(db); err != nil {
			db.Close()
			initErr = fmt.Errorf("failed to create tables: %w", err)
			return
		}

		logger.Info("Database connection established and tables created")

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

func createAllTables(db *sql.DB) error {
	tables := []struct {
		name   string
		create func(*sql.DB) error
	}{
		{"tasks", createTasksTable},
		{"task_history", createTaskHistoryTable},
		{"time_entries", createTimeEntriesTable},
		{"users", createUsersTable},
		{"projects", createProjectsTable},
		{"user_project_assignments", createUserProjectAssignmentsTable},
		{"threshold_notifications", createThresholdNotificationsTable},
		{"slack_users", createSlackUsersTable},
	}

	for _, table := range tables {
		if err := table.create(db); err != nil {
			return fmt.Errorf("failed to create %s table: %w", table.name, err)
		}
	}

	return populateProjectsFromTasks(db)
}

func createTasksTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS tasks (
		task_id INTEGER PRIMARY KEY,
		parent_id INTEGER NOT NULL,
		assigned_by INTEGER NOT NULL,
		name TEXT NOT NULL,
		level INTEGER NOT NULL,
		root_group_id INTEGER NOT NULL,
		archived INTEGER DEFAULT 0,
		project_id INTEGER REFERENCES projects(id)
	)`

	_, err := db.Exec(query)
	return err
}

func createTaskHistoryTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS task_history (
		id SERIAL PRIMARY KEY,
		task_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		change_type TEXT NOT NULL,
		previous_value TEXT,
		current_value TEXT,
		FOREIGN KEY (task_id) REFERENCES tasks(task_id)
	)`

	_, err := db.Exec(query)
	return err
}

func createTimeEntriesTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS time_entries (
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
	)`

	_, err := db.Exec(query)
	return err
}

func createUsersTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS users (
		user_id INTEGER PRIMARY KEY,
		username TEXT NOT NULL,
		display_name TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err := db.Exec(query)
	return err
}

func createProjectsTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS projects (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		timecamp_task_id INTEGER NOT NULL UNIQUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (timecamp_task_id) REFERENCES tasks(task_id)
	)`

	_, err := db.Exec(query)
	return err
}

func createUserProjectAssignmentsTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS user_project_assignments (
		id SERIAL PRIMARY KEY,
		slack_user_id TEXT NOT NULL,
		project_id INTEGER NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (project_id) REFERENCES projects(id),
		UNIQUE(slack_user_id, project_id)
	)`

	_, err := db.Exec(query)
	return err
}

func createThresholdNotificationsTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS threshold_notifications (
		id SERIAL PRIMARY KEY,
		task_id INTEGER NOT NULL,
		threshold_percentage INTEGER NOT NULL,
		current_percentage DECIMAL(5,2) NOT NULL,
		notified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_time_entry_date TEXT NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(task_id),
		UNIQUE(task_id, threshold_percentage)
	)`

	_, err := db.Exec(query)
	return err
}

type project struct {
	taskID int
	name   string
}

func populateProjectsFromTasks(db *sql.DB) error {
	logger := GetGlobalLogger()

	// Check if projects table already has data to avoid unnecessary work
	var projectCount int
	countQuery := `SELECT COUNT(*) FROM projects`
	if err := db.QueryRow(countQuery).Scan(&projectCount); err != nil {
		return fmt.Errorf("failed to count existing projects: %w", err)
	}

	// If projects already exist, skip population
	if projectCount > 0 {
		logger.Debugf("Projects table already has %d entries, skipping population", projectCount)
		return nil
	}

	// Get all project-level tasks in one query
	query := `
		SELECT DISTINCT task_id, name
		FROM tasks
		WHERE level = 2
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query project-level tasks: %w", err)
	}
	defer rows.Close()

	// Collect all projects first for batch insert
	var projects []project

	for rows.Next() {
		var p project
		if err := rows.Scan(&p.taskID, &p.name); err != nil {
			logger.Warnf("Failed to scan project row: %v", err)
			continue
		}
		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating project rows: %w", err)
	}

	if len(projects) == 0 {
		logger.Debug("No projects found to populate")
		return nil
	}

	// Use batch insert for better performance
	insertedCount := 0
	batchSize := 50 // Process in batches to avoid too large transactions

	for i := 0; i < len(projects); i += batchSize {
		end := i + batchSize
		if end > len(projects) {
			end = len(projects)
		}

		batch := projects[i:end]
		if err := insertProjectBatch(db, batch); err != nil {
			logger.Warnf("Failed to insert project batch %d-%d: %v", i, end-1, err)
			continue
		}
		insertedCount += len(batch)
	}

	logger.Infof("Successfully populated %d projects from task hierarchy", insertedCount)
	return nil
}

func insertProjectBatch(db *sql.DB, projects []project) error {
	if len(projects) == 0 {
		return nil
	}

	// Build multi-row insert query
	valueStrings := make([]string, 0, len(projects))
	valueArgs := make([]interface{}, 0, len(projects)*2)
	
	for i, p := range projects {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2))
		valueArgs = append(valueArgs, p.name, p.taskID)
	}

	query := fmt.Sprintf(`
		INSERT INTO projects (name, timecamp_task_id) 
		VALUES %s 
		ON CONFLICT (timecamp_task_id) DO NOTHING`,
		strings.Join(valueStrings, ","))

	_, err := db.Exec(query, valueArgs...)
	return err
}

func createSlackUsersTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS slack_users (
		slack_user_id TEXT PRIMARY KEY,
		real_name TEXT,
		display_name TEXT,
		email TEXT,
		is_bot BOOLEAN DEFAULT FALSE,
		deleted BOOLEAN DEFAULT FALSE,
		last_sync TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err := db.Exec(query)
	return err
}
