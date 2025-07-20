package main

import (
	"database/sql"
	"fmt"
)

type UserProjectAssignment struct {
	ID          int    `json:"id"`
	SlackUserID string `json:"slack_user_id"`
	ProjectID   int    `json:"project_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// GetUserProjects returns all projects assigned to a user
func GetUserProjects(db *sql.DB, slackUserID string) ([]Project, error) {
	query := `
		SELECT p.id, p.name, p.timecamp_task_id, p.created_at, p.updated_at
		FROM projects p
		INNER JOIN user_project_assignments upa ON p.id = upa.project_id
		WHERE upa.slack_user_id = $1
		ORDER BY p.name
	`

	rows, err := db.Query(query, slackUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		err := rows.Scan(&project.ID, &project.Name, &project.TimeCampTaskID,
			&project.CreatedAt, &project.UpdatedAt)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	return projects, nil
}

// AssignUserToProject assigns a user to a project
func AssignUserToProject(db *sql.DB, slackUserID string, projectID int) error {
	query := `
		INSERT INTO user_project_assignments (slack_user_id, project_id, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (slack_user_id, project_id) DO UPDATE SET
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, slackUserID, projectID)
	return err
}

// UnassignUserFromProject removes a user from a project
func UnassignUserFromProject(db *sql.DB, slackUserID string, projectID int) error {
	query := `DELETE FROM user_project_assignments WHERE slack_user_id = $1 AND project_id = $2`
	result, err := db.Exec(query, slackUserID, projectID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user is not assigned to this project")
	}

	return nil
}
