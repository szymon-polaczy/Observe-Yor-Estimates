package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseTimeToSeconds converts time strings like "2h 30m" to seconds
func ParseTimeToSeconds(timeStr string) int {
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

// FormatDuration converts seconds to readable time format
func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "0h 0m"
	}

	hours := seconds / 3600
	remainingSeconds := seconds % 3600
	minutes := remainingSeconds / 60

	return formatHoursMinutes(hours, minutes)
}

// formatHoursMinutes creates standardized hour/minute strings
func formatHoursMinutes(hours, minutes int) string {
	if hours == 0 && minutes == 0 {
		return "0h 0m"
	}
	if hours == 0 {
		return strconv.Itoa(minutes) + "m"
	}
	if minutes == 0 {
		return strconv.Itoa(hours) + "h"
	}
	return strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
}

// CalcDateRanges calculates date ranges for different period types
func CalcDateRanges(periodType string, days int) PeriodDateRanges {
	now := time.Now()
	
	switch periodType {
	case "today":
		return PeriodDateRanges{
			Current: DateRange{
				Start: now.Format("2006-01-02"),
				End:   now.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   now.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	case "yesterday":
		yesterday := now.AddDate(0, 0, -1)
		return PeriodDateRanges{
			Current: DateRange{
				Start: yesterday.Format("2006-01-02"),
				End:   yesterday.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   now.AddDate(0, 0, -2).Format("2006-01-02"),
			},
		}
		
	case "this_week":
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		startOfWeek := now.AddDate(0, 0, -weekday+1)
		endOfWeek := startOfWeek.AddDate(0, 0, 6)
		
		return PeriodDateRanges{
			Current: DateRange{
				Start: startOfWeek.Format("2006-01-02"),
				End:   endOfWeek.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   startOfWeek.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	case "last_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfThisWeek := now.AddDate(0, 0, -weekday+1)
		startOfLastWeek := startOfThisWeek.AddDate(0, 0, -7)
		endOfLastWeek := startOfLastWeek.AddDate(0, 0, 6)
		
		return PeriodDateRanges{
			Current: DateRange{
				Start: startOfLastWeek.Format("2006-01-02"),
				End:   endOfLastWeek.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   startOfLastWeek.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	case "this_month":
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, -1)
		
		return PeriodDateRanges{
			Current: DateRange{
				Start: startOfMonth.Format("2006-01-02"),
				End:   endOfMonth.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   startOfMonth.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	case "last_month":
		startOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		startOfLastMonth := startOfThisMonth.AddDate(0, -1, 0)
		endOfLastMonth := startOfThisMonth.AddDate(0, 0, -1)
		
		return PeriodDateRanges{
			Current: DateRange{
				Start: startOfLastMonth.Format("2006-01-02"),
				End:   endOfLastMonth.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   startOfLastMonth.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	case "last_x_days":
		endDate := now.AddDate(0, 0, -1) // Yesterday
		startDate := endDate.AddDate(0, 0, -days+1)
		
		return PeriodDateRanges{
			Current: DateRange{
				Start: startDate.Format("2006-01-02"),
				End:   endDate.Format("2006-01-02"),
			},
			Previous: DateRange{
				Start: "2000-01-01",
				End:   startDate.AddDate(0, 0, -1).Format("2006-01-02"),
			},
		}
		
	default:
		// Default to yesterday
		return CalcDateRanges("yesterday", 1)
	}
}

// GetPeriodDisplayName returns human-readable period names
func GetPeriodDisplayName(periodType string, days int) string {
	switch periodType {
	case "today":
		return "Today"
	case "yesterday":
		return "Yesterday"
	case "this_week":
		return "This Week"
	case "last_week":
		return "Last Week"
	case "this_month":
		return "This Month"
	case "last_month":
		return "Last Month"
	case "last_x_days":
		if days == 1 {
			return "Yesterday"
		}
		return "Last " + strconv.Itoa(days) + " Days"
	default:
		return "Unknown Period"
	}
}

// ParsePeriodFromText extracts period information from natural language
func ParsePeriodFromText(text string) PeriodInfo {
	text = strings.ToLower(strings.TrimSpace(text))
	words := strings.Fields(text)

	// Look for "last X days" pattern
	for i, word := range words {
		if word == "last" && i+2 < len(words) && words[i+2] == "days" {
			if days, err := strconv.Atoi(words[i+1]); err == nil && days >= 1 && days <= 60 {
				return PeriodInfo{
					Type:        "last_x_days",
					Days:        days,
					DisplayName: GetPeriodDisplayName("last_x_days", days),
				}
			}
		}
	}

	// Check for specific patterns
	switch {
	case contains(text, "today"):
		return PeriodInfo{Type: "today", Days: 0, DisplayName: "Today"}
	case contains(text, "yesterday"):
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	case contains(text, "this week"):
		return PeriodInfo{Type: "this_week", Days: 0, DisplayName: "This Week"}
	case contains(text, "last week"):
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	case contains(text, "this month"):
		return PeriodInfo{Type: "this_month", Days: 0, DisplayName: "This Month"}
	case contains(text, "last month"):
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	case contains(text, "week"):
		return PeriodInfo{Type: "last_week", Days: 7, DisplayName: "Last Week"}
	case contains(text, "month"):
		return PeriodInfo{Type: "last_month", Days: 30, DisplayName: "Last Month"}
	case contains(text, "day"):
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	default:
		return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
	}
}

// AddTimeStrings adds two time strings together
func AddTimeStrings(time1, time2 string) string {
	seconds1 := ParseTimeToSeconds(time1)
	seconds2 := ParseTimeToSeconds(time2)
	return FormatDuration(seconds1 + seconds2)
}

// SubtractTimeStrings subtracts second time from first time
func SubtractTimeStrings(time1, time2 string) string {
	seconds1 := ParseTimeToSeconds(time1)
	seconds2 := ParseTimeToSeconds(time2)
	result := seconds1 - seconds2
	if result < 0 {
		result = 0
	}
	return FormatDuration(result)
}

// CompareTimeStrings returns -1, 0, or 1 if time1 is less, equal, or greater than time2
func CompareTimeStrings(time1, time2 string) int {
	seconds1 := ParseTimeToSeconds(time1)
	seconds2 := ParseTimeToSeconds(time2)
	
	if seconds1 < seconds2 {
		return -1
	} else if seconds1 > seconds2 {
		return 1
	}
	return 0
}

// IsValidTimeString checks if a string is a valid time format
func IsValidTimeString(timeStr string) bool {
	if timeStr == "" || timeStr == "0h 0m" {
		return true
	}
	
	// Check if it matches expected pattern
	matched, _ := regexp.MatchString(`^\d+h \d+m$|^\d+h$|^\d+m$`, timeStr)
	return matched
}

// Helper functions

func contains(text, substr string) bool {
	return strings.Contains(text, substr)
}