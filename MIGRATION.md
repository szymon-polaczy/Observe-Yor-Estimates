# Migration from Netlify Functions to Go HTTP Server

This document outlines the migration from Netlify Functions to a long-running Go HTTP server while still deploying on Netlify.

## What Changed

### Before (Netlify Functions)
- JavaScript functions in `/functions/` directory
- Each request spawned the Go binary as a subprocess
- Cold start latency on every request
- Complex error handling and timeout management
- Limited to 10-second execution time per function

### After (Go HTTP Server)
- Single long-running Go HTTP server
- Direct HTTP request handling
- No cold starts - server stays warm
- Better cron scheduling reliability
- Unlimited execution time for background tasks

## Architecture Changes

### Request Flow Before
```
User Request → Netlify Edge → Netlify Function → Spawn Go Binary → Response
```

### Request Flow After
```
User Request → Netlify Edge → Go HTTP Server → Response
```

## Benefits of the Migration

1. **Performance**: No cold starts, faster response times
2. **Reliability**: Better error handling, no subprocess management
3. **Scheduling**: Persistent cron jobs that don't depend on external triggers
4. **Debugging**: Easier to monitor and debug a single process
5. **Resource Usage**: More efficient memory and CPU usage

## Files Removed/Changed

### Removed Files
- `/functions/slack-command.js` - Replaced by Go HTTP handlers
- `/functions/health.js` - Replaced by Go health endpoint

### Updated Files
- `netlify.toml` - Updated redirects to point to Go server instead of functions
- `deploy.sh` - Modified to start the Go HTTP server after building
- `.env.example` - Cleaned up and removed function-specific configurations

### New Features Added
- Additional API endpoints (`/api/*`) for programmatic access
- Better status monitoring (`/status` endpoint)
- Service information endpoint (`/` root)

## Deployment Process

The deployment process remains the same:
1. Netlify builds the project using `./deploy.sh`
2. The script builds the Go binary
3. The script starts the Go HTTP server as a background process
4. Netlify redirects all traffic to the running server

## Environment Variables

No changes to environment variables - all existing configurations work the same way.

## Endpoint Mapping

| Old Endpoint | New Endpoint | Notes |
|--------------|--------------|-------|
| `/.netlify/functions/health` | `/health` | Direct server endpoint |
| `/.netlify/functions/slack-command` | `/slack/*` | Multiple endpoints now available |
| N/A | `/api/*` | New programmatic API endpoints |
| N/A | `/status` | New detailed status endpoint |
| N/A | `/` | Service information |

## Monitoring and Debugging

### Server Logs
The Go server logs are available in `server.log` in the deployment directory.

### Health Checks
- `/health` - Basic health check
- `/status` - Detailed status including database health

### Manual Testing
```bash
# Test health
curl https://your-site.netlify.app/health

# Test Slack endpoint
curl -X POST https://your-site.netlify.app/slack/daily-update \
  -d "token=your_token&response_url=webhook_url"

# Test API endpoint
curl -X POST https://your-site.netlify.app/api/daily-update
```

## Rollback Plan

If issues arise, you can rollback by:
1. Reverting the `netlify.toml` changes
2. Restoring the `/functions/` directory
3. Redeploying

The original function files are preserved in git history.

## Performance Expectations

- **Response Time**: Expect 50-90% reduction in response time
- **Reliability**: Improved uptime for scheduled tasks
- **Resource Usage**: Lower overall resource consumption
- **Concurrent Requests**: Better handling of simultaneous requests

## Next Steps

After migration:
1. Monitor server logs for any issues
2. Verify all Slack commands work correctly
3. Check that cron jobs are running as expected
4. Consider adding additional monitoring/alerting

## Support

If you encounter issues:
1. Check the server logs in `server.log`
2. Verify environment variables are set correctly
3. Test endpoints manually using curl
4. Check Netlify build logs for deployment issues
