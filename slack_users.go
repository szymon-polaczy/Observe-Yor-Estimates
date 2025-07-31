package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// SlackUsersListResponse represents the response from Slack's users.list API
type SlackUsersListResponse struct {
	OK      bool        `json:"ok"`
	Members []SlackUser `json:"members"`
	Error   string      `json:"error,omitempty"`
}

// SlackUserProfile represents the profile information from Slack API
type SlackUserProfile struct {
	RealName    string `json:"real_name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// SlackAPIUser represents the user object from Slack API
type SlackAPIUser struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Profile SlackUserProfile `json:"profile"`
	IsBot   bool             `json:"is_bot"`
	Deleted bool             `json:"deleted"`
}

// GetAllSlackUsers retrieves all users from the Slack workspace
func GetAllSlackUsers() ([]SlackUser, error) {
	logger := GetGlobalLogger()
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackBotToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN not configured")
	}

	logger.Info("Fetching all Slack workspace users")

	// Create HTTP request to Slack users.list API
	req, err := http.NewRequest("GET", "https://slack.com/api/users.list", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+slackBotToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Slack API returned status %d", resp.StatusCode)
	}

	var slackResp struct {
		OK      bool           `json:"ok"`
		Members []SlackAPIUser `json:"members"`
		Error   string         `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	if !slackResp.OK {
		return nil, fmt.Errorf("Slack API error: %s", slackResp.Error)
	}

	// Convert to our SlackUser type and filter active users
	var users []SlackUser
	for _, member := range slackResp.Members {
		// Skip bots and deleted users
		if member.IsBot || member.Deleted {
			continue
		}

		user := SlackUser{
			ID:          member.ID,
			Name:        member.Name,
			RealName:    member.Profile.RealName,
			DisplayName: member.Profile.DisplayName,
			Email:       member.Profile.Email,
			IsBot:       member.IsBot,
			Deleted:     member.Deleted,
		}
		users = append(users, user)
	}

	logger.Infof("Retrieved %d active users from Slack workspace", len(users))
	return users, nil
}

// SyncSlackUsersToDatabase syncs Slack workspace users to the database
func SyncSlackUsersToDatabase() error {
	logger := GetGlobalLogger()
	logger.Info("Starting Slack user sync to database")

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}

	users, err := GetAllSlackUsers()
	if err != nil {
		return fmt.Errorf("failed to get Slack users: %v", err)
	}

	// Start transaction for bulk insert/update
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %v", err)
	}
	defer tx.Rollback()

	// Prepare upsert statement (PostgreSQL syntax)
	stmt, err := tx.Prepare(`
		INSERT INTO slack_users (slack_user_id, real_name, display_name, email, is_bot, deleted, last_sync)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (slack_user_id) 
		DO UPDATE SET 
			real_name = EXCLUDED.real_name,
			display_name = EXCLUDED.display_name,
			email = EXCLUDED.email,
			is_bot = EXCLUDED.is_bot,
			deleted = EXCLUDED.deleted,
			last_sync = EXCLUDED.last_sync
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	syncTime := time.Now()
	updatedCount := 0

	for _, user := range users {
		_, err := stmt.Exec(
			user.ID,
			user.RealName,
			user.DisplayName,
			user.Email,
			user.IsBot,
			user.Deleted,
			syncTime,
		)
		if err != nil {
			logger.Errorf("Failed to sync user %s (%s): %v", user.ID, user.RealName, err)
			continue
		}
		updatedCount++
	}

	// Mark users as deleted if they're not in the current workspace
	// Get all current user IDs from our sync
	currentUserIDs := make([]string, len(users))
	for i, user := range users {
		currentUserIDs[i] = user.ID
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	logger.Infof("Successfully synced %d Slack users to database", updatedCount)
	return nil
}

// GetSlackUsersFromDatabase retrieves active Slack users from the database
func GetSlackUsersFromDatabase() ([]SlackUser, error) {
	logger := GetGlobalLogger()
	db, err := GetDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %v", err)
	}

	query := `
		SELECT slack_user_id, real_name, display_name, email, is_bot, deleted
		FROM slack_users 
		WHERE deleted = false AND is_bot = false
		ORDER BY real_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query slack users: %v", err)
	}
	defer rows.Close()

	var users []SlackUser
	for rows.Next() {
		var user SlackUser
		var displayName, email sql.NullString

		err := rows.Scan(
			&user.ID,
			&user.RealName,
			&displayName,
			&email,
			&user.IsBot,
			&user.Deleted,
		)
		if err != nil {
			logger.Errorf("Failed to scan user row: %v", err)
			continue
		}

		if displayName.Valid {
			user.DisplayName = displayName.String
		}
		if email.Valid {
			user.Email = email.String
		}

		users = append(users, user)
	}

	logger.Infof("Retrieved %d active users from database", len(users))
	return users, nil
}
