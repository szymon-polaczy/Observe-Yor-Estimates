package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// validateRequiredEnvVars checks that all required environment variables are set
func validateRequiredEnvVars() error {
	// For Netlify builds, skip validation since API keys are set at runtime
	if isNetlifyBuild() {
		return nil
	}

	required := []string{
		"SLACK_WEBHOOK_URL",
		"TIMECAMP_API_KEY",
		// Optional but recommended environment variables
		// "DATABASE_PATH", "SLACK_API_URL", "TIMECAMP_API_URL" - these have defaults
		// "SLACK_VERIFICATION_TOKEN" - optional for security
	}
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

// isNetlifyBuild checks if we're running in a Netlify build environment
func isNetlifyBuild() bool {
	return os.Getenv("NETLIFY") != "" || os.Getenv("DEPLOY_URL") != "" || os.Getenv("NETLIFY_BUILD_BASE") != ""
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

// RetryConfig defines configuration for HTTP retry attempts
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  getEnvInt("TIMECAMP_API_MAX_RETRIES", 3),
		InitialWait: time.Duration(getEnvInt("TIMECAMP_API_INITIAL_WAIT_MS", 1000)) * time.Millisecond,
		MaxWait:     time.Duration(getEnvInt("TIMECAMP_API_MAX_WAIT_MS", 30000)) * time.Millisecond,
		Multiplier:  getEnvFloat("TIMECAMP_API_RETRY_MULTIPLIER", 2.0),
	}
}

// getEnvInt gets an integer from environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvFloat gets a float from environment variable with a default value
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// IsRetryableHTTPError determines if an HTTP error should be retried
func IsRetryableHTTPError(statusCode int) bool {
	switch statusCode {
	case http.StatusInternalServerError, // 500
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout,     // 504
		http.StatusTooManyRequests:    // 429
		return true
	default:
		return false
	}
}

// DoHTTPWithRetry executes an HTTP request with retry logic for temporary failures
func DoHTTPWithRetry(client *http.Client, request *http.Request, config RetryConfig) (*http.Response, error) {
	logger := NewLogger()

	if client == nil {
		client = http.DefaultClient
	}

	var lastErr error
	waitTime := config.InitialWait

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			logger.Warnf("Retrying HTTP request (attempt %d/%d) after %v: %s",
				attempt, config.MaxRetries, waitTime, request.URL.String())
			time.Sleep(waitTime)

			// Exponential backoff with jitter
			waitTime = time.Duration(float64(waitTime) * config.Multiplier)
			if waitTime > config.MaxWait {
				waitTime = config.MaxWait
			}
		}

		response, err := client.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			logger.Debugf("Request attempt %d failed: %v", attempt+1, err)
			continue
		}

		// Check if we should retry based on status code
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			// Success
			if attempt > 0 {
				logger.Infof("HTTP request succeeded on attempt %d", attempt+1)
			}
			return response, nil
		}

		// For retryable errors, close the response body and try again
		if IsRetryableHTTPError(response.StatusCode) {
			response.Body.Close()
			lastErr = fmt.Errorf("HTTP request returned retryable status %d", response.StatusCode)
			logger.Debugf("Request attempt %d returned retryable status %d", attempt+1, response.StatusCode)
			continue
		}

		// For non-retryable errors, return the response as-is for the caller to handle
		if attempt > 0 {
			logger.Warnf("HTTP request failed with non-retryable status %d after %d attempts",
				response.StatusCode, attempt+1)
		}
		return response, nil
	}

	// All retries exhausted
	return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}
