# Netlify Deployment Guide - Timeout-Resistant Architecture

This guide explains how to deploy the new job queue architecture that prevents Slack timeout errors.

## Problem Solved

Previously, Slack slash commands would timeout because:
- Netlify Functions have 10-15 second timeout limits
- Database operations and API calls can take 30-60 seconds
- Large sync operations would fail mid-execution

## New Architecture

The new system uses a **job queue pattern**:
1. Slack command → **Immediate response** (< 1 second)
2. Job queued for background processing
3. Separate job processor handles long-running work
4. Results sent back to Slack when complete

## Deployment Options

### Option 1: Single Netlify Function (Recommended for Simple Cases)

Use the built-in job processor endpoint that runs in the same function:

```bash
# Build command in netlify.toml remains the same
CGO_ENABLED=0 go build -o ./functions/server .
```

Environment variables:
```
JOB_PROCESSOR_URL=/.netlify/functions/server/slack/process-job
```

### Option 2: External Job Processor (Recommended for Production)

Deploy the job processor separately to avoid all timeout issues:

#### 2a. Separate Server/VPS
```bash
# On your server/VPS
./observe-yor-estimates job-processor

# Or with specific port
JOB_PROCESSOR_PORT=8081 ./observe-yor-estimates job-processor
```

Environment variables:
```
JOB_PROCESSOR_URL=https://your-job-server.com/process-job
```

#### 2b. Separate Netlify Function
Create a second Netlify function for job processing:

```bash
# Create functions/job-processor.go
cp observe-yor-estimates functions/job-processor
```

Update netlify.toml:
```toml
[build]
  command = "CGO_ENABLED=0 go build -o ./functions/server . && CGO_ENABLED=0 go build -o ./functions/job-processor ."
  publish = "."
  functions = "functions/"

[build.environment]
  GO_VERSION = "1.22"

[[redirects]]
  from = "/api/*"
  to = "/.netlify/functions/server/:splat"
  status = 200
  force = true

[[redirects]]
  from = "/jobs/*"
  to = "/.netlify/functions/job-processor/:splat"
  status = 200
  force = true
```

Environment variables:
```
JOB_PROCESSOR_URL=/.netlify/functions/job-processor/process-job
```

## Updated Environment Variables

Add these new environment variables to Netlify:

```bash
# Optional: Custom job processor URL
JOB_PROCESSOR_URL=/.netlify/functions/server/slack/process-job

# For standalone job processor
JOB_PROCESSOR_PORT=8081
```

## How It Works

### Before (Problematic)
```
Slack → /daily-update → [30 seconds of work] → Response
                                ↑
                         TIMEOUT HERE! ❌
```

### After (Fixed)
```
Slack → /daily-update → Immediate Response (⏳ Working...)
                            ↓
        Job Queue → Background Processor → Final Response (✅ Done!)
```

## User Experience

### Old Experience:
- User runs `/daily-update`
- Command hangs for 30 seconds
- Times out with "operation_timeout" error ❌

### New Experience:
- User runs `/daily-update`
- Immediate response: "⏳ Your daily update is being prepared..."
- 10-30 seconds later: "✅ Daily update completed!" ✅

## Testing

Test the new architecture:

```bash
# 1. Test immediate response
curl -X POST https://your-app.netlify.app/.netlify/functions/server/slack/update \
  -d "command=/daily-update&response_url=https://hooks.slack.com/test"

# 2. Test job processor
curl -X POST https://your-app.netlify.app/.netlify/functions/server/slack/process-job \
  -H "Content-Type: application/json" \
  -d '{"job_id":"test_123","job_type":"slack_update","parameters":{"period":"daily"},"response_url":"","user_info":"test user"}'

# 3. Test standalone job processor (if using Option 2a)
./observe-yor-estimates job-processor &
curl -X POST http://localhost:8081/process-job \
  -H "Content-Type: application/json" \
  -d '{"job_id":"test_123","job_type":"full_sync","parameters":{},"response_url":"","user_info":"test user"}'
```

## Migration Steps

1. **Deploy new code** to Netlify
2. **Test with a single slash command** to verify no timeouts
3. **Monitor logs** to ensure job processing works
4. **Update Slack app settings** if needed (no changes required)
5. **Inform users** about improved response time

## Monitoring

Monitor job processing with these log patterns:

```bash
# Success patterns
"Successfully queued job"
"Completed processing job"
"Full sync completed successfully"

# Error patterns
"Failed to queue job"
"Job processor returned status"
"Job failed:"
```

## Benefits

✅ **No more timeouts** - Slack commands respond in < 1 second
✅ **Better user experience** - Clear progress updates
✅ **Reliable processing** - Jobs complete even if they take minutes
✅ **Error handling** - Failed jobs send clear error messages
✅ **Scalable** - Can handle multiple concurrent jobs
✅ **Flexible deployment** - Works on Netlify, VPS, or hybrid

This architecture completely eliminates the `operation_timeout` errors you were experiencing! 