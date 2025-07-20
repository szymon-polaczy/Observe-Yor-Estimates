package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Consolidated estimation patterns (supports . and , as decimal separators)
var estimationPatterns = []struct {
	regex      *regexp.Regexp
	isRange    bool
	isAddition bool
}{
	// Range formats
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)\]`), true, false},   // [2-4]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)h\]`), true, false}, // [2h-4h]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)-([0-9]+(?:[.,][0-9]+)?)h\]`), true, false},  // [2-4h]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h-([0-9]+(?:[.,][0-9]+)?)\]`), true, false},  // [2h-4]

	// Addition formats
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)\]`), true, true},   // [2+1]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\+([0-9]+(?:[.,][0-9]+)?)h\]`), true, true},  // [2+1h]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)\]`), true, true},  // [2h+1]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\+([0-9]+(?:[.,][0-9]+)?)h\]`), true, true}, // [2h+1h]

	// Single formats
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)\]`), false, false},  // [3]
	{regexp.MustCompile(`\[([0-9]+(?:[.,][0-9]+)?)h\]`), false, false}, // [3h]
}

// parseFloat supports both . and , as decimal separators
func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

// formatFloat removes unnecessary decimals
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}

// ParseTaskEstimation extracts estimation info from task name
func ParseTaskEstimation(taskName string) EstimationInfo {
	for _, pattern := range estimationPatterns {
		matches := pattern.regex.FindStringSubmatch(taskName)

		if pattern.isRange && len(matches) == 3 {
			return parseRangeEstimation(matches, pattern.isAddition, taskName)
		} else if !pattern.isRange && len(matches) == 2 {
			return parseSingleEstimation(matches, taskName)
		}
	}

	return EstimationInfo{
		Text:         "",
		ErrorMessage: "no estimation given",
	}
}

// parseRangeEstimation handles range and addition patterns
func parseRangeEstimation(matches []string, isAddition bool, taskName string) EstimationInfo {
	first, err1 := parseFloat(matches[1])
	second, err2 := parseFloat(matches[2])

	if err1 != nil || err2 != nil {
		return EstimationInfo{
			Text:         "",
			ErrorMessage: "invalid estimation numbers",
		}
	}

	var optimistic, pessimistic float64
	if isAddition {
		optimistic = first
		pessimistic = first + second
	} else {
		optimistic = first
		pessimistic = second
	}

	// Validation
	if optimistic > 100 || pessimistic > 100 {
		return EstimationInfo{
			Text:         fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)),
			Optimistic:   optimistic,
			Pessimistic:  pessimistic,
			HasRange:     true,
			ErrorMessage: "estimation numbers too large (max: 100)",
		}
	}

	if optimistic > pessimistic {
		return EstimationInfo{
			Text:         fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)),
			Optimistic:   optimistic,
			Pessimistic:  pessimistic,
			HasRange:     true,
			ErrorMessage: "broken estimation (optimistic > pessimistic)",
		}
	}

	return EstimationInfo{
		Text:        fmt.Sprintf("Estimation: %s-%s hours", formatFloat(optimistic), formatFloat(pessimistic)),
		Optimistic:  optimistic,
		Pessimistic: pessimistic,
		HasRange:    true,
	}
}

// parseSingleEstimation handles single number patterns
func parseSingleEstimation(matches []string, taskName string) EstimationInfo {
	estimate, err := parseFloat(matches[1])
	if err != nil {
		return EstimationInfo{
			Text:         "",
			ErrorMessage: "invalid estimation number",
		}
	}

	if estimate > 100 {
		return EstimationInfo{
			Text:         fmt.Sprintf("Estimation: %s hours", formatFloat(estimate)),
			Optimistic:   estimate,
			Pessimistic:  estimate,
			HasRange:     false,
			ErrorMessage: "estimation number too large (max: 100)",
		}
	}

	return EstimationInfo{
		Text:        fmt.Sprintf("Estimation: %s hours", formatFloat(estimate)),
		Optimistic:  estimate,
		Pessimistic: estimate,
		HasRange:    false,
	}
}

// CalcUsagePercent calculates time usage percentage against estimation
func CalcUsagePercent(currentTime, previousTime string, estimation EstimationInfo) (float64, error) {
	if estimation.ErrorMessage != "" {
		return 0, fmt.Errorf("invalid estimation: %s", estimation.ErrorMessage)
	}

	currentSeconds := parseTimeToSeconds(currentTime)
	previousSeconds := parseTimeToSeconds(previousTime)
	totalSeconds := currentSeconds + previousSeconds

	// Use pessimistic estimate for percentage calculation
	estimateHours := estimation.Pessimistic
	estimateSeconds := estimateHours * 3600

	if estimateSeconds == 0 {
		return 0, nil
	}

	percentage := (float64(totalSeconds) / estimateSeconds) * 100
	return percentage, nil
}

// ParseTaskEstimationWithUsage combines estimation parsing with usage calculation
func ParseTaskEstimationWithUsage(taskName, currentTime, previousTime string) EstimationInfo {
	estimation := ParseTaskEstimation(taskName)
	if estimation.ErrorMessage != "" {
		return estimation
	}

	percentage, err := CalcUsagePercent(currentTime, previousTime, estimation)
	if err != nil {
		estimation.ErrorMessage = err.Error()
		return estimation
	}

	estimation.Percentage = percentage
	estimation.Status = GetTaskStatus(percentage)

	// Enhanced text with usage info
	estimation.Text = fmt.Sprintf("%s | %s %.1f%%",
		estimation.Text,
		estimation.Status.Emoji,
		percentage)

	return estimation
}

// parseTimeToSeconds converts time strings like "2h 30m" to seconds
func parseTimeToSeconds(timeStr string) int {
	if timeStr == "0h 0m" || timeStr == "" {
		return 0
	}

	var hours, minutes int
	hRegex := regexp.MustCompile(`(\d+)h`)
	mRegex := regexp.MustCompile(`(\d+)m`)

	hMatch := hRegex.FindStringSubmatch(timeStr)
	if len(hMatch) > 1 {
		hours, _ = strconv.Atoi(hMatch[1])
	}

	mMatch := mRegex.FindStringSubmatch(timeStr)
	if len(mMatch) > 1 {
		minutes, _ = strconv.Atoi(mMatch[1])
	}

	return hours*3600 + minutes*60
}
