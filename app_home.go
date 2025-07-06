package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// AppHomeView represents the App Home tab view
type AppHomeView struct {
	Type   string  `json:"type"`
	Blocks []Block `json:"blocks"`
}

// Extended structures for App Home (separate from existing simple ones)
type AppHomeBlock struct {
	Type      string            `json:"type"`
	Text      *Text             `json:"text,omitempty"`
	Elements  []AppHomeElement  `json:"elements,omitempty"`
	Accessory *AppHomeAccessory `json:"accessory,omitempty"`
}

type AppHomeElement struct {
	Type     string            `json:"type"`
	Text     map[string]string `json:"text,omitempty"`
	ActionID string            `json:"action_id,omitempty"`
	Style    string            `json:"style,omitempty"`
}

type AppHomeAccessory struct {
	Type           string                   `json:"type"`
	ActionID       string                   `json:"action_id,omitempty"`
	Options        []map[string]interface{} `json:"options,omitempty"`
	InitialOptions []map[string]interface{} `json:"initial_options,omitempty"`
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
	logger := GetGlobalLogger()
	slackClient := NewSlackAPIClient()

	// Get user's current project assignments
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	userProjects, err := GetUserProjects(db, userID)
	if err != nil {
		return fmt.Errorf("failed to get user projects: %w", err)
	}

	allProjects, err := GetAllProjects(db)
	if err != nil {
		return fmt.Errorf("failed to get all projects: %w", err)
	}

	view := BuildSimpleAppHomeView(userProjects, allProjects, userID)

	payload := map[string]interface{}{
		"user_id": userID,
		"view":    view,
	}

	// Validate payload size before sending
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal app home payload: %w", err)
	}

	const maxAppHomeSize = 3000 // Slack's character limit for App Home
	if len(payloadJSON) > maxAppHomeSize {
		logger.Errorf("App Home payload too large: %d > %d characters", len(payloadJSON), maxAppHomeSize)
		// Return a simplified view instead of failing
		simpleView := BuildFallbackAppHomeView(len(userProjects), len(allProjects))
		payload["view"] = simpleView
	}

	return slackClient.sendSlackAPIRequest("views.publish", payload)
}

// BuildSimpleAppHomeView builds a simplified App Home view without complex interactive components
func BuildSimpleAppHomeView(userProjects []Project, allProjects []Project, userID string) AppHomeView {
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
			Text: "*📋 Your Current Project Assignments*",
		},
	})

	// Show current assignments
	if len(userProjects) == 0 {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: "• _No projects assigned yet_\n• Select projects below to assign yourself",
			},
		})
	} else {
		assignmentText := fmt.Sprintf("*%d projects assigned:*\n", len(userProjects))
		const maxToShow = 8
		for i, project := range userProjects {
			if i >= maxToShow {
				remaining := len(userProjects) - maxToShow
				assignmentText += fmt.Sprintf("• _...and %d more_", remaining)
				break
			}
			assignmentText += fmt.Sprintf("• %s\n", project.Name)
		}

		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: assignmentText,
			},
		})
	}

	// Interactive project assignment section with checkboxes
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*🔧 Quick Project Assignment*\nCheck/uncheck projects to assign yourself:",
		},
	})

	// Create a map of assigned projects for quick lookup
	assignedProjectMap := make(map[int]bool)
	for _, up := range userProjects {
		assignedProjectMap[up.ID] = true
	}

	// Create individual toggle buttons for ALL projects
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*🔧 Toggle Project Assignments*\nClick any project to assign/unassign yourself:",
		},
	})

	// Split projects into manageable chunks for display
	const projectsPerChunk = 25
	for i := 0; i < len(allProjects); i += projectsPerChunk {
		end := i + projectsPerChunk
		if end > len(allProjects) {
			end = len(allProjects)
		}

		chunk := allProjects[i:end]

		// Create buttons for this chunk
		var elements []Element
		for _, project := range chunk {
			isAssigned := assignedProjectMap[project.ID]
			status := "➕"
			style := "primary"
			if isAssigned {
				status = "✅"
				style = "danger" // Red button to indicate "click to remove"
			}

			buttonText := fmt.Sprintf("%s %s", status, project.Name)
			if len(buttonText) > 75 { // Slack button text limit
				buttonText = fmt.Sprintf("%s %s...", status, project.Name[:70])
			}

			elements = append(elements, Element{
				Type:     "button",
				Text:     buttonText,
				ActionID: fmt.Sprintf("toggle_project_%d", project.ID),
				Style:    style,
				Value:    fmt.Sprintf("%d", project.ID),
			})
		}

		// Add action block with buttons (max 5 buttons per actions block)
		for j := 0; j < len(elements); j += 5 {
			endIdx := j + 5
			if endIdx > len(elements) {
				endIdx = len(elements)
			}

			blocks = append(blocks, Block{
				Type:     "actions",
				Elements: elements[j:endIdx],
			})
		}

		// Add some spacing between large chunks
		if end < len(allProjects) && len(chunk) == projectsPerChunk {
			blocks = append(blocks, Block{
				Type: "context",
				Elements: []Element{
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("_Showing projects %d-%d of %d_", i+1, end, len(allProjects)),
					},
				},
			})
		}
	}

	// Project management section
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*📁 Project Management*\n\n• **Total projects available:** %d\n• **Your assignments:** %d\n\n*Quick Commands:*\n• `/oye available-projects` - View all projects\n• `/oye assign \"Project Name\"` - Assign yourself\n• `/oye unassign \"Project Name\"` - Remove assignment", len(allProjects), len(userProjects)),
		},
	})

	// Instructions section
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*💡 How Project Filtering Works*\n\n• **With assignments:** Automatic updates show only your assigned projects\n• **Without assignments:** Automatic updates show all projects\n\n*Note: Use the commands above to manage your project assignments efficiently.*",
		},
	})

	// Footer
	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Element{
			{
				Type: "mrkdwn",
				Text: "🔄 This page updates automatically when you make changes",
			},
		},
	})

	return AppHomeView{
		Type:   "home",
		Blocks: blocks,
	}
}

