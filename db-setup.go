package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const dbPath = "./oye.db"

// GetDB opens a connection to the SQLite database, creates or migrates tables as needed, and returns the DB handle.
func GetDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := migrateTasksTable(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// migrateTasksTable ensures the tasks table exists and matches the desired schema.
func migrateTasksTable(db *sql.DB) error {
	// Check if table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks';")
	var name string
	err := row.Scan(&name)
	if err == sql.ErrNoRows {
		// Table does not exist, create it
		return createTasksTable(db)
	} else if err != nil && err != sql.ErrNoRows {
		return err
	}

	// Table exists, for now we don't do migration logic,
	// but we could add checks for schema changes in the future.
	return nil
}

func createTasksTable(db *sql.DB) error {
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
	fmt.Println("tasks table created or migrated correctly")
	return nil
}
