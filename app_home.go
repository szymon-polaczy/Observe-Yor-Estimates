package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	Type          string                   `json:"type"`
	ActionID      string                   `json:"action_id,omitempty"`
	Options       []map[string]interface{} `json:"options,omitempty"`
	InitialOption map[string]interface{}   `json:"initial_option,omitempty"`
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

	// Show current assignments (limited to prevent overflow)
	if len(userProjects) == 0 {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: "• _No projects assigned yet_\n• You will see all projects in automatic updates\n• Use `/oye assign \"Project Name\"` to assign yourself to specific projects",
			},
		})
	} else {
		const maxProjectsToShow = 10
		assignmentText := ""
		projectsToShow := userProjects

		if len(userProjects) > maxProjectsToShow {
			projectsToShow = userProjects[:maxProjectsToShow]
		}

		for i, project := range projectsToShow {
			if i > 0 {
				assignmentText += "\n"
			}
			assignmentText += fmt.Sprintf("• ☑️ %s", project.Name)
		}

		if len(userProjects) > maxProjectsToShow {
			remaining := len(userProjects) - maxProjectsToShow
			assignmentText += fmt.Sprintf("\n• _...and %d more projects_", remaining)
		}

		assignmentText += "\n\n_Use `/oye my-projects` to see all assignments or `/oye unassign \"Project Name\"` to remove assignments_"

		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: assignmentText,
			},
		})
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

	// For now, just acknowledge the request since we're using a simplified approach
	logger.Info("Interactive component request received")
	w.WriteHeader(http.StatusOK)
}

// Simple struct for interactive payloads (not used in simplified version)
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
