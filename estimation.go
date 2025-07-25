package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}

func ParseTaskEstimation(taskName string) EstimationInfo {
	for _, pattern := range estimationPatterns {
		matches := pattern.regex.FindStringSubmatch(taskName)

		if pattern.isRange && len(matches) == 3 {
			first, err1 := parseFloat(matches[1])
			second, err2 := parseFloat(matches[2])

			if err1 != nil || err2 != nil {
				return EstimationInfo{ErrorMessage: "invalid estimation numbers"}
			}

			var optimistic, pessimistic float64
			if pattern.isAddition {
				optimistic = first
				pessimistic = first + second
			} else {
				optimistic = first
				pessimistic = second
			}

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

		} else if !pattern.isRange && len(matches) == 2 {
			estimate, err := parseFloat(matches[1])
			if err != nil {
				return EstimationInfo{ErrorMessage: "invalid estimation number"}
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
	}

	return EstimationInfo{ErrorMessage: "no estimation given"}
}

func CalcUsagePercent(currentTime, previousTime string, estimation EstimationInfo) (float64, error) {
	if estimation.ErrorMessage != "" {
		return 0, fmt.Errorf("invalid estimation: %s", estimation.ErrorMessage)
	}

	currentSeconds := parseTimeToSeconds(currentTime)
	previousSeconds := parseTimeToSeconds(previousTime)
	totalSeconds := currentSeconds + previousSeconds

	estimateSeconds := estimation.Pessimistic * 3600

	if estimateSeconds == 0 {
		return 0, nil
	}

	return (float64(totalSeconds) / estimateSeconds) * 100, nil
}

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
	estimation.Text = fmt.Sprintf("%s | %s %.1f%%", estimation.Text, estimation.Status.Emoji, percentage)

	return estimation
}

func parseTimeToSeconds(timeStr string) int {
	if timeStr == "0h 0m" || timeStr == "" {
		return 0
	}

	var hours, minutes int
	hRegex := regexp.MustCompile(`(\d+)h`)
	mRegex := regexp.MustCompile(`(\d+)m`)

	if hMatch := hRegex.FindStringSubmatch(timeStr); len(hMatch) > 1 {
		hours, _ = strconv.Atoi(hMatch[1])
	}

	if mMatch := mRegex.FindStringSubmatch(timeStr); len(mMatch) > 1 {
		minutes, _ = strconv.Atoi(mMatch[1])
	}

	return hours*3600 + minutes*60
}
