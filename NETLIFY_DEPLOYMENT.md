# Netlify Deployment Troubleshooting Guide

This document provides solutions for the `ENOENT` error when deploying the Observe Your Estimates application to Netlify.

## Problem

The Netlify Function fails with:
```
Error: spawn /var/observe-yor-estimates ENOENT
```

This means the Go binary `observe-yor-estimates` cannot be found in the Netlify environment.

## Root Cause

The issue occurs because:
1. The Go binary is excluded from git by `.gitignore`
2. Netlify Functions need the binary to be available at runtime
3. The binary must be built for the correct architecture (Linux x86-64)

## Solution

### 1. Build Process Fix

The `deploy.sh` script has been updated to:
- Build the binary specifically for Linux when deploying to Netlify
- Ensure the binary is executable
- Test the binary during build

### 2. Netlify Configuration Updates

The `netlify.toml` has been updated with:
- Proper build command that includes binary verification
- Function timeout increased to 30 seconds
- Included files specification for the binary

### 3. Function Path Resolution

The `functions/slack-command.js` has been improved with:
- Better path resolution for the Netlify environment
- Enhanced debugging and error reporting
- Fallback mechanisms

### 4. Debug Tools

Added debugging capabilities:
- `/debug` endpoint to inspect the Netlify environment
- Enhanced error messages in Slack responses
- Build verification scripts

## Deployment Steps

1. **Ensure Environment Variables are Set in Netlify:**
   - `TIMECAMP_API_KEY`
   - `SLACK_WEBHOOK_URL`

2. **Deploy the Application:**
   The build process will automatically:
   - Build the Go binary for Linux
   - Verify the binary works
   - Include it in the deployment

3. **Test the Deployment:**
   - Visit `/health` to check basic function operation
   - Visit `/debug` to inspect the environment
   - Test Slack commands

## Verification

To verify the fix is working:

1. **Check Health Endpoint:**
   ```
   GET https://your-site.netlify.app/health
   ```

2. **Check Debug Endpoint:**
   ```
   GET https://your-site.netlify.app/debug
   ```

3. **Test Slack Command:**
   ```
   POST https://your-site.netlify.app/slack/daily-update
   ```

## File Changes Made

1. **`.gitignore`** - Binary remains excluded (correct for development)
2. **`netlify.toml`** - Updated build process and function configuration
3. **`deploy.sh`** - Enhanced Netlify detection and binary verification
4. **`functions/slack-command.js`** - Improved path resolution and error handling
5. **`functions/debug.js`** - New debugging endpoint
6. **`.netlify-build-hook.sh`** - Build verification script

## Environment Variables

The application requires these environment variables in Netlify:

- `TIMECAMP_API_KEY` - Your TimeCamp API key
- `SLACK_WEBHOOK_URL` - Slack webhook URL for notifications

Optional:
- `SLACK_VERIFICATION_TOKEN` - For additional security

## If Issues Persist

1. Check the Netlify build logs for errors
2. Use the `/debug` endpoint to inspect the runtime environment
3. Verify environment variables are set correctly
4. Check function logs in the Netlify dashboard

The debug endpoint will show:
- Binary existence and permissions
- Environment variables
- File system structure
- Runtime context
