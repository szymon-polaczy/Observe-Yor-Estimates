# Slack Integration Guide

This guide covers the complete setup and usage of OYE's Slack integration, including slash commands, bot configuration, and response formats.

## 🎯 Overview

The OYE Slack integration provides:
- **🎯 Unified `/oye` command** for all time tracking operations
- **📊 Real-time reporting** with rich formatting
- **⚠️ Threshold monitoring** for budget overruns
- **🔒 Public/private responses** based on context
- **📈 Progress updates** for long-running operations

## 🔄 Integration Flow Diagram

```mermaid
sequenceDiagram
    participant User as 👤 Slack User
    participant Slack as 💬 Slack API
    participant OYE as 🎯 OYE Server
    participant TC as 📅 TimeCamp API
    participant DB as 🗄️ Database
    
    User->>Slack: /oye daily
    Slack->>OYE: POST /slack/oye
    
    Note over OYE: 🔍 Verify token &<br/>parse command
    
    OYE->>Slack: 200 OK: "⏳ Processing..."
    OYE->>OYE: 📋 Queue async job
    
    par Background Processing
        OYE->>DB: Query time entries
        OYE->>DB: Query user data
        DB-->>OYE: Return aggregated data
        
        opt If sync needed
            OYE->>TC: Fetch latest data
            TC-->>OYE: Return updates
            OYE->>DB: Store updates
        end
    end
    
    Note over OYE: 🎨 Format report<br/>with Slack blocks
    
    OYE->>Slack: POST response_url
    Slack->>User: 📊 Daily Report
    
    Note over User: Report visible<br/>in Slack channel
```

## 🚀 Initial Setup

### Step 1: Create Slack App

