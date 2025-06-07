package main

import (
	"fmt"
	"os"
)

// validateRequiredEnvVars checks that all required environment variables are set
func validateRequiredEnvVars() error {
	required := []string{"SLACK_TOKEN", "SLACK_WEBHOOK_URL", "TIMECAMP_API_KEY"}
	var missing []string

	for _, env := range required {
		if os.Getenv(env) == "" {
			missing = append(missing, env)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}

	return nil
}

// CloseWithErrorLog safely closes a resource and logs any error
// This is the recommended pattern for non-critical close operations
func CloseWithErrorLog(closer interface{ Close() error }, resourceName string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		appLogger.Errorf("Error closing %s: %v", resourceName, err)
	}
}

// CloseWithErrorReturn safely closes a resource and returns any error
// Use this pattern when close errors need to be handled by the caller
func CloseWithErrorReturn(closer interface{ Close() error }) error {
	if closer == nil {
		return nil
	}
	return closer.Close()
}
