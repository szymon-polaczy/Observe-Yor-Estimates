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

	view := BuildSimpleAppHomeView(userProjects, allProjects, userID, 0)

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

// PublishAppHomeViewWithPage publishes the app home view for a user with a specific page
func PublishAppHomeViewWithPage(userID string, page int) error {
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

	view := BuildSimpleAppHomeView(userProjects, allProjects, userID, page)

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
func BuildSimpleAppHomeView(userProjects []Project, allProjects []Project, userID string, page int) AppHomeView {
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
			Text: "*🔧 Project Assignment*\nUse checkboxes to select which projects you want to be assigned to:",
		},
	})

	// Create a map of assigned projects for quick lookup
	assignedProjectMap := make(map[int]bool)
	for _, up := range userProjects {
		assignedProjectMap[up.ID] = true
	}

	// Show projects with pagination to stay within payload limits
	const projectsPerPage = 10
	currentPage := page // Use provided page parameter

	startIdx := currentPage * projectsPerPage
	endIdx := startIdx + projectsPerPage
	if endIdx > len(allProjects) {
		endIdx = len(allProjects)
	}

	totalPages := (len(allProjects) + projectsPerPage - 1) / projectsPerPage

	// Header with pagination info
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Projects* (Page %d of %d)", currentPage+1, totalPages),
		},
	})

	// Show current page of projects
	if startIdx < len(allProjects) {
		currentPageProjects := allProjects[startIdx:endIdx]

		// Create checkbox options for current page
		var checkboxOptions []map[string]interface{}
		for _, project := range currentPageProjects {
			option := map[string]interface{}{
				"text": map[string]string{
					"type": "plain_text",
					"text": project.Name,
				},
				"value": fmt.Sprintf("%d", project.ID),
			}

			checkboxOptions = append(checkboxOptions, option)
		}

		// Create initial options (pre-selected checkboxes for assigned projects)
		var initialOptions []map[string]interface{}
		for _, project := range currentPageProjects {
			if assignedProjectMap[project.ID] {
				initialOption := map[string]interface{}{
					"text": map[string]string{
						"type": "plain_text",
						"text": project.Name,
					},
					"value": fmt.Sprintf("%d", project.ID),
				}
				initialOptions = append(initialOptions, initialOption)
			}
		}

		// Add checkbox group for project assignments
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: "Select projects to assign yourself to:",
			},
			Accessory: &Accessory{
				Type:           "checkboxes",
				ActionID:       fmt.Sprintf("project_assignments_page_%d", currentPage),
				Options:        checkboxOptions,
				InitialOptions: initialOptions,
			},
		})

		// Show pagination info
		blocks = append(blocks, Block{
			Type: "context",
			Elements: []Element{
				{
					Type: "mrkdwn",
					Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("_Showing projects %d-%d of %d total_", startIdx+1, endIdx, len(allProjects))},
				},
			},
		})

		// Pagination navigation (only show if we have multiple pages)
		if totalPages > 1 {
			var navElements []Element

			// Previous button
			if currentPage > 0 {
				navElements = append(navElements, Element{
					Type:     "button",
					Text:     &Text{Type: "plain_text", Text: "⬅️ Previous"},
					ActionID: fmt.Sprintf("page_%d", currentPage-1),
					Value:    fmt.Sprintf("%d", currentPage-1),
				})
			}

			// Page indicator
			navElements = append(navElements, Element{
				Type:     "button",
				Text:     &Text{Type: "plain_text", Text: fmt.Sprintf("Page %d/%d", currentPage+1, totalPages)},
				ActionID: "page_info",
				Value:    "info",
			})

			// Next button
			if currentPage < totalPages-1 {
				navElements = append(navElements, Element{
					Type:     "button",
					Text:     &Text{Type: "plain_text", Text: "Next ➡️"},
					ActionID: fmt.Sprintf("page_%d", currentPage+1),
					Value:    fmt.Sprintf("%d", currentPage+1),
				})
			}

			blocks = append(blocks, Block{
				Type:     "actions",
				Elements: navElements,
			})
		}
	}

	// Footer
	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Element{
			{
				Type: "mrkdwn",
				Text: &Text{Type: "mrkdwn", Text: "🔄 This page updates automatically when you make changes"},
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

			// Extract page number from action_id
			pageNum := 0
			if strings.Contains(action.ActionID, "page_") {
				parts := strings.Split(action.ActionID, "page_")
				if len(parts) > 1 {
					if num, err := strconv.Atoi(parts[1]); err == nil {
						pageNum = num
					}
				}
			}

			if err := HandleProjectAssignmentCheckboxes(payload.User.ID, action.SelectedOptions, pageNum); err != nil {
				logger.Errorf("Failed to handle project assignment checkboxes: %v", err)
				http.Error(w, "Failed to process assignments", http.StatusInternalServerError)
				return
			}

			// Refresh the App Home view with the same page
			logger.Info("Refreshing App Home view...")
			if err := PublishAppHomeViewWithPage(payload.User.ID, pageNum); err != nil {
				logger.Errorf("Failed to refresh app home view: %v", err)
			} else {
				logger.Info("App Home view refreshed successfully")
			}
		} else if strings.HasPrefix(action.ActionID, "page_") {
			logger.Info("Processing page navigation...")
			if err := HandlePageNavigation(payload.User.ID, action.ActionID, action.Value); err != nil {
				logger.Errorf("Failed to handle page navigation: %v", err)
				http.Error(w, "Failed to process page navigation", http.StatusInternalServerError)
				return
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
func HandleProjectAssignmentCheckboxes(userID string, selectedOptions []SelectedOption, pageNum int) error {
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

	// Get all projects to determine which are visible on current page
	allProjects, err := GetAllProjects(db)
	if err != nil {
		return fmt.Errorf("failed to get all projects: %w", err)
	}

	// Calculate which projects are visible on current page using same pagination logic
	const projectsPerPage = 10
	startIdx := pageNum * projectsPerPage
	endIdx := startIdx + projectsPerPage
	if endIdx > len(allProjects) {
		endIdx = len(allProjects)
	}

	if startIdx >= len(allProjects) {
		return fmt.Errorf("page %d is out of range", pageNum)
	}

	currentPageProjects := allProjects[startIdx:endIdx]

	// Create a map of projects visible on current page
	visibleProjectIDs := make(map[int]bool)
	for _, project := range currentPageProjects {
		visibleProjectIDs[project.ID] = true
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

	// Determine what to add and what to remove - ONLY for projects visible on current page
	var toAdd []int
	var toRemove []int

	// Find projects to add (selected but not currently assigned, and visible on current page)
	for projectID := range selectedProjectIDs {
		if visibleProjectIDs[projectID] && !currentAssignments[projectID] {
			toAdd = append(toAdd, projectID)
		}
	}

	// Find projects to remove (currently assigned, visible on current page, but not selected)
	for projectID := range visibleProjectIDs {
		if currentAssignments[projectID] && !selectedProjectIDs[projectID] {
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

	logger.Infof("Project assignment update completed for user %s on page %d: %d added, %d removed", userID, pageNum, len(toAdd), len(toRemove))
	return nil
}

// HandlePageNavigation processes page navigation button clicks
func HandlePageNavigation(userID, actionID, value string) error {
	logger := GetGlobalLogger()

	// Skip the page_info button (just shows current page)
	if actionID == "page_info" {
		return nil
	}

	// Extract page number from value
	pageNum, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid page number: %s", value)
	}

	// Refresh the App Home view with the new page
	logger.Infof("Navigating to page %d for user %s", pageNum, userID)
	if err := PublishAppHomeViewWithPage(userID, pageNum); err != nil {
		return fmt.Errorf("failed to refresh app home view with page %d: %w", pageNum, err)
	}

	logger.Infof("Successfully navigated to page %d for user %s", pageNum, userID)
	return nil
}
