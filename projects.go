package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Project represents a project in the database
type Project struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	TimeCampTaskID int       `json:"timecamp_task_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

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

// SyncProjectsFromTasks updates the projects table from the current task hierarchy
func SyncProjectsFromTasks(db *sql.DB) error {
	logger := GetGlobalLogger()
	logger.Info("Syncing projects table from current task hierarchy")
	
	// Get all project-level tasks (level 2 in TimeCamp hierarchy)
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
	
	// Collect current project tasks
	type projectTask struct {
		TaskID int
		Name   string
	}
	var currentProjects []projectTask
	
	for rows.Next() {
		var pt projectTask
		if err := rows.Scan(&pt.TaskID, &pt.Name); err != nil {
			logger.Warnf("Failed to scan project task: %v", err)
			continue
		}
		currentProjects = append(currentProjects, pt)
	}
	
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating project task rows: %w", err)
	}
	
	// Upsert projects
	updateCount := 0
	insertCount := 0
	
	for _, pt := range currentProjects {
		// Handle potential duplicate names by making the name unique with task_id suffix if needed
		finalName := pt.Name
		
		// First try with original name
		upsertQuery := `
			INSERT INTO projects (name, timecamp_task_id, updated_at) 
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (timecamp_task_id) 
			DO UPDATE SET 
				name = EXCLUDED.name,
				updated_at = CURRENT_TIMESTAMP
			RETURNING (xmax = 0) AS inserted
		`
		
		var inserted bool
		err := db.QueryRow(upsertQuery, finalName, pt.TaskID).Scan(&inserted)
		
		// If we get a name conflict, try with a unique suffix
		if err != nil && strings.Contains(err.Error(), "duplicate key value violates unique constraint \"projects_name_key\"") {
			finalName = fmt.Sprintf("%s (Task ID: %d)", pt.Name, pt.TaskID)
			err = db.QueryRow(upsertQuery, finalName, pt.TaskID).Scan(&inserted)
		}
		
		if err != nil {
			logger.Errorf("Failed to upsert project %s (task_id: %d): %v", pt.Name, pt.TaskID, err)
			continue
		}
		
		if inserted {
			insertCount++
			if finalName != pt.Name {
				logger.Infof("Created project with unique name: %s", finalName)
			}
		} else {
			updateCount++
		}
	}
	
	logger.Infof("Project sync completed: %d inserted, %d updated", insertCount, updateCount)
	
	// Now update project_id in tasks table for efficient lookups
	logger.Info("Updating project_id mapping in tasks table")
	if err := updateTaskProjectMapping(db); err != nil {
		logger.Warnf("Failed to update task project mapping: %v", err)
		// Don't fail the whole sync for this
	}
	
	return nil
}

// ParseProjectFromCommand extracts project name from command text
// Supports both quoted and unquoted project names
func ParseProjectFromCommand(commandText string) (projectName string, remainingText string) {
	commandText = strings.TrimSpace(commandText)
	
	// Check for quoted project name
	if strings.HasPrefix(commandText, "\"") {
		endQuote := strings.Index(commandText[1:], "\"")
		if endQuote != -1 {
			projectName = commandText[1 : endQuote+1]
			remainingText = strings.TrimSpace(commandText[endQuote+2:])
			return projectName, remainingText
		}
	}
	
	// Check for single quoted project name
	if strings.HasPrefix(commandText, "'") {
		endQuote := strings.Index(commandText[1:], "'")
		if endQuote != -1 {
			projectName = commandText[1 : endQuote+1]
			remainingText = strings.TrimSpace(commandText[endQuote+2:])
			return projectName, remainingText
		}
	}
	
	// Check for special keywords
	parts := strings.Fields(commandText)
	if len(parts) > 0 {
		firstWord := strings.ToLower(parts[0])
		if firstWord == "all" {
			return "all", strings.TrimSpace(strings.Join(parts[1:], " "))
		}
		
		// Try to find where the project name ends (before "over", "daily", "weekly", "monthly" or time keywords)
		endKeywords := []string{"over", "daily", "weekly", "monthly", "this", "last", "today", "yesterday", "week", "month", "day", "days"}
		projectParts := []string{}
		
		for i, part := range parts {
			lowerPart := strings.ToLower(part)
			isEndKeyword := false
			for _, keyword := range endKeywords {
				if lowerPart == keyword {
					isEndKeyword = true
					break
				}
			}
			
			if isEndKeyword {
				remainingText = strings.TrimSpace(strings.Join(parts[i:], " "))
				break
			}
			
			projectParts = append(projectParts, part)
		}
		
		if len(projectParts) > 0 {
			projectName = strings.Join(projectParts, " ")
			if remainingText == "" {
				// No end keyword found, assume whole text is project name
				remainingText = ""
			}
		}
	}
	
	return projectName, remainingText
}

// updateTaskProjectMapping updates the project_id column in tasks table for efficient project filtering
func updateTaskProjectMapping(db *sql.DB) error {
	logger := GetGlobalLogger()
	
	// First, clear all existing project_id mappings
	_, err := db.Exec("UPDATE tasks SET project_id = NULL")
	if err != nil {
		return fmt.Errorf("failed to clear existing project mappings: %w", err)
	}
	
	// For each project, find all its descendant tasks and update their project_id
	projects, err := GetAllProjects(db)
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}
	
	for _, project := range projects {
		// Get all task IDs that belong to this project (including the project task itself and all descendants)
		taskIDs, err := GetProjectTaskIDs(db, project.TimeCampTaskID)
		if err != nil {
			logger.Warnf("Failed to get task IDs for project %s: %v", project.Name, err)
			continue
		}
		
		if len(taskIDs) == 0 {
			continue
		}
		
		// Update all these tasks to have this project_id
		updateQuery := `UPDATE tasks SET project_id = $1 WHERE task_id = ANY($2)`
		_, err = db.Exec(updateQuery, project.ID, pq.Array(taskIDs))
		if err != nil {
			logger.Warnf("Failed to update project mapping for %s: %v", project.Name, err)
			continue
		}
		
	}
	
	logger.Info("Task project mapping updated successfully")
	return nil
}