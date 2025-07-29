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

	logger.Infof("App Home payload size: %d characters", len(payloadJSON))

	// Debug: log the first part of the payload to see structure
	payloadStr := string(payloadJSON)
	if len(payloadStr) > 1000 {
		logger.Infof("App Home payload preview: %s...", payloadStr[:1000])
	} else {
		logger.Infof("App Home payload: %s", payloadStr)
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
	return PublishAppHomeViewWithSearch(userID, page, "")
}

// PublishAppHomeViewWithSearch publishes the app home view for a user with a specific page and search query
func PublishAppHomeViewWithSearch(userID string, page int, searchQuery string) error {
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

	view := BuildSimpleAppHomeViewWithSearch(userProjects, allProjects, userID, page, searchQuery)

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
	return BuildSimpleAppHomeViewWithSearch(userProjects, allProjects, userID, page, "")
}

// BuildSimpleAppHomeViewWithSearch builds App Home view with search functionality
func BuildSimpleAppHomeViewWithSearch(userProjects []Project, allProjects []Project, userID string, page int, searchQuery string) AppHomeView {
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

	// Show current search state if there's an active search
	if searchQuery != "" {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*🔍 Search Results for: \"%s\"*", searchQuery),
			},
		})
	}

	// Add search section with input and button
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*🔍 Search Projects*",
		},
	})

	// Search input and clear buttons - use proper input block
	blocks = append(blocks, Block{
		Type:    "input",
		BlockID: "search_input_block",
		Label: &Text{
			Type: "plain_text",
			Text: "🔍 Search Projects",
		},
		Element: InputElement{
			Type:         "plain_text_input",
			ActionID:     "project_search_input",
			Placeholder: map[string]string{
				"type": "plain_text",
				"text": "Enter project name...",
			},
			InitialValue: searchQuery,
		},
	})
	// Buttons in separate actions block
	var buttonElements []interface{}
	buttonElements = append(buttonElements, ButtonElement{
		Type:     "button",
		Text:     &Text{Type: "plain_text", Text: "🔍 Search"},
		ActionID: "search_submit",
		Value:    "search",
		Style:    "primary",
	})

	if searchQuery != "" {
		buttonElements = append(buttonElements, ButtonElement{
			Type:     "button",
			Text:     &Text{Type: "plain_text", Text: "❌ Clear"},
			ActionID: "clear_search",
			Value:    "clear",
		})
	}

	blocks = append(blocks, Block{
		Type:     "actions",
		Elements: buttonElements,
	})

	// Add clear search button if there's an active search (moved this logic above)
	// Apply search filter to projects
	filteredProjects := filterProjectsBySearch(allProjects, searchQuery)

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
	if endIdx > len(filteredProjects) {
		endIdx = len(filteredProjects)
	}

	totalPages := (len(filteredProjects) + projectsPerPage - 1) / projectsPerPage

	// Header with pagination info and search results
	headerText := fmt.Sprintf("*Projects* (Page %d of %d)", currentPage+1, totalPages)
	if searchQuery != "" {
		headerText += fmt.Sprintf(" - Found %d projects matching \"%s\"", len(filteredProjects), searchQuery)
	}
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: headerText,
		},
	})

	// Show current page of projects
	if startIdx < len(filteredProjects) {
		currentPageProjects := filteredProjects[startIdx:endIdx]

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
				ActionID:       fmt.Sprintf("project_assignments_page_%d_search_%s", currentPage, searchQuery),
				Options:        checkboxOptions,
				InitialOptions: initialOptions,
			},
		})

		// Show pagination info
		blocks = append(blocks, Block{
			Type: "context",
			Elements: []interface{}{
				Element{
					Type: "mrkdwn",
					Text: fmt.Sprintf("_Showing projects %d-%d of %d total_", startIdx+1, endIdx, len(filteredProjects)),
				},
			},
		})

		// Pagination navigation (only show if we have multiple pages)
		if totalPages > 1 {
			var navElements []interface{}

			// Previous button
			if currentPage > 0 {
				navElements = append(navElements, ButtonElement{
					Type:     "button",
					Text:     &Text{Type: "plain_text", Text: "⬅️ Previous"},
					ActionID: fmt.Sprintf("page_%d_search_%s", currentPage-1, searchQuery),
					Value:    fmt.Sprintf("%d|%s", currentPage-1, searchQuery),
				})
			}

			// Page indicator
			navElements = append(navElements, ButtonElement{
				Type:     "button",
				Text:     &Text{Type: "plain_text", Text: fmt.Sprintf("Page %d/%d", currentPage+1, totalPages)},
				ActionID: "page_info",
				Value:    "info",
			})

			// Next button
			if currentPage < totalPages-1 {
				navElements = append(navElements, ButtonElement{
					Type:     "button",
					Text:     &Text{Type: "plain_text", Text: "Next ➡️"},
					ActionID: fmt.Sprintf("page_%d_search_%s", currentPage+1, searchQuery),
					Value:    fmt.Sprintf("%d|%s", currentPage+1, searchQuery),
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
		Elements: []interface{}{
			Element{
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
		logger.Infof("Full payload structure: %+v", genericPayload)
		if actions, ok := genericPayload["actions"].([]interface{}); ok && len(actions) > 0 {
			if action, ok := actions[0].(map[string]interface{}); ok {
				logger.Infof("Action ID: %v", action["action_id"])
				logger.Infof("Action type: %v", action["type"])
				logger.Infof("Action value: %v", action["value"])
			}
		}
		// Also check for state/view information that might contain input values
		if state, ok := genericPayload["state"].(map[string]interface{}); ok {
			logger.Infof("State found: %+v", state)
		}
		if view, ok := genericPayload["view"].(map[string]interface{}); ok {
			logger.Infof("View found: %+v", view)
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

	// Extract search value from state if available
	searchValue := extractSearchValueFromState(payload.State)
	if searchValue != "" {
		logger.Infof("Found search value in state: '%s'", searchValue)
	} else {
		logger.Info("No search value found in state")
	}

	// Log all actions for debugging
	for i, action := range payload.Actions {
		logger.Infof("Action %d: ID='%s', Type='%s', Value='%s'", i, action.ActionID, action.Type, action.Value)
	}

	// Handle checkbox actions
	if len(payload.Actions) > 0 {
		action := payload.Actions[0]
		logger.Infof("Action ID: %s, Selected options: %d", action.ActionID, len(action.SelectedOptions))

		// Use search value from state for all actions (this ensures search is preserved)
		currentSearchQuery := searchValue

		if strings.HasPrefix(action.ActionID, "project_assignments") {
			logger.Info("Processing project assignment checkboxes...")

			// Extract page number from action_id (search query comes from state now)
			pageNum := 0
			if strings.Contains(action.ActionID, "page_") {
				parts := strings.Split(action.ActionID, "page_")
				if len(parts) > 1 {
					pageSearchParts := strings.Split(parts[1], "_search_")
					if num, err := strconv.Atoi(pageSearchParts[0]); err == nil {
						pageNum = num
					}
				}
			}

			if err := HandleProjectAssignmentCheckboxes(payload.User.ID, action.SelectedOptions, pageNum); err != nil {
				logger.Errorf("Failed to handle project assignment checkboxes: %v", err)
				http.Error(w, "Failed to process assignments", http.StatusInternalServerError)
				return
			}

			// Refresh the App Home view with the same page and current search
			logger.Info("Refreshing App Home view...")
			if err := PublishAppHomeViewWithSearch(payload.User.ID, pageNum, currentSearchQuery); err != nil {
				logger.Errorf("Failed to refresh app home view: %v", err)
			} else {
				logger.Info("App Home view refreshed successfully")
			}
		} else if strings.HasPrefix(action.ActionID, "page_") {
			logger.Info("Processing page navigation...")
			if err := HandlePageNavigationWithSearch(payload.User.ID, action.ActionID, action.Value); err != nil {
				logger.Errorf("Failed to handle page navigation: %v", err)
				http.Error(w, "Failed to process page navigation", http.StatusInternalServerError)
				return
			}
		} else if action.ActionID == "project_search_input" {
			logger.Info("Processing project search input action...")
			// For input actions, refresh with the value from state
			if err := PublishAppHomeViewWithSearch(payload.User.ID, 0, currentSearchQuery); err != nil {
				logger.Errorf("Failed to update app home with search: %v", err)
			}
		} else if action.ActionID == "search_submit" {
			logger.Info("Processing search submit button click...")
			logger.Infof("Search submit - current search value from state: '%s'", currentSearchQuery)
			// Use the search value from state when search button is clicked
			if err := PublishAppHomeViewWithSearch(payload.User.ID, 0, currentSearchQuery); err != nil {
				logger.Errorf("Failed to submit search: %v", err)
			} else {
				logger.Infof("Successfully processed search for query: '%s'", currentSearchQuery)
			}
		} else if action.ActionID == "clear_search" {
			logger.Info("Processing clear search...")
			if err := PublishAppHomeViewWithSearch(payload.User.ID, 0, ""); err != nil {
				logger.Errorf("Failed to clear search: %v", err)
			}
		} else {
			logger.Warnf("Unknown action ID: %s", action.ActionID)
			// For any unknown action, still try to preserve search state
			logger.Info("Refreshing with preserved search state...")
			if err := PublishAppHomeViewWithSearch(payload.User.ID, 0, currentSearchQuery); err != nil {
				logger.Errorf("Failed to refresh with search state: %v", err)
			}
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
	State struct {
		Values map[string]map[string]struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"values"`
	} `json:"state,omitempty"`
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
	return HandlePageNavigationWithSearch(userID, actionID, value)
}

// HandlePageNavigationWithSearch processes page navigation with search support
func HandlePageNavigationWithSearch(userID, actionID, value string) error {
	logger := GetGlobalLogger()

	// Skip the page_info button (just shows current page)
	if actionID == "page_info" {
		return nil
	}

	// Extract page number and search query from value (format: "pageNum|searchQuery")
	parts := strings.Split(value, "|")
	pageNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid page number: %s", parts[0])
	}

	searchQuery := ""
	if len(parts) > 1 {
		searchQuery = parts[1]
	}

	// Refresh the App Home view with the new page and search
	logger.Infof("Navigating to page %d with search '%s' for user %s", pageNum, searchQuery, userID)
	if err := PublishAppHomeViewWithSearch(userID, pageNum, searchQuery); err != nil {
		return fmt.Errorf("failed to refresh app home view with page %d and search '%s': %w", pageNum, searchQuery, err)
	}

	logger.Infof("Successfully navigated to page %d with search '%s' for user %s", pageNum, searchQuery, userID)
	return nil
}

// extractSearchValueFromState extracts the search input value from the Slack state object
func extractSearchValueFromState(state struct {
	Values map[string]map[string]struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"values"`
}) string {
	logger := GetGlobalLogger()
	logger.Infof("=== DEBUGGING STATE EXTRACTION ===")
	logger.Infof("Total blocks in state: %d", len(state.Values))
	
	// Debug: Print the entire state structure
	for blockID, block := range state.Values {
		logger.Infof("BLOCK ID: '%s' (contains %d elements)", blockID, len(block))
		for actionID, element := range block {
			logger.Infof("  ACTION ID: '%s' | TYPE: '%s' | VALUE: '%s'", actionID, element.Type, element.Value)
		}
	}

	// Look for the search input in the state values
	for blockID, block := range state.Values {
		for actionID, element := range block {
			// Primary check: look for our specific action_id
			if actionID == "project_search_input" {
				logger.Infof("✅ FOUND project_search_input with value: '%s'", element.Value)
				return strings.TrimSpace(element.Value)
			}
		}
		
		// Fallback: check if this is our specific search block
		if blockID == "search_input_block" {
			logger.Infof("✅ FOUND search_input_block, checking all elements...")
			for actionID, element := range block {
				logger.Infof("  Checking element in search block: actionID='%s', type='%s', value='%s'", actionID, element.Type, element.Value)
				if element.Type == "plain_text_input" {
					logger.Infof("✅ FOUND plain_text_input in search block with value: '%s'", element.Value)
					return strings.TrimSpace(element.Value)
				}
			}
		}
	}

	logger.Warnf("❌ NO SEARCH INPUT FOUND - returning empty string")
	return ""
}

// filterProjectsBySearch filters projects based on a search query
func filterProjectsBySearch(projects []Project, query string) []Project {
	if query == "" {
		return projects
	}

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return projects
	}

	var filtered []Project
	for _, project := range projects {
		projectName := strings.ToLower(project.Name)
		if strings.Contains(projectName, query) {
			filtered = append(filtered, project)
		}
	}

	return filtered
}
