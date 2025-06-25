# API Reference

This document provides complete reference for the OYE (Observe-Yor-Estimates) HTTP API endpoints, request/response formats, and error handling.

## 📡 Base URL

```
Development: http://localhost:8080
Production:  https://your-domain.com
```

## 🔐 Authentication

All Slack endpoints use token-based authentication via Slack's verification token system.

**Headers Required**:
```
Content-Type: application/x-www-form-urlencoded
```

**Authentication Method**: Slack verification token in request body.

## 📋 Endpoints Overview

| Endpoint | Method | Purpose | Authentication |
|----------|--------|---------|----------------|
| `/health` | GET | Health check | None |
| `/slack/oye` | POST | Unified OYE command handler | Slack Token |

## 🔍 Endpoint Details

### Health Check

**Endpoint**: `GET /health`

**Purpose**: Application health verification

**Authentication**: None required

**Request**:
```http
GET /health HTTP/1.1
Host: your-domain.com
```

**Response**:
```json
{
  "status": "healthy"
}
```

**Status Codes**:
- `200 OK`: Service is healthy
- `500 Internal Server Error`: Service has issues

**Example**:
```bash
curl https://your-domain.com/health
```

---

### Unified OYE Command Handler

**Endpoint**: `POST /slack/oye`

**Purpose**: Processes all Slack slash commands for the OYE system

**Authentication**: Slack verification token required

#### Request Format

**Content-Type**: `application/x-www-form-urlencoded`

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `token` | string | Yes | Slack verification token |
| `team_id` | string | Yes | Slack team ID |
| `team_domain` | string | Yes | Slack team domain |
| `channel_id` | string | Yes | Channel where command was issued |
| `channel_name` | string | Yes | Channel name |
| `user_id` | string | Yes | User who issued command |
| `user_name` | string | Yes | Username |
| `command` | string | Yes | The slash command (e.g., "/oye") |
| `text` | string | No | Command arguments |
| `response_url` | string | Yes | Slack response webhook URL |
| `trigger_id` | string | Yes | Slack trigger ID |

#### Response Format

**Immediate Response** (returned within 3 seconds):

```json
{
  "response_type": "ephemeral",
  "text": "⏳ Processing your request..."
}
```

**Delayed Response** (sent to response_url):

```json
{
  "response_type": "ephemeral",
  "text": "Report content here",
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "Formatted report data"
      }
    }
  ]
}
```

#### Command Types

##### 1. Help Commands

**Input**: Empty text or "help"
```
/oye
/oye help
```

**Response**:
```json
{
  "response_type": "ephemeral",
  "text": "*🎯 OYE Commands*\n\n*Quick Updates:*\n• `/oye` - Daily update\n..."
}
```

##### 2. Update Commands

**Input**: Period specification
```
/oye daily
/oye weekly  
/oye monthly
/oye daily public
```

**Processing**:
1. Immediate acknowledgment sent
2. Data aggregation performed
3. Report formatted with Slack blocks
4. Final response sent to response_url

**Sample Response**:
```json
{
  "response_type": "ephemeral",
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
        "text": "• Today: 7h 30m [John: 4h, Mary: 3h 30m]"
      }
    }
  ]
}
```

##### 3. Sync Commands

**Input**: Sync request
```
/oye sync
/oye full-sync
```

**Processing**:
1. Immediate acknowledgment
2. Background sync initiated
3. Progress updates sent
4. Completion status delivered

##### 4. Threshold Commands

**Input**: Threshold monitoring
```
/oye over 50 daily
/oye over 80 weekly
/oye over 100 monthly
```

**Response Format**:
```json
{
  "response_type": "ephemeral",
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "⚠️ Tasks Over 80% (Weekly)"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "• Task A: 150% (15h / 10h estimated)\n• Task B: 120% (12h / 10h estimated)"
      }
    }
  ]
}
```

## 🎨 Response Types

### Slack Response Types

| Type | Visibility | Description |
|------|------------|-------------|
| `ephemeral` | Private | Only visible to command user |
| `in_channel` | Public | Visible to entire channel |

### Slack Block Types

OYE uses Slack's Block Kit for rich formatting:

#### Header Block
```json
{
  "type": "header",
  "text": {
    "type": "plain_text",
    "text": "Report Title"
  }
}
```

#### Section Block
```json
{
  "type": "section",
  "text": {
    "type": "mrkdwn",
    "text": "Report content with *formatting*"
  }
}
```

