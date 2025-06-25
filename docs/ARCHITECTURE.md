# Architecture Overview

This document provides a comprehensive overview of the Observe-Yor-Estimates (OYE) system architecture, design decisions, and component interactions.

## 🏗️ System Architecture

### High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   TimeCamp API  │    │   OYE Service   │    │   Slack API     │
│                 │◄──►│                 │◄──►│                 │
│ • Tasks         │    │ • HTTP Server   │    │ • Slash Cmds    │
│ • Time Entries  │    │ • Cron Jobs     │    │ • Webhooks      │
│ • Users         │    │ • Smart Router  │    │ • Bot Messages  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                               │
                               ▼
                    ┌─────────────────┐
                    │ PostgreSQL DB   │
                    │                 │
                    │ • Tasks         │
                    │ • Time Entries  │
                    │ • Users         │
                    │ • Sync Status   │
                    └─────────────────┘
```

### Monolithic Design

OYE follows a **monolithic architecture** for several key reasons:

1. **Simplicity**: Single deployment unit, easier to manage
2. **Performance**: No network latency between components
3. **Consistency**: Single database transaction scope
4. **Development**: Easier debugging and testing
5. **Resource Efficiency**: Lower overhead than microservices

## 🔧 Core Components

### 1. HTTP Server (`server.go`)

**Purpose**: Handles external HTTP requests from Slack

**Key Features**:
- RESTful endpoints for Slack slash commands
- Request validation and authentication
- Asynchronous job processing via Smart Router
- Graceful shutdown handling

**Endpoints**:
```
POST /slack/oye        # Unified OYE command handler
GET  /health          # Health check endpoint
```

**Request Flow**:
1. Receives Slack slash command
2. Validates request token
3. Routes to Smart Router
4. Returns immediate acknowledgment
5. Processes request asynchronously

### 2. Smart Router (`smart_router.go`)

**Purpose**: Intelligent request routing and job management

**Key Features**:
- Command parsing and classification
- Asynchronous job queuing
- Progress tracking and reporting
- Context-aware responses

**Command Types**:
```go
type CommandType int

const (
    CommandUpdate CommandType = iota    // daily, weekly, monthly
    CommandSync                         // full-sync
    CommandThreshold                    // over X% daily/weekly
    CommandHelp                        // help, empty command
)
```

**Processing Pipeline**:
1. Parse command text
2. Determine command type
3. Queue appropriate job
4. Send immediate response
5. Execute job asynchronously
6. Send progress updates

### 3. Data Synchronization

#### TimeCamp Sync (`sync_*.go`)

**Tasks Sync** (`sync_tasks_to_db.go`):
- Fetches project/task hierarchy from TimeCamp
- Updates local database with latest task information
- Handles project assignments and task relationships

**Time Entries Sync** (`sync_time_entries_to_db.go`):
- Fetches time entries for specified date ranges
- Processes and stores time tracking data
- Handles orphaned entries and data cleanup

**Full Sync** (`full_sync.go`):
- Orchestrates complete data synchronization
- Coordinates tasks and time entries sync
- Provides progress feedback during sync

#### Sync Strategies

1. **Incremental Sync**: Default for regular operations
2. **Full Sync**: Complete data refresh
3. **Date Range Sync**: Targeted sync for specific periods

### 4. Scheduled Tasks (Cron Jobs)

**Implementation**: Uses `github.com/robfig/cron/v3`

**Default Schedules**:
```go
taskSyncSchedule        = "*/5 * * * *"     // Every 5 minutes
timeEntriesSyncSchedule = "*/10 * * * *"    // Every 10 minutes
dailyUpdateSchedule     = "0 6 * * *"       // 6 AM daily
weeklyUpdateSchedule    = "0 8 * * 1"       // 8 AM Monday
monthlyUpdateSchedule   = "0 9 1 * *"       // 9 AM 1st of month
```

**Job Types**:
- **Data Sync Jobs**: Keep database current with TimeCamp
- **Report Jobs**: Generate and send Slack updates
- **Maintenance Jobs**: Clean orphaned data, optimize database

### 5. Database Layer

**Technology**: PostgreSQL with `lib/pq` driver

**Schema Design**:
```sql
-- Core Tables
tasks (id, name, project_id, estimated_time, ...)
time_entries (id, task_id, user_id, duration, date, ...)
users (user_id, username, display_name, ...)

