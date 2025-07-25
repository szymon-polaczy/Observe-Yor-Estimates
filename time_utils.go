package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "0h 0m"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60

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

func CalculatePeriodRange(periodType string, days int) PeriodRange {
	now := time.Now()

	switch periodType {
	case "today":
		// Today: 00:00 to 23:59
		today := now.Format("2006-01-02")
		todayStart := today + " 00:00:00"
		todayEnd := today + " 23:59:59"

		return PeriodRange{Start: todayStart, End: todayEnd, Label: "Today"}

	case "yesterday", "daily":
		// Yesterday: 00:00 to 23:59
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		yesterdayStart := yesterday + " 00:00:00"
		yesterdayEnd := yesterday + " 23:59:59"

		return PeriodRange{Start: yesterdayStart, End: yesterdayEnd, Label: "Yesterday"}

	case "this_week":
		// This week: Monday 00:00 to coming Sunday 23:59
		daysFromMonday := int(now.Weekday()) - int(time.Monday)
		if daysFromMonday < 0 {
			daysFromMonday += 7 // Handle Sunday (0) -> 6
		}

		mondayThisWeek := now.AddDate(0, 0, -daysFromMonday)
		sundayThisWeek := mondayThisWeek.AddDate(0, 0, 6)

		weekStart := mondayThisWeek.Format("2006-01-02") + " 00:00:00"
		weekEnd := sundayThisWeek.Format("2006-01-02") + " 23:59:59"

		return PeriodRange{Start: weekStart, End: weekEnd, Label: "This Week"}

	case "last_week", "weekly":
		// Last week: Previous Monday 00:00 to previous Sunday 23:59
		daysFromMonday := int(now.Weekday()) - int(time.Monday)
		if daysFromMonday < 0 {
			daysFromMonday += 7 // Handle Sunday (0) -> 6
		}

		mondayThisWeek := now.AddDate(0, 0, -daysFromMonday)
		mondayLastWeek := mondayThisWeek.AddDate(0, 0, -7)
		sundayLastWeek := mondayLastWeek.AddDate(0, 0, 6)

		weekStart := mondayLastWeek.Format("2006-01-02") + " 00:00:00"
		weekEnd := sundayLastWeek.Format("2006-01-02") + " 23:59:59"

		return PeriodRange{Start: weekStart, End: weekEnd, Label: "Last Week"}

	case "this_month":
		// This month: 1st 00:00 to last day 23:59
		monthStart := now.AddDate(0, 0, -now.Day()+1)
		monthEnd := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1) // Last day of month

		monthStartStr := monthStart.Format("2006-01-02") + " 00:00:00"
		monthEndStr := monthEnd.Format("2006-01-02") + " 23:59:59"

		return PeriodRange{Start: monthStartStr, End: monthEndStr, Label: "This Month"}

	case "last_month":
		// Last month: 1st 00:00 to last day 23:59 of previous month
		thisMonthStart := now.AddDate(0, 0, -now.Day()+1)
		lastMonthStart := thisMonthStart.AddDate(0, -1, 0)
		lastMonthEnd := thisMonthStart.AddDate(0, 0, -1) // Last day of previous month

		monthStartStr := lastMonthStart.Format("2006-01-02") + " 00:00:00"
		monthEndStr := lastMonthEnd.Format("2006-01-02") + " 23:59:59"

		return PeriodRange{Start: monthStartStr, End: monthEndStr, Label: "Last Month"}

	case "last_x_days":
		// Last X days: X days ago 00:00 to today 23:59
		startDate := now.AddDate(0, 0, -days)
		currentStart := startDate.Format("2006-01-02") + " 00:00:00"
		currentEnd := now.Format("2006-01-02") + " 23:59:59"

		currentLabel := fmt.Sprintf("Last %d Days", days)

		return PeriodRange{Start: currentStart, End: currentEnd, Label: currentLabel}

	default:
		// Fallback to yesterday
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		yesterdayStart := yesterday + " 00:00:00"
		yesterdayEnd := yesterday + " 23:59:59"

		return PeriodRange{Start: yesterdayStart, End: yesterdayEnd, Label: "Yesterday"}
	}
}

// ParsePeriodFromText parses period information from text input
func ParsePeriodFromText(text, command string) PeriodInfo {
	text = strings.ToLower(strings.TrimSpace(text))

	words := strings.Fields(text)
	for i, word := range words {
		if word == "last" && i+2 < len(words) {
			if words[i+2] == "days" || words[i+2] == "day" {
				if days, err := strconv.Atoi(words[i+1]); err == nil && days >= 1 && days <= 60 {
					return PeriodInfo{
						Type:        "last_x_days",
						Days:        days,
						DisplayName: fmt.Sprintf("Last %d Days", days),
					}
				}
			}
		}
	}

	periodMap := map[string]PeriodInfo{
		"today":      {Type: "today", Days: 0, DisplayName: "Today"},
		"yesterday":  {Type: "yesterday", Days: 1, DisplayName: "Yesterday"},
		"this week":  {Type: "this_week", Days: 0, DisplayName: "This Week"},
		"last week":  {Type: "last_week", Days: 7, DisplayName: "Last Week"},
		"this month": {Type: "this_month", Days: 0, DisplayName: "This Month"},
		"last month": {Type: "last_month", Days: 30, DisplayName: "Last Month"},
		"weekly":     {Type: "last_week", Days: 7, DisplayName: "Last Week"},
		"monthly":    {Type: "last_month", Days: 30, DisplayName: "Last Month"},
	}

	for keyword, period := range periodMap {
		if strings.Contains(text, keyword) {
			return period
		}
	}

	return PeriodInfo{Type: "yesterday", Days: 1, DisplayName: "Yesterday"}
}

func GetPeriodDisplayName(periodType string, days int) string {
	displayNames := map[string]string{
		"today":      "Today",
		"yesterday":  "Yesterday",
		"this_week":  "This Week",
		"last_week":  "Last Week",
		"this_month": "This Month",
		"last_month": "Last Month",
	}

	if name, exists := displayNames[periodType]; exists {
		return name
	}

	if periodType == "last_x_days" {
		if days == 1 {
			return "Yesterday"
		}
		return "Last " + strconv.Itoa(days) + " Days"
	}

	return "Unknown Period"
}
