package main

import (
	"strconv"
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

func CalcDateRanges(periodType string, days int) PeriodDateRanges {
	now := time.Now()
	dateFormat := "2006-01-02"
	historicalStart := "2000-01-01"

	switch periodType {
	case "today":
		today := now.Format(dateFormat)
		return PeriodDateRanges{
			Current:  DateRange{Start: today, End: today},
			Previous: DateRange{Start: historicalStart, End: now.AddDate(0, 0, -1).Format(dateFormat)},
		}

	case "yesterday":
		yesterday := now.AddDate(0, 0, -1).Format(dateFormat)
		return PeriodDateRanges{
			Current:  DateRange{Start: yesterday, End: yesterday},
			Previous: DateRange{Start: historicalStart, End: now.AddDate(0, 0, -2).Format(dateFormat)},
		}

	case "this_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfWeek := now.AddDate(0, 0, -weekday+1)
		endOfWeek := startOfWeek.AddDate(0, 0, 6)

		return PeriodDateRanges{
			Current:  DateRange{Start: startOfWeek.Format(dateFormat), End: endOfWeek.Format(dateFormat)},
			Previous: DateRange{Start: historicalStart, End: startOfWeek.AddDate(0, 0, -1).Format(dateFormat)},
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
			Current:  DateRange{Start: startOfLastWeek.Format(dateFormat), End: endOfLastWeek.Format(dateFormat)},
			Previous: DateRange{Start: historicalStart, End: startOfLastWeek.AddDate(0, 0, -1).Format(dateFormat)},
		}

	case "this_month":
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, -1)

		return PeriodDateRanges{
			Current:  DateRange{Start: startOfMonth.Format(dateFormat), End: endOfMonth.Format(dateFormat)},
			Previous: DateRange{Start: historicalStart, End: startOfMonth.AddDate(0, 0, -1).Format(dateFormat)},
		}

	case "last_month":
		startOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		startOfLastMonth := startOfThisMonth.AddDate(0, -1, 0)
		endOfLastMonth := startOfThisMonth.AddDate(0, 0, -1)

		return PeriodDateRanges{
			Current:  DateRange{Start: startOfLastMonth.Format(dateFormat), End: endOfLastMonth.Format(dateFormat)},
			Previous: DateRange{Start: historicalStart, End: startOfLastMonth.AddDate(0, 0, -1).Format(dateFormat)},
		}

	case "last_x_days":
		endDate := now.AddDate(0, 0, -1)
		startDate := endDate.AddDate(0, 0, -days+1)

		return PeriodDateRanges{
			Current:  DateRange{Start: startDate.Format(dateFormat), End: endDate.Format(dateFormat)},
			Previous: DateRange{Start: historicalStart, End: startDate.AddDate(0, 0, -1).Format(dateFormat)},
		}

	default:
		return CalcDateRanges("yesterday", 1)
	}
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
