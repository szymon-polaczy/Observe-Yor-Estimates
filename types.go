package main

import "time"

// Core status information
type StatusInfo struct {
	Emoji       string
	Status      string
	Description string
}

// Estimation parsing results
type EstimationInfo struct {
	Text         string  // "Estimation: 2-4 hours"
	Optimistic   float64 // 2.0
	Pessimistic  float64 // 4.0
	HasRange     bool    // true if range, false if single value
	Percentage   float64 // calculated usage percentage
	Status       StatusInfo
	ErrorMessage string // if parsing failed
}

// Time period information
type PeriodInfo struct {
	Type        string // "last_x_days", "today", "yesterday", etc.
	Days        int    // Number of days for "last_x_days" type
	DisplayName string // Human-readable name
}

// User contribution to a task (legacy name for compatibility)
type UserTimeContribution struct {
	UserID       int
	CurrentTime  string
	PreviousTime string
}

// User contribution to a task (new name)
type UserContribution struct {
	UserID       int
	CurrentTime  string
	PreviousTime string
}

// TaskUpdateInfo - legacy format still used by existing functions
type TaskUpdateInfo struct {
	TaskID           int
	ParentID         int
	Name             string
	EstimationInfo   string
	EstimationStatus string
	CurrentPeriod    string
	CurrentTime      string
	PreviousPeriod   string
	PreviousTime     string
	DaysWorked       int
	Comments         []string
	UserBreakdown    map[int]UserTimeContribution
}

// Simplified task information (consolidates TaskUpdateInfo variations)
type TaskInfo struct {
	TaskID           int
	ParentID         int
	Name             string
	EstimationInfo   EstimationInfo
	CurrentPeriod    string
	CurrentTime      string
	PreviousPeriod   string
	PreviousTime     string
	DaysWorked       int
	Comments         []string
	UserBreakdown    map[int]UserTimeContribution
}

// Threshold alert information
type ThresholdAlert struct {
	TaskID           int
	ParentID         int
	Name             string
	EstimationInfo   EstimationInfo
	CurrentTime      string
	PreviousTime     string
	Percentage       float64
	ThresholdCrossed int
	JustCrossed      bool
}

// Slack message structures (consolidated)
type SlackMessage struct {
	Text   string `json:"text"`
	Blocks []Block `json:"blocks"`
}

type Block struct {
	Type      string                 `json:"type"`
	Text      *Text                  `json:"text,omitempty"`
	Fields    []Field                `json:"fields,omitempty"`
	Elements  []Element              `json:"elements,omitempty"`
	Accessory *Accessory             `json:"accessory,omitempty"`
	BlockID   string                 `json:"block_id,omitempty"`
	Label     *Text                  `json:"label,omitempty"`
	Element   map[string]interface{} `json:"element,omitempty"`
}

type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Field struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Element struct {
	Type     string      `json:"type"`
	Text     interface{} `json:"text,omitempty"`
	ActionID string      `json:"action_id,omitempty"`
	Style    string      `json:"style,omitempty"`
	Value    string      `json:"value,omitempty"`
}

type Accessory struct {
	Type           string                   `json:"type"`
	Text           *Text                    `json:"text,omitempty"`
	ActionID       string                   `json:"action_id,omitempty"`
	Options        []map[string]interface{} `json:"options,omitempty"`
	InitialOptions []map[string]interface{} `json:"initial_options,omitempty"`
}

// Slack API structures
type SlackAPIClient struct {
	botToken string
	logger   *Logger
}

type SlackAPIResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"ts,omitempty"`
	Channel   string `json:"channel,omitempty"`
}

type SlackCommandRequest struct {
	Token       string `json:"token"`
	TeamID      string `json:"team_id"`
	TeamDomain  string `json:"team_domain"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	Command     string `json:"command"`
	Text        string `json:"text"`
	ResponseURL string `json:"response_url"`
	ProjectName string `json:"project_name,omitempty"`
	TriggerID   string `json:"trigger_id"`
}

type SlackCommandResponse struct {
	ResponseType string  `json:"response_type"`
	Text         string  `json:"text"`
	Blocks       []Block `json:"blocks,omitempty"`
}

// Conversation context for Slack interactions
type ConversationContext struct {
	ChannelID   string
	UserID      string
	ThreadTS    string
	CommandType string
	ProjectName string
}

// Message validation results
type MessageValidation struct {
	IsValid        bool
	BlockCount     int
	CharacterCount int
	ExceedsBlocks  bool
	ExceedsChars   bool
	ErrorMessage   string
}

// Task hierarchy information
type Task struct {
	ID       int
	ParentID int
	Name     string
}

// Project information
type Project struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	TimeCampTaskID int       `json:"timecamp_task_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Date range for period calculations
type DateRange struct {
	Start string
	End   string
	Label string
}

type PeriodDateRanges struct {
	Current  DateRange
	Previous DateRange
}

// Retry configuration for HTTP requests
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// Format options for messages
type FormatOptions struct {
	IsPersonal    bool
	InThread      bool
	ShowHeader    bool
	ShowFooter    bool
	MaxTasks      int
	Threshold     *float64 // nil for normal updates, value for threshold reports
}