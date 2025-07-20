package main

import (
	"fmt"
	"strings"
	"time"
)

// Common block builders for Slack messages

// CreateHeaderBlock creates a header block with title
func CreateHeaderBlock(title string) Block {
	return Block{
		Type: "header",
		Text: &Text{Type: "plain_text", Text: title},
	}
}

// CreateSectionBlock creates a section block with markdown text
func CreateSectionBlock(text string) Block {
	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: text},
	}
}

// CreateContextBlock creates a context block with elements
func CreateContextBlock(texts ...string) Block {
	elements := make([]Element, len(texts))
	for i, text := range texts {
		elements[i] = Element{Type: "mrkdwn", Text: text}
	}
	
	return Block{
		Type:     "context",
		Elements: elements,
	}
}

// CreateDividerBlock creates a divider block
func CreateDividerBlock() Block {
	return Block{Type: "divider"}
}

// CreateSpacingBlock creates an empty section for spacing
func CreateSpacingBlock() Block {
	return Block{
		Type: "section",
		Text: &Text{Type: "mrkdwn", Text: " "},
	}
}

// Block builder functions for common patterns

// BuildProgressBlocks creates blocks for progress messages
func BuildProgressBlocks(message string) []Block {
	return []Block{
		CreateSectionBlock(message),
	}
}

// BuildErrorBlocks creates blocks for error messages
func BuildErrorBlocks(errorMsg string) []Block {
	fullMessage := fmt.Sprintf("%s %s", EMOJI_CROSS, errorMsg)
	return []Block{
		CreateSectionBlock(fullMessage),
	}
}

// BuildNoChangesBlocks creates blocks for "no changes" messages
func BuildNoChangesBlocks(period string) []Block {
	message := fmt.Sprintf(TEMPLATE_NO_CHANGES, EMOJI_CHART, period, EMOJI_CELEBRATION)
	return []Block{
		CreateSectionBlock(message),
	}
}

// BuildThresholdHeaderBlocks creates header blocks for threshold reports
func BuildThresholdHeaderBlocks(threshold float64, period string, taskCount, projectCount int) []Block {
	status := GetThresholdStatus(threshold)
	title := fmt.Sprintf(TEMPLATE_THRESHOLD_TITLE, status.Emoji, status.Description, threshold)
	description := fmt.Sprintf("📅 Period: %s | Found %d tasks across %d projects", period, taskCount, projectCount)
	
	return []Block{
		CreateHeaderBlock(title),
		CreateSectionBlock(fmt.Sprintf("*%s*\n%s", title, description)),
		CreateContextBlock("_Tasks split by project for better readability_"),
		CreateDividerBlock(),
	}
}

// BuildTaskHeaderBlocks creates header blocks for standard task updates
func BuildTaskHeaderBlocks(title, period string, showDate bool) []Block {
	blocks := []Block{
		CreateSpacingBlock(),
		CreateHeaderBlock(title),
	}
	
	if showDate {
		dateStr := time.Now().Format("January 2, 2006")
		blocks = append(blocks, CreateContextBlock(dateStr))
	}
	
	blocks = append(blocks, CreateDividerBlock())
	return blocks
}

// BuildProjectHeaderBlocks creates header blocks for project-specific messages
func BuildProjectHeaderBlocks(project string, taskCount, projectNum, totalProjects, partNum, totalParts int) []Block {
	var projectTitle string
	if project == "Other" {
		projectTitle = fmt.Sprintf(TEMPLATE_OTHER_TASKS, EMOJI_CLIPBOARD)
	} else {
		projectTitle = fmt.Sprintf(TEMPLATE_PROJECT_TITLE, EMOJI_FOLDER, project)
	}

	headerText := fmt.Sprintf("%s (%d/%d)", projectTitle, projectNum, totalProjects)
	if totalParts > 1 {
		headerText = fmt.Sprintf("%s - Part %d of %d", headerText, partNum, totalParts)
	}

	description := fmt.Sprintf("_%d tasks in this project_", taskCount)

	return []Block{
		CreateSpacingBlock(),
		CreateSectionBlock(fmt.Sprintf("*%s*\n%s", headerText, description)),
		CreateDividerBlock(),
	}
}

