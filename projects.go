package main

import (
	"database/sql"
	"fmt"
)

// GetAllProjects returns all projects from the database
func GetAllProjects(db *sql.DB) ([]Project, error) {
	query := `
		SELECT id, name, timecamp_task_id, created_at, updated_at 
		FROM projects 
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		err := rows.Scan(&project.ID, &project.Name, &project.TimeCampTaskID,
			&project.CreatedAt, &project.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating project rows: %w", err)
	}

	return projects, nil
}

// GetProjectByName returns a project by exact name match
func GetProjectByName(db *sql.DB, name string) (*Project, error) {
	query := `
		SELECT id, name, timecamp_task_id, created_at, updated_at 
		FROM projects 
		WHERE name = $1
	`

	var project Project
	err := db.QueryRow(query, name).Scan(&project.ID, &project.Name,
		&project.TimeCampTaskID, &project.CreatedAt, &project.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query project by name: %w", err)
	}

	return &project, nil
}

// FindProjectsByName returns projects that match the given name (fuzzy matching)
func FindProjectsByName(db *sql.DB, name string) ([]Project, error) {
	// Try exact match first
	if project, err := GetProjectByName(db, name); err != nil {
		return nil, err
	} else if project != nil {
		return []Project{*project}, nil
	}

	// Try case-insensitive match
	query := `
		SELECT id, name, timecamp_task_id, created_at, updated_at 
		FROM projects 
		WHERE LOWER(name) = LOWER($1)
	`

	rows, err := db.Query(query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects by name (case-insensitive): %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		err := rows.Scan(&project.ID, &project.Name, &project.TimeCampTaskID,
			&project.CreatedAt, &project.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}
		projects = append(projects, project)
	}

	if len(projects) > 0 {
		return projects, nil
	}

	// Try partial match (contains)
	query = `
		SELECT id, name, timecamp_task_id, created_at, updated_at 
		FROM projects 
		WHERE LOWER(name) LIKE LOWER('%' || $1 || '%')
		ORDER BY 
			CASE 
				WHEN LOWER(name) LIKE LOWER($1 || '%') THEN 1  -- starts with
				WHEN LOWER(name) LIKE LOWER('%' || $1) THEN 2  -- ends with
				ELSE 3                                          -- contains
			END,
			LENGTH(name)  -- prefer shorter matches
	`

	rows, err = db.Query(query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects by name (partial): %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var project Project
		err := rows.Scan(&project.ID, &project.Name, &project.TimeCampTaskID,
			&project.CreatedAt, &project.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating project rows: %w", err)
	}

	return projects, nil
}

// GetProjectTaskIDs returns all task IDs that belong to a project (including sub-tasks)
func GetProjectTaskIDs(db *sql.DB, projectTaskID int) ([]int, error) {
	// Get all tasks that are descendants of the project task
	query := `
		WITH RECURSIVE task_hierarchy AS (
			-- Start with the project task itself
			SELECT task_id, parent_id, name, 0 as depth
			FROM tasks
			WHERE task_id = $1
			
			UNION ALL
			
			-- Recursively get all child tasks
			SELECT t.task_id, t.parent_id, t.name, th.depth + 1
			FROM tasks t
			JOIN task_hierarchy th ON t.parent_id = th.task_id
			WHERE th.depth < 10  -- Prevent infinite recursion
		)
		SELECT task_id FROM task_hierarchy
		WHERE task_id != $1 OR $1 IN (SELECT task_id FROM tasks WHERE parent_id != 0)
	`

	rows, err := db.Query(query, projectTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query project task IDs: %w", err)
	}
	defer rows.Close()

	var taskIDs []int
	for rows.Next() {
		var taskID int
		if err := rows.Scan(&taskID); err != nil {
			return nil, fmt.Errorf("failed to scan task ID: %w", err)
		}
		taskIDs = append(taskIDs, taskID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating task ID rows: %w", err)
	}

	return taskIDs, nil
}
