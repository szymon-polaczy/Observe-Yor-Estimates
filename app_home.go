package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// AppHomeView represents the App Home tab view
type AppHomeView struct {
	Type   string  `json:"type"`
	Blocks []Block `json:"blocks"`
}

// HandleAppHome handles app home opened events
func HandleAppHome(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	var event SlackEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		logger.Errorf("Failed to decode app home event: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Handle URL verification
	if event.Type == "url_verification" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(event.Challenge))
		return
	}

	// Handle app home opened
	if event.Type == "event_callback" && event.Event.Type == "app_home_opened" {
		if err := PublishAppHomeView(event.Event.User); err != nil {
			logger.Errorf("Failed to publish app home view: %v", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// PublishAppHomeView publishes the app home view for a user
func PublishAppHomeView(userID string) error {
	slackClient := NewSlackAPIClient()

	// Get user's current project assignments
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	userProjects, err := GetUserProjects(db, userIDInt)
	if err != nil {
		return fmt.Errorf("failed to get user projects: %w", err)
	}

	allProjects, err := GetAllProjects(db)
	if err != nil {
		return fmt.Errorf("failed to get all projects: %w", err)
	}

	view := BuildAppHomeView(userProjects, allProjects, userIDInt)

	payload := map[string]interface{}{
		"user_id": userID,
		"view":    view,
	}

	return slackClient.sendSlackAPIRequest("views.publish", payload)
}

// BuildAppHomeView builds the App Home view structure
func BuildAppHomeView(userProjects []Project, allProjects []Project, userID int) AppHomeView {
	// Create a map for quick lookup of assigned projects
	assignedProjects := make(map[int]bool)
	for _, project := range userProjects {
		assignedProjects[project.ID] = true
	}

	var blocks []Block

	// Header
	blocks = append(blocks, Block{
		Type: "header",
		Text: &Text{
			Type: "plain_text",
			Text: "🏠 OYE Time Tracker Settings",
		},
	})

	// Current assignments section
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*📋 Your Project Assignments*\nSelect which projects you want to see in automatic updates:",
		},
	})

	// Project checkboxes
	var projectOptions []interface{}
	for _, project := range allProjects {
		isAssigned := assignedProjects[project.ID]

		option := map[string]interface{}{
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("%s %s",
					func() string {
						if isAssigned {
							return "☑️"
						} else {
							return "☐"
						}
					}(),
					project.Name),
			},
			"value": strconv.Itoa(project.ID),
		}

		projectOptions = append(projectOptions, option)
	}

	// Add project selection as checkboxes
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: " ",
		},
		Accessory: &Accessory{
			Type:     "checkboxes",
			ActionID: "project_assignments",
			Options:  projectOptions,
			InitialOptions: func() []interface{} {
				var selected []interface{}
				for _, project := range userProjects {
					selected = append(selected, map[string]interface{}{
						"text": map[string]string{
							"type": "mrkdwn",
							"text": fmt.Sprintf("☑️ %s", project.Name),
						},
						"value": strconv.Itoa(project.ID),
					})
				}
				return selected
			}(),
		},
	})

	// Update preferences
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*📊 Update Preferences*\nHow should automatic updates work for you?",
		},
	})

	// Radio buttons for update preference
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "When you receive automatic updates:",
		},
		Accessory: &Accessory{
			Type:     "radio_buttons",
			ActionID: "update_preference",
			Options: []interface{}{
				map[string]interface{}{
					"text":  map[string]string{"type": "plain_text", "text": "Show all projects"},
					"value": "all_projects",
				},
				map[string]interface{}{
					"text":  map[string]string{"type": "plain_text", "text": "Show only assigned projects"},
					"value": "assigned_only",
				},
			},
			InitialOption: map[string]interface{}{
				"text":  map[string]string{"type": "plain_text", "text": "Show all projects"},
				"value": "all_projects",
			},
		},
	})

	// Save button
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "actions",
		Elements: []Element{
			{
				Type:     "button",
				ActionID: "save_settings",
				Text:     map[string]string{"type": "plain_text", "text": "💾 Save Settings"},
				Style:    "primary",
			},
			{
				Type:     "button",
				ActionID: "reset_settings",
				Text:     map[string]string{"type": "plain_text", "text": "🔄 Reset to Defaults"},
			},
		},
	})

	return AppHomeView{
		Type:   "home",
		Blocks: blocks,
	}
}

// SlackEvent represents events from Slack
type SlackEvent struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Event     struct {
		Type string `json:"type"`
		User string `json:"user"`
	} `json:"event"`
}

// HandleInteractiveComponents handles button clicks and form interactions
func HandleInteractiveComponents(w http.ResponseWriter, r *http.Request) {
	logger := GetGlobalLogger()

	var payload SlackInteractivePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Errorf("Failed to decode interactive payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	switch payload.Type {
	case "block_actions":
		HandleBlockActions(payload)
	case "view_submission":
		HandleViewSubmission(payload)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleBlockActions(payload SlackInteractivePayload) {
	for _, action := range payload.Actions {
		switch action.ActionID {
		case "project_assignments":
			HandleProjectAssignmentChange(payload.User.ID, action.SelectedOptions)
		case "save_settings":
			HandleSaveSettings(payload.User.ID)
		case "reset_settings":
			HandleResetSettings(payload.User.ID)
		}
	}
}

func HandleProjectAssignmentChange(userID string, selectedOptions []SelectedOption) {
	logger := GetGlobalLogger()

	db, err := GetDB()
	if err != nil {
		logger.Errorf("Failed to get database connection: %v", err)
		return
	}

	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		logger.Errorf("Invalid user ID: %v", err)
		return
	}

	// Clear existing assignments
	_, err = db.Exec("DELETE FROM user_project_assignments WHERE user_id = $1", userIDInt)
	if err != nil {
		logger.Errorf("Failed to clear existing assignments: %v", err)
		return
	}

	// Add new assignments
	for _, option := range selectedOptions {
		projectID, err := strconv.Atoi(option.Value)
		if err != nil {
			logger.Errorf("Invalid project ID: %v", err)
			continue
		}

		err = AssignUserToProject(db, userIDInt, projectID)
		if err != nil {
			logger.Errorf("Failed to assign project %d to user %d: %v", projectID, userIDInt, err)
		}
	}

	// Refresh the App Home view
	PublishAppHomeView(userID)
}

type SlackInteractivePayload struct {
	Type string `json:"type"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Actions []struct {
		ActionID        string           `json:"action_id"`
		SelectedOptions []SelectedOption `json:"selected_options"`
	} `json:"actions"`
}

type SelectedOption struct {
	Value string `json:"value"`
}
