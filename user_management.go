package main

import (
	"fmt"
	"os"
)

// PopulateUsers adds sample users to the database for testing
func PopulateUsers() error {
	logger := GetGlobalLogger()
	
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Sample users that might be common in the time entries
	sampleUsers := []struct {
		UserID      int
		Username    string
		DisplayName string
	}{
		{1820471, "john.smith", "John Smith"},
		{1721068, "mary.johnson", "Mary Johnson"},
		{2490949, "alex.wilson", "Alex Wilson"},
		{1234567, "sarah.brown", "Sarah Brown"},
		{9876543, "mike.davis", "Mike Davis"},
	}

	logger.Info("Adding sample users to the database...")

	for _, user := range sampleUsers {
		err := UpsertUser(db, user.UserID, user.Username, user.DisplayName)
		if err != nil {
			logger.Warnf("Failed to add user %d (%s): %v", user.UserID, user.Username, err)
		} else {
			logger.Infof("Added/updated user: %d - %s (%s)", user.UserID, user.DisplayName, user.Username)
		}
	}

	logger.Info("User population completed")
	return nil
}

// ListUsers shows all users in the database
func ListUsers() error {
	logger := GetGlobalLogger()
	
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `SELECT user_id, username, display_name, created_at FROM users ORDER BY user_id`
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	fmt.Println("Current users in database:")
	fmt.Println("User ID  | Username      | Display Name  | Created At")
	fmt.Println("---------|---------------|---------------|-------------------")

	userCount := 0
	for rows.Next() {
		var userID int
		var username, displayName, createdAt string
		
		err := rows.Scan(&userID, &username, &displayName, &createdAt)
		if err != nil {
			logger.Warnf("Failed to scan user row: %v", err)
			continue
		}

		fmt.Printf("%-8d | %-13s | %-13s | %s\n", userID, username, displayName, createdAt[:19])
		userCount++
	}

	if userCount == 0 {
		fmt.Println("No users found in database")
	} else {
		fmt.Printf("\nTotal: %d users\n", userCount)
	}

	return nil
}

// AddUser adds a single user to the database
func AddUser(userID int, username, displayName string) error {
	logger := GetGlobalLogger()
	
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	err = UpsertUser(db, userID, username, displayName)
	if err != nil {
		return fmt.Errorf("failed to add user: %w", err)
	}

	logger.Infof("Successfully added/updated user: %d - %s (%s)", userID, displayName, username)
	return nil
}

// GetActiveUserIDs returns a list of user IDs that have time entries
func GetActiveUserIDs() ([]int, error) {
	db, err := GetDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `SELECT DISTINCT user_id FROM time_entries ORDER BY user_id`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active user IDs: %w", err)
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		err := rows.Scan(&userID)
		if err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	return userIDs, nil
}

func init() {
	// Add command line support for user management
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "populate-users":
			if err := PopulateUsers(); err != nil {
				fmt.Printf("Error populating users: %v\n", err)
				os.Exit(1)
			}
		case "list-users":
			if err := ListUsers(); err != nil {
				fmt.Printf("Error listing users: %v\n", err)
				os.Exit(1)
			}
		case "active-users":
			userIDs, err := GetActiveUserIDs()
			if err != nil {
				fmt.Printf("Error getting active user IDs: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Active user IDs from time entries:")
			for _, userID := range userIDs {
				fmt.Printf("- %d\n", userID)
			}
		case "add-user":
			if len(os.Args) < 5 {
				fmt.Println("Usage: ./oye-time-tracker add-user <user_id> <username> <display_name>")
				os.Exit(1)
			}
			var userID int
			fmt.Sscanf(os.Args[2], "%d", &userID)
			username := os.Args[3]
			displayName := os.Args[4]
			if err := AddUser(userID, username, displayName); err != nil {
				fmt.Printf("Error adding user: %v\n", err)
				os.Exit(1)
			}
		}
	}
}