package main

import (
	"os"
	"strconv"
)

// GetTaskStatus determines status based on percentage with configurable thresholds
func GetTaskStatus(percentage float64) StatusInfo {
	// Get configurable thresholds from environment or use defaults
	midPoint := getThresholdFromEnv("MID_POINT", DEFAULT_MID_POINT)
	highPoint := getThresholdFromEnv("HIGH_POINT", DEFAULT_HIGH_POINT)

	switch {
	case percentage == 0:
		return StatusInfo{
			Emoji:       EMOJI_NO_TIME,
			Status:      STATUS_NO_TIME,
			Description: STATUS_NO_TIME,
		}
	case percentage > 0 && percentage <= midPoint:
		return StatusInfo{
			Emoji:       EMOJI_ON_TRACK,
			Status:      STATUS_ON_TRACK,
			Description: STATUS_ON_TRACK,
		}
	case percentage > midPoint && percentage <= highPoint:
		return StatusInfo{
			Emoji:       EMOJI_HIGH_USAGE,
			Status:      STATUS_HIGH_USAGE,
			Description: STATUS_HIGH_USAGE,
		}
	case percentage > highPoint && percentage < THRESHOLD_OVER:
		return StatusInfo{
			Emoji:       EMOJI_CRITICAL,
			Status:      STATUS_CRITICAL,
			Description: STATUS_CRITICAL,
		}
	case percentage >= THRESHOLD_OVER:
		return StatusInfo{
			Emoji:       EMOJI_OVER_BUDGET,
			Status:      STATUS_OVER_BUDGET,
			Description: STATUS_OVER_BUDGET,
		}
	default:
		return StatusInfo{
			Emoji:       EMOJI_NO_TIME,
			Status:      STATUS_UNKNOWN,
			Description: STATUS_UNKNOWN,
		}
	}
}

// GetThresholdStatus returns status info for threshold reporting
func GetThresholdStatus(threshold float64) StatusInfo {
	switch {
	case threshold >= THRESHOLD_OVER:
		return StatusInfo{
			Emoji:       EMOJI_CRITICAL,
			Status:      STATUS_OVER_BUDGET,
			Description: DESC_OVER_BUDGET,
		}
	case threshold >= THRESHOLD_CRITICAL:
		return StatusInfo{
			Emoji:       EMOJI_OVER_BUDGET,
			Status:      STATUS_CRITICAL,
			Description: DESC_CRITICAL_USAGE,
		}
	case threshold >= THRESHOLD_HIGH:
		return StatusInfo{
			Emoji:       EMOJI_HIGH_USAGE,
			Status:      STATUS_HIGH_USAGE,
			Description: DESC_HIGH_USAGE,
		}
	case threshold >= THRESHOLD_WARNING:
		return StatusInfo{
			Emoji:       EMOJI_WARNING,
			Status:      STATUS_WARNING,
			Description: DESC_WARNING_LEVEL,
		}
	default:
		return StatusInfo{
			Emoji:       EMOJI_CHART,
			Status:      STATUS_UNKNOWN,
			Description: DESC_USAGE_REPORT,
		}
	}
}

// GetSuggestionForThreshold returns an appropriate suggestion for a threshold level
func GetSuggestionForThreshold(threshold float64) string {
	// Find the closest threshold
	for _, level := range []float64{THRESHOLD_OVER, THRESHOLD_CRITICAL, THRESHOLD_HIGH, THRESHOLD_WARNING} {
		if threshold >= level {
			if suggestion, exists := ThresholdSuggestions[level]; exists {
				return suggestion
			}
		}
	}
	return "📈 Regular monitoring helps maintain project visibility and accurate estimations."
}

// IsOverThreshold checks if percentage exceeds a threshold
func IsOverThreshold(percentage, threshold float64) bool {
	return percentage >= threshold
}

// GetWorstStatus returns the worst status from a list of percentages
func GetWorstStatus(percentages []float64) StatusInfo {
	worstPercentage := 0.0
	for _, p := range percentages {
		if p > worstPercentage {
			worstPercentage = p
		}
	}
	return GetTaskStatus(worstPercentage)
}

// getThresholdFromEnv gets threshold from environment with fallback to default
func getThresholdFromEnv(envVar string, defaultValue float64) float64 {
	if envValue := os.Getenv(envVar); envValue != "" {
		if parsed, err := strconv.ParseFloat(envValue, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// GetStatusEmoji is a convenience function for just getting the emoji
func GetStatusEmoji(percentage float64) string {
	return GetTaskStatus(percentage).Emoji
}

// GetStatusDescription is a convenience function for just getting the description
func GetStatusDescription(percentage float64) string {
	return GetTaskStatus(percentage).Description
}