#### Divider Block
```json
{
  "type": "divider"
}
```

## ❌ Error Handling

### HTTP Status Codes

| Code | Meaning | Description |
|------|---------|-------------|
| `200 OK` | Success | Request processed successfully |
| `400 Bad Request` | Client Error | Invalid request format |
| `401 Unauthorized` | Auth Error | Invalid or missing Slack token |
| `405 Method Not Allowed` | Client Error | Wrong HTTP method |
| `500 Internal Server Error` | Server Error | Internal processing error |

### Error Response Format

```json
{
  "response_type": "ephemeral",
  "text": "❌ Error: Description of what went wrong"
}
```

### Common Error Scenarios

#### 1. Invalid Slack Token
**Request**: Missing or incorrect verification token

**Response**:
```http
HTTP/1.1 401 Unauthorized
Content-Type: text/plain

Unauthorized
```

#### 2. Malformed Command
**Input**: Invalid command syntax

**Response**:
```json
{
  "response_type": "ephemeral",
  "text": "❌ Invalid command format. Use `/oye help` for usage information."
}
```

#### 3. Database Connection Error
**Scenario**: Database unavailable during processing

**Response**:
```json
{
  "response_type": "ephemeral",
  "text": "❌ Service temporarily unavailable. Please try again later."
}
```

#### 4. TimeCamp API Error
**Scenario**: TimeCamp API unavailable or returns error

**Response**:
```json
{
  "response_type": "ephemeral",
  "text": "❌ Unable to sync with TimeCamp. Data may be outdated."
}
```

## 🔧 Request Examples

### Basic Health Check

```bash
curl -X GET https://your-domain.com/health
```

**Response**:
```json
{
  "status": "healthy"
}
```

### Slack Command (using curl)

```bash
curl -X POST https://your-domain.com/slack/oye \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=your_verification_token" \
  -d "team_id=T1234567890" \
  -d "team_domain=yourteam" \
  -d "channel_id=C1234567890" \
  -d "channel_name=general" \
  -d "user_id=U1234567890" \
  -d "user_name=johndoe" \
  -d "command=/oye" \
  -d "text=daily" \
  -d "response_url=https://hooks.slack.com/commands/1234/5678" \
  -d "trigger_id=123456789.987654321.abcd1234567890"
```

## 📊 Rate Limiting

**Current Implementation**: No explicit rate limiting

**Planned Limits**:
- 10 requests per minute per user
- 100 requests per minute per team
- Exponential backoff for repeated failures

## 🔮 Future API Enhancements

### Planned Endpoints

#### User Management API
```
GET    /api/users           # List all users
POST   /api/users           # Create user
GET    /api/users/{id}      # Get specific user
PUT    /api/users/{id}      # Update user
DELETE /api/users/{id}      # Delete user
```

#### Reports API
```
GET /api/reports/daily    # Get daily report data
GET /api/reports/weekly   # Get weekly report data
GET /api/reports/monthly  # Get monthly report data
```

#### Sync Status API
```
GET  /api/sync/status     # Get current sync status
POST /api/sync/trigger    # Trigger manual sync
```

### Authentication for Future APIs

**Planned**: API Key based authentication
```
Authorization: Bearer your-api-key
```

## 📝 SDK Examples

### JavaScript/Node.js

```javascript
// Example Slack app integration
const { App } = require('@slack/bolt');

const app = new App({
  token: process.env.SLACK_BOT_TOKEN,
  signingSecret: process.env.SLACK_SIGNING_SECRET
});

app.command('/oye', async ({ command, ack, respond }) => {
  await ack();
  
  // Forward to OYE service
  const response = await fetch('https://your-domain.com/slack/oye', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams(command)
  });
  
  const result = await response.json();
  await respond(result);
});
```

### Python

```python
# Example Slack app integration
from slack_bolt import App
import requests

app = App(token=os.environ["SLACK_BOT_TOKEN"])

@app.command("/oye")
def handle_oye_command(ack, command):
    ack()
    
    # Forward to OYE service
    response = requests.post(
        'https://your-domain.com/slack/oye',
        data=command,
        headers={'Content-Type': 'application/x-www-form-urlencoded'}
    )
    
    return response.json()
```

## 📖 Related Documentation

- [Slack Integration Guide](SLACK_INTEGRATION.md) - Detailed Slack setup
- [Error Handling](TROUBLESHOOTING.md) - Troubleshooting common issues
- [Development Guide](DEVELOPMENT.md) - Contributing to the API 