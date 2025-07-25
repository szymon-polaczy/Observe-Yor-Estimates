package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

func validateRequiredEnvVars() error {
	required := []string{
		"SLACK_WEBHOOK_URL",
		"TIMECAMP_API_KEY",
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

func CloseWithErrorLog(closer interface{ Close() error }, resourceName string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		appLogger.Errorf("Error closing %s: %v", resourceName, err)
	}
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  getEnvInt("TIMECAMP_API_MAX_RETRIES", 3),
		InitialWait: time.Duration(getEnvInt("TIMECAMP_API_INITIAL_WAIT_MS", 1000)) * time.Millisecond,
		MaxWait:     time.Duration(getEnvInt("TIMECAMP_API_MAX_WAIT_MS", 30000)) * time.Millisecond,
		Multiplier:  getEnvFloat("TIMECAMP_API_RETRY_MULTIPLIER", 2.0),
	}
}

func IsRetryableHTTPError(statusCode int) bool {
	retryableCodes := []int{500, 502, 503, 504, 429}
	for _, code := range retryableCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func DoHTTPWithRetry(client *http.Client, request *http.Request, config RetryConfig) (*http.Response, error) {
	logger := GetGlobalLogger()

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

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			if attempt > 0 {
				logger.Infof("HTTP request succeeded on attempt %d", attempt+1)
			}
			return response, nil
		}

		if IsRetryableHTTPError(response.StatusCode) {
			response.Body.Close()
			lastErr = fmt.Errorf("HTTP request returned retryable status %d", response.StatusCode)
			logger.Debugf("Request attempt %d returned retryable status %d", attempt+1, response.StatusCode)
			continue
		}

		if attempt > 0 {
			logger.Warnf("HTTP request failed with non-retryable status %d after %d attempts",
				response.StatusCode, attempt+1)
		}
		return response, nil
	}

	return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}