-- Sync Tracking
sync_status (table_name, last_sync, status, ...)
```

**Key Patterns**:
- **Foreign Key Constraints**: Ensure data integrity
- **Indexes**: Optimize query performance
- **Transactions**: Atomic operations for data consistency

### 6. User Management (`user_management.go`)

**Purpose**: Resolve user IDs to human-readable names

**Features**:
- User CRUD operations
- Bulk user import/export
- Display name preferences
- Fallback to user ID if name unavailable

**CLI Commands**:
```bash
./oye add-user <id> <username> <display_name>
./oye list-users
./oye active-users
./oye populate-users
```

## 🔄 Data Flow

### 1. Slack Command Processing

```
Slack User → /oye command
    ↓
HTTP Server → validates request
    ↓
Smart Router → parses command
    ↓
Job Queue → stores async job
    ↓
Worker → processes job
    ↓
Response → sends result to Slack
```

### 2. Data Synchronization Flow

```
Cron Trigger → sync job starts
    ↓
TimeCamp API → fetch data
    ↓
Data Processing → transform/validate
    ↓
Database → store/update records
    ↓
Sync Status → record completion
```

### 3. Report Generation Flow

```
Report Request → determine period/filter
    ↓
Database Query → aggregate time data
    ↓
User Resolution → convert IDs to names
    ↓
Format Response → create Slack blocks
    ↓
Send to Slack → deliver formatted report
```

## 🎯 Design Patterns

### 1. Repository Pattern

**Implementation**: Database access through centralized functions

```go
// Examples
func GetTasksFromDB() ([]Task, error)
func UpsertTimeEntry(entry TimeEntry) error
func GetUserByID(userID int) (*User, error)
```

**Benefits**:
- Consistent database access
- Easy to test with mocks
- Centralized error handling

### 2. Command Pattern

**Implementation**: Slack commands as discrete operations

```go
type CommandHandler interface {
    Execute(req *SlackCommandRequest) error
    GetDescription() string
}
```

**Benefits**:
- Extensible command system
- Consistent error handling
- Easy to add new commands

### 3. Observer Pattern

**Implementation**: Progress tracking for long-running operations

```go
type ProgressReporter interface {
    ReportProgress(stage string, percent int)
    ReportCompletion(result interface{})
    ReportError(err error)
}
```

### 4. Factory Pattern

**Implementation**: Smart Router creates appropriate handlers

```go
func (sr *SmartRouter) CreateHandler(cmdType CommandType) CommandHandler {
    switch cmdType {
    case CommandUpdate:
        return &UpdateHandler{}
    case CommandSync:
        return &SyncHandler{}
    // ...
    }
}
```

## 🔒 Security Considerations

### 1. Request Validation

- **Slack Token Verification**: Validates requests from Slack
- **Input Sanitization**: Prevents injection attacks
- **Rate Limiting**: Prevents abuse (planned)

### 2. API Key Management

- **Environment Variables**: Secure storage of sensitive data
- **No Hardcoded Secrets**: All secrets configurable
- **Minimal Permissions**: APIs use least privilege

### 3. Database Security

- **Connection Encryption**: SSL/TLS for database connections
- **Parameterized Queries**: Prevents SQL injection
- **Limited User Permissions**: Database user has minimal required access

## 📈 Performance Characteristics

### 1. Scalability

**Current Limits**:
- Single instance design
- PostgreSQL connection pool
- Memory usage scales with data volume

**Scaling Strategies**:
- Vertical scaling (more CPU/RAM)
- Database optimization (indexes, queries)
- Caching layer (planned)

### 2. Response Times

**Slack Commands**: < 3 seconds (immediate acknowledgment)
**Report Generation**: 5-30 seconds (depending on data volume)
**Data Sync**: 1-10 minutes (depending on TimeCamp data)

### 3. Resource Usage

**Memory**: ~50-200MB (depending on data cache)
**CPU**: Low (mostly I/O bound)
**Storage**: Grows with time entries (~1MB per 1000 entries)

## 🔮 Future Architecture Considerations

### 1. Microservices Migration

**Potential Split**:
- **API Service**: Handle Slack requests
- **Sync Service**: Manage TimeCamp synchronization  
- **Report Service**: Generate and format reports
- **Notification Service**: Handle Slack messaging

### 2. Event-Driven Architecture

**Benefits**:
- Better decoupling
- Improved scalability
- Enhanced monitoring

**Implementation**:
- Message queue (Redis, RabbitMQ)
- Event sourcing for audit trail
- CQRS for read/write separation

### 3. Caching Layer

**Candidates**:
- Redis for session/job state
- In-memory cache for user lookups
- CDN for static assets (if web UI added)

## 📖 Related Documentation

- [Database Schema](DATABASE.md) - Detailed database design
- [API Reference](API_REFERENCE.md) - Complete API documentation
- [Performance Guide](PERFORMANCE.md) - Optimization strategies
- [Security Guide](SECURITY.md) - Security best practices 