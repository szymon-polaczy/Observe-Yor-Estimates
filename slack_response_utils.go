package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// sendDelayedResponseShared sends a delayed response to Slack using the response URL
// This function is accessible from both CLI and server modes
func sendDelayedResponseShared(responseURL string, message SlackMessage) error {
	logger := GetGlobalLogger()

	if responseURL == "" {
		logger.Debug("No response URL provided for delayed response")
		return nil
	}

	// Convert SlackMessage to a format compatible with Slack response URLs
	response := map[string]interface{}{
		"response_type": "in_channel", // Visible to everyone in the channel
		"text":          message.Text,
	}

	// Add blocks if they exist
	if len(message.Blocks) > 0 {
		response["blocks"] = message.Blocks
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	logger.Debugf("Sending delayed response to URL: %s", responseURL)

	resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending delayed response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Errorf("Slack response URL returned status %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("slack API returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Debug("Successfully sent delayed response to Slack")
	return nil
}
