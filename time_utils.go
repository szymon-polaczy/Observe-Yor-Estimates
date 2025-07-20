package main

import (
	"strconv"
	"time"
)

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