1. 🌐 Visit [Slack API Apps](https://api.slack.com/apps)
2. 📝 Click **"Create New App"**
3. 🎯 Choose **"From scratch"**
4. ⚙️ Enter app details:
   - **App Name**: "OYE Time Tracker"
   - **Workspace**: Select your target workspace
5. ✅ Click **"Create App"**

### Step 2: Configure Bot Permissions

Navigate to **"OAuth & Permissions"** and add these scopes:

#### Bot Token Scopes (Required)
| Scope | Purpose |
|-------|---------|
| `chat:write` | Send messages to channels |
| `commands` | Receive slash command invocations |
| `chat:write.public` | Send messages to channels the app isn't in |

#### Optional Scopes (Future Features)
| Scope | Purpose |
|-------|---------|
| `users:read` | Read user information |
| `channels:read` | Read channel information |

### Step 3: Install App to Workspace

1. 🔧 In **"OAuth & Permissions"**, click **"Install to Workspace"**
2. 📋 Review permissions and click **"Allow"**
3. 🔑 Copy the **Bot User OAuth Token** (starts with `xoxb-`)
4. 💾 Store this as `SLACK_BOT_TOKEN` in your environment

### Step 4: Create Slash Command

1. 💬 Navigate to **"Slash Commands"**
2. ➕ Click **"Create New Command"**
3. ⚙️ Configure the command:

| Field | Value |
|-------|-------|
| **Command** | `/oye` |
| **Request URL** | `https://your-domain.com/slack/oye` |
| **Short Description** | "OYE time tracking commands" |
| **Usage Hint** | `[daily\|weekly\|monthly\|sync\|help] [public]` |

4. 💾 Save the command
5. 🔄 **Reinstall your app** to activate the slash command

### Step 5: Get Verification Token

1. 🔍 Go to **"Basic Information"**
2. 📄 Find **"App Credentials"** section
3. 🔑 Copy the **Verification Token**
4. 💾 Store this as `SLACK_VERIFICATION_TOKEN` in your environment

## 🔧 Environment Configuration

Add these variables to your `.env` file:

```bash
# Slack Bot Configuration
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_VERIFICATION_TOKEN=your_verification_token_here

# Optional: Default channel for direct API usage
SLACK_DEFAULT_CHANNEL=your_channel_id_here
```

## 📝 Command Reference

### Basic Command Structure

```bash
/oye [command] [options]
```

### 📊 Available Commands

#### 1. Help Commands
| Command | Description |
|---------|-------------|
| `/oye` | Show help (same as `/oye help`) |
| `/oye help` | Display command reference |

#### 2. Time Updates
| Command | Description |
|---------|-------------|
| `/oye daily` | Daily time summary (private) |
| `/oye weekly` | Weekly time summary (private) |
| `/oye monthly` | Monthly time summary (private) |
| `/oye daily public` | Daily summary visible to channel |

#### 3. Data Synchronization
| Command | Description |
|---------|-------------|
| `/oye sync` | Quick data sync with TimeCamp |
| `/oye full-sync` | Complete data synchronization |

#### 4. Threshold Monitoring
| Command | Description |
|---------|-------------|
| `/oye over 50 daily` | Tasks over 50% estimate (daily) |
| `/oye over 80 weekly` | Tasks over 80% estimate (weekly) |
| `/oye over 100 monthly` | Tasks over budget (monthly) |

### 🔒 Command Options

#### Public vs Private Responses

**Private (default)**:
- 🔒 Only visible to the user who ran the command
- 👤 Use for personal time tracking queries
- 📊 Personal dashboard and reports

**Public**:
- 👥 Visible to entire channel
- 📢 Add `public` anywhere in the command
- 📈 Use for team updates and reports

**Examples**:
```bash
/oye daily          # 🔒 Private response
/oye daily public   # 👥 Public response
/oye public weekly  # 👥 Public response (order doesn't matter)
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

#### 📊 Daily Update Example

```markdown
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

#### ⚠️ Threshold Monitoring Example

```markdown
⚠️ Tasks Over 80% Estimate (Weekly)

🔴 Critical (Over 100%):
• Bug Fix #123: 150% (12h / 8h estimated)
• Performance Optimization: 120% (9.6h / 8h estimated)

🟡 Warning (80-100%):
• Feature Implementation: 85% (6.8h / 8h estimated)
• Code Review: 90% (7.2h / 8h estimated)

📊 Summary: 4 tasks need attention out of 15 total
```

### 📈 Progress Updates

For long-running operations, OYE sends progress updates:

```markdown
⏳ Syncing with TimeCamp... (Step 1/4)
📥 Fetching tasks and projects...
📥 Fetching time entries...
💾 Updating database...
✅ Sync completed! Updated 1,247 time entries and 156 tasks.
```

## 🔒 Security Configuration

### 🔐 Token Security

**Never expose tokens in**:
- ❌ Public repositories
- ❌ Client-side code
- ❌ Log files
- ❌ Error messages

**Store tokens securely**:
```bash
# ✅ Use environment variables
export SLACK_BOT_TOKEN="xoxb-your-bot-token-here"
export SLACK_VERIFICATION_TOKEN="your-verification-token-here"

# ✅ Or .env file (not committed to git)
echo "SLACK_BOT_TOKEN=xoxb-your-bot-token-here" >> .env
echo "SLACK_VERIFICATION_TOKEN=your-verification-token-here" >> .env
```

### 🔍 Request Verification

OYE automatically verifies all requests using the verification token:

```mermaid
graph TD
    Request[📥 Slack Request] --> Token{🔍 Valid Token?}
    Token -->|✅ Yes| Process[🔄 Process Command]
    Token -->|❌ No| Reject[🚫 401 Unauthorized]
    Process --> Response[📤 Send Response]
```

**Verification Process**:
1. 📥 Slack sends token with each request
2. 🔍 OYE compares with `SLACK_VERIFICATION_TOKEN`
3. ❌ Invalid tokens receive `401 Unauthorized`
4. ⚠️ No token skips verification (not recommended for production)

### 🛡️ Permissions Best Practices

**✅ Minimal Scopes**: Only request necessary permissions
| Permission | Status | Purpose |
|------------|--------|---------|
| `chat:write` | ✅ Required | Send responses |
| `commands` | ✅ Required | Receive slash commands |
| `admin` | ❌ Not needed | Unnecessary privileges |
| `files:write` | ❌ Not needed | Not used by OYE |

## 🎛️ Advanced Configuration

### Custom Response URLs

For advanced integrations, you can configure custom response handling:

```bash
# Custom response URL for specific channels
SLACK_RESPONSE_URL_OVERRIDE=https://hooks.slack.com/services/...

# Custom formatting options
SLACK_RESPONSE_FORMAT=blocks  # or 'text'
SLACK_TIMEZONE=America/New_York
```

### 🔄 Webhook Configuration

For production deployments, configure webhooks properly:

1. **SSL Certificate**: Ensure HTTPS is enabled
2. **Domain Verification**: Use a verified domain
3. **Rate Limiting**: Implement proper rate limiting
4. **Error Handling**: Configure proper error responses

## 🧪 Testing & Validation

### 🔧 Manual Testing

```bash
# Test health endpoint
curl https://your-domain.com/health

# Test Slack endpoint
curl -X POST https://your-domain.com/slack/oye \
  -d "token=your_verification_token" \
  -d "text=help" \
  -d "user_id=U123456" \
  -d "channel_id=C123456"
```

### 🤖 Automated Testing

```bash
# Test script for Slack integration
cat > test_slack.sh << 'EOF'
#!/bin/bash
echo "=== Slack Integration Test ==="

# Test basic commands
echo "Testing help command..."
curl -s -X POST https://your-domain.com/slack/oye \
  -d "token=$SLACK_VERIFICATION_TOKEN" \
  -d "text=help" | grep -q "Commands" && echo "✅ Help OK" || echo "❌ Help failed"

echo "Testing daily command..."
curl -s -X POST https://your-domain.com/slack/oye \
  -d "token=$SLACK_VERIFICATION_TOKEN" \
  -d "text=daily" | grep -q "Processing" && echo "✅ Daily OK" || echo "❌ Daily failed"

echo "=== Test Complete ==="
EOF

chmod +x test_slack.sh
./test_slack.sh
```

## 🔍 Debugging & Troubleshooting

### 🚨 Common Issues

| Issue | Symptom | Solution |
|-------|---------|----------|
| **Command not found** | "Unknown command" | Check app installation & slash command config |
| **No response** | Timeout in Slack | Verify app URL and health endpoint |
| **Permission denied** | Error in response | Check bot scopes and workspace permissions |
| **Invalid token** | 401 Unauthorized | Verify `SLACK_VERIFICATION_TOKEN` |

### 🔧 Debug Commands

```bash
# Check Slack configuration
env | grep SLACK

# Test token validity
curl -X POST https://slack.com/api/auth.test \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN"

# Monitor Slack requests
./oye 2>&1 | grep -i slack
```

### 📋 Slack App Audit Checklist

- [ ] App installed in workspace
- [ ] Slash command `/oye` created
- [ ] Request URL points to correct endpoint
- [ ] Bot token copied to environment
- [ ] Verification token copied to environment
- [ ] Required scopes granted
- [ ] App reinstalled after configuration changes

## 📊 Usage Analytics

### 📈 Tracking Usage

Monitor your Slack integration usage:

```bash
# View command usage patterns
./oye 2>&1 | grep "Command processed" | sort | uniq -c

# Monitor response times
./oye 2>&1 | grep "Response sent" | awk '{print $NF}' | sort -n
```

### 📋 Common Usage Patterns

| Pattern | Description | Frequency |
|---------|-------------|-----------|
| **Daily Check-ins** | `/oye daily` | Every morning |
| **Weekly Reports** | `/oye weekly public` | Monday meetings |
| **Budget Monitoring** | `/oye over 80 weekly` | Friday reviews |
| **Data Refresh** | `/oye sync` | As needed |

## 🚀 Best Practices

### 👥 Team Adoption

1. **Start Small**: Begin with daily personal updates
2. **Gradual Public**: Introduce public commands gradually
3. **Training**: Provide team training on available commands
4. **Feedback**: Collect usage feedback and iterate

### 📊 Reporting Strategy

| Report Type | Frequency | Audience | Command |
|-------------|-----------|----------|---------|
| **Personal Check-in** | Daily | Individual | `/oye daily` |
| **Team Status** | Weekly | Team | `/oye weekly public` |
| **Budget Review** | Weekly | Management | `/oye over 80 weekly public` |
| **Monthly Summary** | Monthly | Stakeholders | `/oye monthly public` |

## 📚 Related Documentation

- [Quick Start Guide](QUICK_START.md) - Setup instructions
- [Installation Guide](INSTALLATION.md) - Detailed setup
- [API Reference](API_REFERENCE.md) - Technical details
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues

---

*This Slack integration guide provides comprehensive setup and usage instructions. For technical details, refer to the [API Reference](API_REFERENCE.md).* 