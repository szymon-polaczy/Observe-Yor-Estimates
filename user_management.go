package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// JsonUser represents a user from TimeCamp API
type JsonUser struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// getTimeCampUsers fetches users from TimeCamp API
func getTimeCampUsers() ([]JsonUser, error) {
	logger := GetGlobalLogger()

	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		logger.Warnf("Could not reload .env file (continuing with existing env vars): %v", err)
	}

	// Get TimeCamp API URL from environment variable or use default
	timecampAPIURL := os.Getenv("TIMECAMP_API_URL")
	if timecampAPIURL == "" {
		timecampAPIURL = "https://app.timecamp.com/third_party/api"
	}
	getAllUsersURL := timecampAPIURL + "/users"

	// Validate API key exists
	apiKey := os.Getenv("TIMECAMP_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TIMECAMP_API_KEY environment variable not set")
	}

	authBearer := "Bearer " + apiKey

	request, err := http.NewRequest("GET", getAllUsersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Add("Authorization", authBearer)
	request.Header.Add("Accept", "application/json")

	logger.Debugf("Fetching users from TimeCamp API: %s", request.URL.String())

	// Use optimized HTTP client for better performance
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Use retry mechanism for API calls
	retryConfig := DefaultRetryConfig()
	response, err := DoHTTPWithRetry(client, request, retryConfig)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to TimeCamp API failed after retries: %w", err)
	}
	defer CloseWithErrorLog(response.Body, "HTTP response body")

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("TimeCamp API returned status %d: %s", response.StatusCode, string(body))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// TimeCamp API returns users as an array
	var users []JsonUser
	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	logger.Infof("Successfully fetched %d users from TimeCamp API", len(users))
	return users, nil
}

// SyncUsersFromTimeCamp fetches users from TimeCamp API and stores them in the database
func SyncUsersFromTimeCamp() error {
	logger := GetGlobalLogger()

	timecampUsers, err := getTimeCampUsers()
	if err != nil {
		return fmt.Errorf("failed to fetch users from TimeCamp: %w", err)
	}

	if len(timecampUsers) == 0 {
		logger.Info("No users received from TimeCamp API")
		return nil
	}

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	logger.Infof("Syncing %d users from TimeCamp...", len(timecampUsers))

	successCount := 0
	errorCount := 0

	for _, user := range timecampUsers {
		// Convert string UserID to integer
		userID, err := strconv.Atoi(user.UserID)
		if err != nil {
			logger.Warnf("Failed to convert user ID '%s' to integer: %v", user.UserID, err)
			errorCount++
			continue
		}

		err = UpsertUser(db, userID, user.Email, user.DisplayName)
		if err != nil {
			logger.Warnf("Failed to add user %d (%s): %v", userID, user.Email, err)
			errorCount++
		} else {
			logger.Debugf("Added/updated user: %d - %s (%s)", userID, user.DisplayName, user.Email)
			successCount++
		}
	}

	logger.Infof("User sync completed: %d users synced successfully, %d errors", successCount, errorCount)
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
