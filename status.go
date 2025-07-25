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

	if percentage == 0 {
		return StatusInfo{Emoji: EMOJI_NO_TIME}
	}
	if percentage > 0 && percentage <= midPoint {
		return StatusInfo{Emoji: EMOJI_ON_TRACK}
	}
	if percentage > midPoint && percentage <= highPoint {
		return StatusInfo{Emoji: EMOJI_HIGH_USAGE}
	}
	if percentage > highPoint && percentage < THRESHOLD_OVER {
		return StatusInfo{Emoji: EMOJI_CRITICAL}
	}
	if percentage >= THRESHOLD_OVER {
		return StatusInfo{Emoji: EMOJI_OVER_BUDGET}
	}
	return StatusInfo{Emoji: EMOJI_NO_TIME}
}

// GetThresholdStatus returns status info for threshold reporting
func GetThresholdStatus(threshold float64) StatusInfo {
	if threshold >= THRESHOLD_OVER {
		return StatusInfo{Emoji: EMOJI_CRITICAL}
	}
	if threshold >= THRESHOLD_CRITICAL {
		return StatusInfo{Emoji: EMOJI_OVER_BUDGET}
	}
	if threshold >= THRESHOLD_HIGH {
		return StatusInfo{Emoji: EMOJI_HIGH_USAGE}
	}
	if threshold >= THRESHOLD_WARNING {
		return StatusInfo{Emoji: EMOJI_WARNING}
	}
	return StatusInfo{Emoji: EMOJI_CHART}
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
