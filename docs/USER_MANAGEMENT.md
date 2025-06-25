# User Management for OYE Time Tracker

This document explains how to manage users in the OYE Time Tracker system to fix the user display issue where usernames were showing as "user[ID]" instead of actual names.

## What Changed

- **Added a `users` table** to store user information (user_id, username, display_name)
- **Updated display logic** in three key places to show proper usernames instead of "user[ID]"
- **Added fallback handling** - if a user is not found in the users table, it falls back to "user[ID]" format

## Commands

### View Active User IDs
See which user IDs are currently in your time entries:
```bash
./oye-time-tracker active-users
```

### Populate Sample Users
Add some sample users to get started:
```bash
./oye-time-tracker populate-users
```

### List All Users
View all users currently in the database:
```bash
./oye-time-tracker list-users
```

### Add Individual User
Add a specific user manually:
```bash
./oye-time-tracker add-user <user_id> <username> <display_name>
```

Example:
```bash
./oye-time-tracker add-user 1820471 "john.doe" "John Doe"
```

## Database Schema

The new `users` table has the following structure:

```sql
CREATE TABLE users (
    user_id INTEGER PRIMARY KEY,
    username TEXT NOT NULL,
    display_name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## How It Works

1. **User Lookup**: When displaying user contributions, the system now looks up usernames from the `users` table
2. **Fallback**: If a user is not found, it falls back to the old "user[ID]" format
3. **Priority**: Display name is preferred over username, with user ID as final fallback

## Typical Workflow

1. **Check active users**: Run `./oye-time-tracker active-users` to see which user IDs are in your time entries
2. **Add users**: Use the commands above to add proper usernames for those IDs
3. **Test**: Generate a report to see usernames instead of "user[ID]"

## Integration with External Systems

If you're importing user data from another system (like TimeCamp, Slack, or HR systems), you can:

1. Use the `UpsertUser()` function in your import scripts
2. Call the user management functions programmatically
3. Bulk import via SQL if needed

## Example Output

**Before (with user IDs):**
```
• Today: 5h 30m [user1820471: 3h 30m, user1721068: 2h 0m]
```

**After (with proper names):**
```
• Today: 5h 30m [John Smith: 3h 30m, Mary Johnson: 2h 0m]
```

## Troubleshooting

- **Database connection issues**: Make sure your database environment variables are set correctly
- **Users not showing**: Verify users exist with `./oye-time-tracker list-users`
- **Still seeing user IDs**: The user might not be in the users table or there might be a database connection error (check logs)