// BuildFallbackAppHomeView builds a minimal App Home view when the regular view is too large
func BuildFallbackAppHomeView(userProjectCount, totalProjectCount int) AppHomeView {
	var blocks []Block

	// Minimal header
	blocks = append(blocks, Block{
		Type: "header",
		Text: &Text{
			Type: "plain_text",
			Text: "🏠 OYE Time Tracker",
		},
	})

	// Simple summary
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Project Summary*\n• Your assignments: %d\n• Total projects: %d", userProjectCount, totalProjectCount),
		},
	})

	// Essential commands
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*Commands*\n• `/oye my-projects` - View your assignments\n• `/oye available-projects` - View all projects\n• `/oye assign \"Project Name\"` - Assign project\n• `/oye unassign \"Project Name\"` - Remove assignment",
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
	logger.Info("=== Interactive component request received ===")

	// Parse the interactive payload
	if err := r.ParseForm(); err != nil {
		logger.Errorf("Failed to parse interactive form: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	payloadStr := r.FormValue("payload")
	if payloadStr == "" {
		logger.Error("No payload in interactive request")
		http.Error(w, "No payload", http.StatusBadRequest)
		return
	}

	logger.Infof("Raw payload: %s", payloadStr)

	// Try to unmarshal as a generic map first to see the structure
	var genericPayload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &genericPayload); err != nil {
		logger.Errorf("Failed to unmarshal generic payload: %v", err)
	} else {
		logger.Infof("Payload type: %v", genericPayload["type"])
		if actions, ok := genericPayload["actions"].([]interface{}); ok && len(actions) > 0 {
			if action, ok := actions[0].(map[string]interface{}); ok {
				logger.Infof("Action ID: %v", action["action_id"])
				logger.Infof("Action type: %v", action["type"])
			}
		}
	}

	var payload SlackInteractivePayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		logger.Errorf("Failed to unmarshal interactive payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	logger.Infof("Interactive component request from user %s, payload type: %s", payload.User.ID, payload.Type)
	logger.Infof("Number of actions: %d", len(payload.Actions))

	// Handle checkbox actions
	if len(payload.Actions) > 0 {
		action := payload.Actions[0]
		logger.Infof("Action ID: %s, Selected options: %d", action.ActionID, len(action.SelectedOptions))

		if strings.HasPrefix(action.ActionID, "project_assignments") {
			logger.Info("Processing project assignment checkboxes...")
			if err := HandleProjectAssignmentCheckboxes(payload.User.ID, action.SelectedOptions); err != nil {
				logger.Errorf("Failed to handle project assignment checkboxes: %v", err)
				http.Error(w, "Failed to process assignments", http.StatusInternalServerError)
				return
			}

			// Refresh the App Home view
			logger.Info("Refreshing App Home view...")
			if err := PublishAppHomeView(payload.User.ID); err != nil {
				logger.Errorf("Failed to refresh app home view: %v", err)
			} else {
				logger.Info("App Home view refreshed successfully")
			}
		} else if strings.HasPrefix(action.ActionID, "toggle_project_") {
			logger.Info("Processing project toggle button...")
			if err := HandleProjectToggle(payload.User.ID, action.ActionID, action.Value); err != nil {
				logger.Errorf("Failed to handle project toggle: %v", err)
				http.Error(w, "Failed to process project toggle", http.StatusInternalServerError)
				return
			}

			// Refresh the App Home view
			logger.Info("Refreshing App Home view...")
			if err := PublishAppHomeView(payload.User.ID); err != nil {
				logger.Errorf("Failed to refresh app home view: %v", err)
			} else {
				logger.Info("App Home view refreshed successfully")
			}
		} else {
			logger.Warnf("Unknown action ID: %s", action.ActionID)
		}
	} else {
		logger.Warn("No actions in payload")
	}

	w.WriteHeader(http.StatusOK)
}

// SlackInteractivePayload represents interactive component payloads
type SlackInteractivePayload struct {
	Type string `json:"type"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	} `json:"user"`
	Actions []struct {
		ActionID        string           `json:"action_id"`
		Type            string           `json:"type,omitempty"`
		SelectedOptions []SelectedOption `json:"selected_options,omitempty"`
		Value           string           `json:"value,omitempty"`
	} `json:"actions"`
	View struct {
		Type string `json:"type,omitempty"`
	} `json:"view,omitempty"`
}

type SelectedOption struct {
	Value string `json:"value"`
}

// HandleProjectAssignmentCheckboxes processes checkbox selections for project assignments
func HandleProjectAssignmentCheckboxes(userID string, selectedOptions []SelectedOption) error {
	logger := GetGlobalLogger()

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Get current user assignments
	currentProjects, err := GetUserProjects(db, userID)
	if err != nil {
		return fmt.Errorf("failed to get current user projects: %w", err)
	}

	// Convert current assignments to a map for easy lookup
	currentAssignments := make(map[int]bool)
	for _, project := range currentProjects {
		currentAssignments[project.ID] = true
	}

	// Convert selected options to project IDs
	selectedProjectIDs := make(map[int]bool)
	for _, option := range selectedOptions {
		projectID, err := strconv.Atoi(option.Value)
		if err != nil {
			logger.Warnf("Invalid project ID in checkbox selection: %s", option.Value)
			continue
		}
		selectedProjectIDs[projectID] = true
	}

	// Determine what to add and what to remove
	var toAdd []int
	var toRemove []int

	// Find projects to add (selected but not currently assigned)
	for projectID := range selectedProjectIDs {
		if !currentAssignments[projectID] {
			toAdd = append(toAdd, projectID)
		}
	}

	// Find projects to remove (currently assigned but not selected)
	for projectID := range currentAssignments {
		if !selectedProjectIDs[projectID] {
			toRemove = append(toRemove, projectID)
		}
	}

	// Add new assignments
	for _, projectID := range toAdd {
		if err := AssignUserToProject(db, userID, projectID); err != nil {
			logger.Errorf("Failed to assign user %s to project %d: %v", userID, projectID, err)
			// Continue with other assignments even if one fails
		} else {
			logger.Infof("Assigned user %s to project %d", userID, projectID)
		}
	}

	// Remove old assignments
	for _, projectID := range toRemove {
		if err := UnassignUserFromProject(db, userID, projectID); err != nil {
			logger.Errorf("Failed to unassign user %s from project %d: %v", userID, projectID, err)
			// Continue with other unassignments even if one fails
		} else {
			logger.Infof("Unassigned user %s from project %d", userID, projectID)
		}
	}

	logger.Infof("Project assignment update completed for user %s: %d added, %d removed", userID, len(toAdd), len(toRemove))
	return nil
}

// HandleProjectToggle processes individual project toggle button clicks
func HandleProjectToggle(userID, actionID, value string) error {
	logger := GetGlobalLogger()

	projectID, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid project ID: %s", value)
	}

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Check if user is currently assigned to this project
	userProjects, err := GetUserProjects(db, userID)
	if err != nil {
		return fmt.Errorf("failed to get user projects: %w", err)
	}

	isCurrentlyAssigned := false
	for _, project := range userProjects {
		if project.ID == projectID {
			isCurrentlyAssigned = true
			break
		}
	}

	// Toggle the assignment
	if isCurrentlyAssigned {
		// Remove assignment
		if err := UnassignUserFromProject(db, userID, projectID); err != nil {
			return fmt.Errorf("failed to unassign user from project %d: %w", projectID, err)
		}
		logger.Infof("Unassigned user %s from project %d via toggle", userID, projectID)
	} else {
		// Add assignment
		if err := AssignUserToProject(db, userID, projectID); err != nil {
			return fmt.Errorf("failed to assign user to project %d: %w", projectID, err)
		}
		logger.Infof("Assigned user %s to project %d via toggle", userID, projectID)
	}

	return nil
}
