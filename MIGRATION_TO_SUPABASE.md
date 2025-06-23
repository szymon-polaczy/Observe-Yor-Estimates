# Migration Guide: SQLite to Supabase

This guide explains how to migrate your Observe Your Estimates application from SQLite to Supabase (PostgreSQL).

## Why Migrate to Supabase?

- **Netlify Compatibility**: SQLite has limitations in Netlify's serverless environment (read-only filesystem)
- **Persistent Storage**: Supabase provides reliable, persistent database storage
- **Scalability**: PostgreSQL can handle larger datasets and concurrent connections
- **Real-time Features**: Supabase offers additional features like real-time subscriptions

## Prerequisites

1. A Supabase account (free tier available)
2. A new Supabase project created

## Step 1: Set Up Supabase Project

1. Go to [supabase.com](https://supabase.com) and create an account
2. Create a new project
3. Note down your project details:
   - Project URL
   - Database password
   - Project reference ID

## Step 2: Configure Environment Variables

### Option 1: Using DATABASE_URL (Recommended)

Get your PostgreSQL connection string from Supabase:
1. Go to Project Settings → Database
2. Copy the connection string under "Connection string"
3. Set it as your DATABASE_URL environment variable

```bash
DATABASE_URL=postgresql://postgres:[YOUR-PASSWORD]@db.[YOUR-PROJECT-REF].supabase.co:5432/postgres?sslmode=require
```

### Option 2: Using Individual Variables

Alternatively, you can set individual database components:

```bash
DB_HOST=db.[YOUR-PROJECT-REF].supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=[YOUR-PASSWORD]
DB_NAME=postgres
DB_SSLMODE=require
```

## Step 3: Update Your Environment

### For Local Development

Update your `.env` file:

```bash
# Copy .env.example to .env and update with your values
cp .env.example .env
```

Edit `.env` with your Supabase credentials.

### For Netlify Deployment

1. Go to your Netlify site dashboard
2. Navigate to Site settings → Environment variables
3. Add the following variables:
   - `DATABASE_URL`: Your Supabase connection string
   - `TIMECAMP_API_KEY`: Your existing TimeCamp API key
   - `SLACK_BOT_TOKEN`: Your existing Slack bot token (if applicable)

## Step 4: Deploy the Updated Application

The migration has already been implemented in the codebase. The changes include:

- ✅ Replaced SQLite driver with PostgreSQL driver (`github.com/lib/pq`)
- ✅ Updated SQL syntax from SQLite to PostgreSQL
- ✅ Changed `INSERT OR REPLACE` to `INSERT ... ON CONFLICT ... DO UPDATE`
- ✅ Updated table creation scripts for PostgreSQL
- ✅ Modified database connection logic for Supabase

### Build and Deploy

```bash
# Install dependencies
go mod download
go mod tidy

# Build the application
go build -o observe-yor-estimates .

# For Netlify, commit and push your changes
git add .
git commit -m "Migrate from SQLite to Supabase"
git push origin main
```

## Step 5: Initialize Database

After deployment, run the full sync command to populate your Supabase database:

### Via Slack Command (if configured)
```
/full-sync
```

### Via Direct API Call
Make a POST request to your Netlify function endpoint:
```
POST https://your-site.netlify.app/.netlify/functions/slack-command
```

## Step 6: Verify Migration

1. Check that the `/full-sync` command completes successfully
2. Verify that your daily/weekly/monthly updates work correctly
3. Test Slack integrations (if applicable)

## Key Changes Made

### Database Connection
- **Before**: `sql.Open("sqlite3", "./oye.db")`
- **After**: `sql.Open("postgres", connectionString)`

### SQL Syntax Updates
- **Before**: `INSERT OR REPLACE INTO table VALUES (?, ?, ?)`
- **After**: `INSERT INTO table VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET ...`

- **Before**: `INTEGER PRIMARY KEY AUTOINCREMENT`
- **After**: `SERIAL PRIMARY KEY`

- **Before**: `STRING` data type
- **After**: `TEXT` data type

### Schema Checks
- **Before**: `SELECT name FROM sqlite_master WHERE type='table'`
- **After**: `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'table_name')`

## Troubleshooting

### Connection Issues
- Verify your DATABASE_URL is correct
- Check that Supabase project is active
- Ensure SSL mode is set to 'require'

### Permission Issues
- Verify database password is correct
- Check that your Supabase project allows connections

### Migration Errors
- Check application logs for specific error messages
- Verify all environment variables are set correctly
- Ensure TimeCamp API key is still valid

### Function Timeouts
- Supabase connections may take longer than SQLite
- Consider increasing function timeout in netlify.toml

## Support

If you encounter issues during migration:
1. Check the application logs
2. Verify Supabase project status
3. Test database connectivity manually
4. Ensure all environment variables are properly set

## Rollback Plan

If you need to rollback to SQLite:
1. Revert the git commit: `git revert HEAD`
2. Update environment variables to remove Supabase settings
3. Redeploy the application

---

**Note**: This migration provides a more robust, scalable database solution that works reliably in serverless environments like Netlify Functions.