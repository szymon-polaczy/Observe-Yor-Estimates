package main

// Status Emojis
const (
	EMOJI_ON_TRACK    = "🟢"
	EMOJI_HIGH_USAGE  = "🟠"
	EMOJI_OVER_BUDGET = "🔴"
	EMOJI_CRITICAL    = "🚨"
	EMOJI_WARNING     = "🟡"
	EMOJI_NO_TIME     = "⚫"
)

// General Emojis
const (
	EMOJI_CHART       = "📊"
	EMOJI_TRENDING_UP = "📈"
	EMOJI_CALENDAR    = "📅"
	EMOJI_CLIPBOARD   = "📋"
	EMOJI_FOLDER      = "📁"
	EMOJI_LIGHTNING   = "⚡"
	EMOJI_CHECK       = "✅"
	EMOJI_TARGET      = "🎯"
	EMOJI_LIGHTBULB   = "💡"
	EMOJI_MAGNIFYING  = "🔍"
	EMOJI_CLOCK       = "⏰"
	EMOJI_MEMO        = "📝"
	EMOJI_CROSS       = "❌"
	EMOJI_GEAR        = "🔄"
	EMOJI_ROCKET      = "🚀"
	EMOJI_CELEBRATION = "🎉"
)

// Threshold Constants
const (
	THRESHOLD_WARNING  = 50.0
	THRESHOLD_HIGH     = 80.0
	THRESHOLD_CRITICAL = 90.0
	THRESHOLD_OVER     = 100.0
)

// Status Messages
const (
	STATUS_ON_TRACK    = "on track"
	STATUS_HIGH_USAGE  = "high usage"
	STATUS_CRITICAL    = "critical"
	STATUS_OVER_BUDGET = "over budget"
	STATUS_WARNING     = "warning"
	STATUS_NO_TIME     = "no time"
	STATUS_UNKNOWN     = "unknown"
)

// Status Descriptions
const (
	DESC_WARNING_LEVEL  = "Warning Level"
	DESC_HIGH_USAGE     = "High Usage"
	DESC_CRITICAL_USAGE = "Critical Usage"
	DESC_OVER_BUDGET    = "Over Budget"
	DESC_USAGE_REPORT   = "Usage Report"
)

// Slack Limits
const (
	MAX_SLACK_BLOCKS         = 50
	MAX_SLACK_MESSAGE_CHARS  = 3000
	MAX_BLOCKS_PER_MESSAGE   = 47   // Leave buffer for header/footer
	MAX_MESSAGE_CHARS_BUFFER = 2900 // Leave buffer for safety
)

// Common message templates
const (
	TEMPLATE_NO_CHANGES      = "%s No task changes to report for your %s update! %s"
	TEMPLATE_TASK_UPDATE     = "%s %s Task Update"
	TEMPLATE_PERSONAL_UPDATE = "%s Your %s task update"
	TEMPLATE_THRESHOLD_TITLE = "%s %s: %.0f%% Threshold Report"
	TEMPLATE_PROJECT_TITLE   = "%s %s Project"
	TEMPLATE_OTHER_TASKS     = "%s Other Tasks"
)

// Default configuration values
const (
	DEFAULT_MID_POINT  = 50.0
	DEFAULT_HIGH_POINT = 90.0
)