// BuildSuggestionBlocks creates blocks with suggestions for different contexts
func BuildSuggestionBlocks(threshold float64) []Block {
	suggestion := GetSuggestionForThreshold(threshold)
	return []Block{
		CreateContextBlock(suggestion),
	}
}

// BuildCompletionBlocks creates blocks for completion messages
func BuildCompletionBlocks(operation string, duration time.Duration) []Block {
	title := fmt.Sprintf("%s %s Complete", EMOJI_CHECK, operation)
	details := fmt.Sprintf("*%s completed successfully*\n\n*Duration:* %v\n*Completed at:* %s",
		operation, duration.Round(time.Second), time.Now().Format("2006-01-02 15:04:05"))

	return []Block{
		CreateHeaderBlock(title),
		CreateSectionBlock(details),
	}
}

// BuildSystemAlertBlocks creates blocks for system alerts
func BuildSystemAlertBlocks(operation string, err error) []Block {
	title := fmt.Sprintf("%s System Alert", EMOJI_CRITICAL)
	details := fmt.Sprintf("*Operation:* %s\n*Error:* `%v`\n*Time:* %s",
		operation, err, time.Now().Format("2006-01-02 15:04:05"))

	return []Block{
		CreateHeaderBlock(title),
		CreateSectionBlock(details),
	}
}

// Helper functions for complex block creation

// CreateTaskBlock creates a comprehensive task information block
func CreateTaskBlock(task TaskInfo, showComments bool, maxComments int) Block {
	taskName := sanitizeText(task.Name)
	
	var content string
	content += fmt.Sprintf("*%s*\n", taskName)
	
	// Time information
	content += fmt.Sprintf("• %s: *%s* | %s: *%s*\n",
		task.CurrentPeriod, task.CurrentTime,
		task.PreviousPeriod, task.PreviousTime)
	
	// Estimation info
	if task.EstimationInfo.Text != "" {
		content += fmt.Sprintf("• %s\n", sanitizeText(task.EstimationInfo.Text))
	}
	
	// Comments
	if showComments && len(task.Comments) > 0 {
		content += "• Comments:\n"
		comments := removeEmptyComments(task.Comments)
		
		limit := maxComments
		if limit <= 0 || limit > len(comments) {
			limit = len(comments)
		}
		
		for i := 0; i < limit; i++ {
			comment := sanitizeText(comments[i])
			if len(comment) > 150 {
				comment = comment[:147] + "..."
			}
			content += fmt.Sprintf("  %d. %s\n", i+1, comment)
		}
		
		if len(comments) > limit {
			remaining := len(comments) - limit
			content += fmt.Sprintf("  ... and %d more comments\n", remaining)
		}
	}
	
	return CreateSectionBlock(content)
}

// CreateUserBreakdownBlock creates a block showing user time contributions
func CreateUserBreakdownBlock(userBreakdown map[int]UserContribution) Block {
	if len(userBreakdown) <= 1 {
		return Block{} // Return empty block if single user or no breakdown
	}
	
	var content strings.Builder
	content.WriteString("*User Breakdown:*\n")
	
	for userID, contrib := range userBreakdown {
		content.WriteString(fmt.Sprintf("• User %d: %s current, %s previous\n", 
			userID, contrib.CurrentTime, contrib.PreviousTime))
	}
	
	return CreateSectionBlock(content.String())
}

// Block collection helpers

// CombineBlocks safely combines multiple block slices with limit checking
func CombineBlocks(blockGroups ...[]Block) []Block {
	var combined []Block
	
	for _, group := range blockGroups {
		for _, block := range group {
			if len(combined) >= MAX_BLOCKS_PER_MESSAGE {
				break
			}
			combined = append(combined, block)
		}
		if len(combined) >= MAX_BLOCKS_PER_MESSAGE {
			break
		}
	}
	
	return combined
}

// TruncateBlocks ensures block count doesn't exceed limits
func TruncateBlocks(blocks []Block, maxBlocks int) []Block {
	if len(blocks) <= maxBlocks {
		return blocks
	}
	
	truncated := blocks[:maxBlocks-1]
	truncated = append(truncated, CreateContextBlock("_... (Some content truncated due to message size limits)_"))
	
	return truncated
}