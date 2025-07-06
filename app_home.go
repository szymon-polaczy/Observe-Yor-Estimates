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

	view := BuildSimpleAppHomeView(userProjects, allProjects, userIDInt)

	payload := map[string]interface{}{
		"user_id": userID,
		"view":    view,
	}

	return slackClient.sendSlackAPIRequest("views.publish", payload)
}

// BuildSimpleAppHomeView builds a simplified App Home view without complex interactive components
func BuildSimpleAppHomeView(userProjects []Project, allProjects []Project, userID int) AppHomeView {
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
				Text: "• _No projects assigned yet_\n• You will see all projects in automatic updates\n• Use `/oye assign \"Project Name\"` to assign yourself to specific projects",
			},
		})
	} else {
		assignmentText := ""
		for i, project := range userProjects {
			if i > 0 {
				assignmentText += "\n"
			}
			assignmentText += fmt.Sprintf("• ☑️ %s", project.Name)
		}
		assignmentText += "\n\n_Use `/oye unassign \"Project Name\"` to remove assignments_"

		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{
				Type: "mrkdwn",
				Text: assignmentText,
			},
		})
	}

	// Available projects section
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*📁 Available Projects*",
		},
	})

	// Create a map for quick lookup of assigned projects
	assignedProjects := make(map[int]bool)
	for _, project := range userProjects {
		assignedProjects[project.ID] = true
	}

	// Show available projects
	projectText := ""
	for i, project := range allProjects {
		if i > 0 {
			projectText += "\n"
		}
		if assignedProjects[project.ID] {
			projectText += fmt.Sprintf("• ☑️ %s _(assigned)_", project.Name)
		} else {
			projectText += fmt.Sprintf("• ☐ %s", project.Name)
		}
	}

	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: projectText,
		},
	})

	// Instructions section
	blocks = append(blocks, Block{Type: "divider"})
	blocks = append(blocks, Block{
		Type: "section",
		Text: &Text{
			Type: "mrkdwn",
			Text: "*💡 How to Manage Your Projects*\n\n• **Assign yourself:** `/oye assign \"Project Name\"`\n• **Remove assignment:** `/oye unassign \"Project Name\"`\n• **View your projects:** `/oye my-projects`\n• **View all projects:** `/oye available-projects`\n\n_When you have project assignments, automatic updates will only show your assigned projects. If you have no assignments, you'll see all projects._",
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
