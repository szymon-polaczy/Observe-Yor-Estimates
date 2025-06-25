# Slack Integration Guide

This guide covers the complete setup and usage of OYE's Slack integration, including slash commands, bot configuration, and response formats.

## 🎯 Overview

The OYE Slack integration provides:
- **Unified `/oye` command** for all time tracking operations
- **Real-time reporting** with rich formatting
- **Threshold monitoring** for budget overruns
- **Public/private responses** based on context
- **Progress updates** for long-running operations

## 🚀 Initial Setup

### 1. Create Slack App

1. Visit [Slack API Apps](https://api.slack.com/apps)
2. Click **"Create New App"**
3. Choose **"From scratch"**
4. Enter app details:
   - **App Name**: "OYE Time Tracker"
   - **Workspace**: Select your target workspace
5. Click **"Create App"**

### 2. Configure Bot Permissions

Navigate to **"OAuth & Permissions"** and add these scopes:

#### Bot Token Scopes
```
chat:write          # Send messages to channels
commands            # Receive slash command invocations
chat:write.public   # Send messages to channels the app isn't in
```

#### Optional Scopes (for future features)
```
users:read          # Read user information
channels:read       # Read channel information
```

### 3. Install App to Workspace

1. In **"OAuth & Permissions"**, click **"Install to Workspace"**
2. Review permissions and click **"Allow"**
3. Copy the **Bot User OAuth Token** (starts with `xoxb-`)
4. Store this as `SLACK_BOT_TOKEN` in your environment

### 4. Create Slash Command

1. Navigate to **"Slash Commands"**
2. Click **"Create New Command"**
3. Configure the command:

```
Command: /oye
Request URL: https://your-domain.com/slack/oye
Short Description: OYE time tracking commands
Usage Hint: [daily|weekly|monthly|sync|help] [public]
```

4. Save the command
5. **Reinstall your app** to activate the slash command

### 5. Get Verification Token

1. Go to **"Basic Information"**
2. Find **"App Credentials"** section
3. Copy the **Verification Token**
4. Store this as `SLACK_VERIFICATION_TOKEN` in your environment

## 🔧 Environment Configuration

Add these variables to your `.env` file:

```bash
# Slack Bot Configuration
SLACK_BOT_TOKEN=xoxb-1234567890-1234567890123-abcdefghijklmnopqrstuvwx
SLACK_VERIFICATION_TOKEN=abcdefghijklmnopqrstuvwx

# Optional: Default channel for direct API usage
SLACK_DEFAULT_CHANNEL=C1234567890
```

## 📝 Command Reference

### Basic Command Structure

```
/oye [command] [options]
```

### Available Commands

#### 1. Help Commands
```
/oye              # Show help (same as /oye help)
/oye help         # Display command reference
```

#### 2. Time Updates
```
/oye daily        # Daily time summary (default)
/oye weekly       # Weekly time summary  
/oye monthly      # Monthly time summary
/oye daily public # Daily summary visible to channel
```

#### 3. Data Synchronization
```
/oye sync         # Full data sync with TimeCamp
/oye full-sync    # Complete data synchronization
```

#### 4. Threshold Monitoring
```
/oye over 50 daily    # Tasks over 50% estimate (daily)
/oye over 80 weekly   # Tasks over 80% estimate (weekly)
/oye over 100 monthly # Tasks over budget (monthly)
```

### Command Options

#### Public vs Private Responses

**Private (default)**:
- Only visible to the user who ran the command
- Use for personal time tracking queries

**Public**:
- Visible to entire channel
- Add `public` anywhere in the command
- Use for team updates and reports

Examples:
```
/oye daily          # Private response
/oye daily public   # Public response
/oye public weekly  # Public response (order doesn't matter)
```

## 🎨 Response Formats

### Immediate Responses

All commands return an immediate acknowledgment within 3 seconds:

```json
{
  "response_type": "ephemeral",
  "text": "⏳ Generating your daily update! I'll show progress as I work..."
}
```

### Final Responses

Complete results are sent as follow-up messages with rich formatting.

#### Daily Update Example

```
📊 Daily Time Summary - November 15, 2024

• Today: 7h 30m [John Smith: 4h 15m, Mary Johnson: 3h 15m]
• This Week: 32h 45m [John Smith: 18h 30m, Mary Johnson: 14h 15m]

🎯 Top Tasks Today:
• Website Redesign: 3h 45m (75% of estimate)
• API Development: 2h 30m (50% of estimate) 
• Bug Fixes: 1h 15m (125% of estimate) ⚠️

📈 Weekly Progress:
• On Track: 8 tasks
• Over Estimate: 2 tasks
• Total Progress: 78% of planned work
```

#### Threshold Monitoring Example

```
⚠️ Tasks Over 80% Estimate (Weekly)

🔴 Critical (Over 100%):
• Bug Fix #123: 150% (12h / 8h estimated)
• Performance Optimization: 120% (9.6h / 8h estimated)

🟡 Warning (80-100%):
• Feature Implementation: 85% (6.8h / 8h estimated)
• Code Review: 90% (7.2h / 8h estimated)

📊 Summary: 4 tasks need attention out of 15 total
```

### Progress Updates

For long-running operations, OYE sends progress updates:

```
⏳ Syncing with TimeCamp... (Step 1/4)
📥 Fetching tasks and projects...
📥 Fetching time entries...
💾 Updating database...
✅ Sync completed! Updated 1,247 time entries and 156 tasks.
```

## 🔒 Security Configuration

### Token Security

**Never expose tokens in**:
- Public repositories
- Client-side code
- Log files
- Error messages

**Store tokens securely**:
```bash
# Use environment variables
export SLACK_BOT_TOKEN="xoxb-..."
export SLACK_VERIFICATION_TOKEN="abc..."

# Or .env file (not committed)
echo "SLACK_BOT_TOKEN=xoxb-..." >> .env
echo "SLACK_VERIFICATION_TOKEN=abc..." >> .env
```

### Request Verification

OYE automatically verifies all requests using the verification token:

1. Slack sends token with each request
2. OYE compares with `SLACK_VERIFICATION_TOKEN`
3. Invalid tokens receive `401 Unauthorized`
4. No token skips verification (not recommended for production)

### Permissions Best Practices

**Minimal Scopes**: Only request necessary permissions
```
✅ chat:write      # Required for responses
✅ commands        # Required for slash commands
❌ admin           # Not needed
❌ files:write     # Not needed
```

## 🎛️ Advanced Configuration

### Custom Response URLs

For advanced integrations, you can override response behavior:

```bash
# Direct channel posting
export SLACK_DEFAULT_CHANNEL=C1234567890

# Custom webhook
export RESPONSE_URL=https://hooks.slack.com/commands/...
```

### Response Formatting

#### Slack Block Kit

OYE uses Slack's Block Kit for rich formatting:

```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text", 
        "text": "📊 Daily Time Summary"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "• Today: *7h 30m* [John: 4h, Mary: 3h 30m]"
      }
    },
    {
      "type": "divider"
    }
  ]
}
```

#### Markdown Support

OYE responses support Slack's mrkdwn format:
- `*bold*` for emphasis
- `_italic_` for subtle text
- `~strikethrough~` for cancelled items
- `` `code` `` for inline code
- Lists with `•` bullets

## 🔧 Troubleshooting

### Common Issues

#### 1. Slash Command Not Working

**Symptoms**: `/oye` command not recognized in Slack

**Solutions**:
```bash
# Check if app is installed to workspace
# Verify slash command is configured
# Ensure request URL is correct and accessible
curl -X POST https://your-domain.com/slack/oye
```

#### 2. "Application did not respond" Error

**Symptoms**: Slack shows timeout error

**Causes & Solutions**:
- **App down**: Check health endpoint `curl https://your-domain.com/health`
- **Slow response**: OYE must respond within 3 seconds
- **Invalid URL**: Verify request URL in Slack app config

#### 3. "Unauthorized" Responses

**Symptoms**: Commands work but return authorization errors

**Solutions**:
```bash
# Check token format (should start with xoxb-)
echo $SLACK_BOT_TOKEN

# Verify token in Slack app settings
# Ensure app has correct permissions
```

#### 4. No Response After Initial Acknowledgment

**Symptoms**: Get "Processing..." but no final response

**Debug Steps**:
```bash
# Check application logs
./oye update daily  # Test via CLI

# Verify database connection
./oye --init-db

# Test TimeCamp API
curl -H "Authorization: Bearer $TIMECAMP_API_KEY" \
  https://www.timecamp.com/third_party/api/users/format/json
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
export LOG_LEVEL=debug
./oye
```

### Test Commands

#### Manual API Testing

```bash
# Test health endpoint
curl https://your-domain.com/health

# Test slash command endpoint (requires valid token)
curl -X POST https://your-domain.com/slack/oye \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=YOUR_VERIFICATION_TOKEN" \
  -d "text=help"
```

#### CLI Testing

```bash
# Test update generation
./oye update daily

# Test with Slack context
export SLACK_BOT_TOKEN="xoxb-..."
export CHANNEL_ID="C1234567890"
./oye update daily
```

## 📊 Usage Analytics

### Command Usage Patterns

Track which commands are used most frequently:
- `daily` updates: Most common
- `weekly` reports: Regular team updates  
- `threshold` monitoring: Proactive management
- `sync` operations: Troubleshooting

### Response Time Optimization

**Immediate Response**: < 3 seconds (required by Slack)
**Final Response**: 5-30 seconds depending on data volume

**Optimization Strategies**:
- Database query optimization
- Caching frequently accessed data
- Async processing for heavy operations

## 🔮 Future Enhancements

### Planned Features

#### Interactive Components
- Buttons for common actions
- Dropdown menus for date ranges
- Modal dialogs for configuration

#### Rich Notifications
- Scheduled daily/weekly reports
- Threshold breach alerts
- Project completion notifications

#### Team Management
- User-specific time tracking
- Team performance dashboards
- Project allocation reports

## 📖 Related Documentation

- [Installation Guide](INSTALLATION.md) - Setting up the application
- [API Reference](API_REFERENCE.md) - Technical API details
- [CLI Commands](CLI_COMMANDS.md) - Command-line usage
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions 