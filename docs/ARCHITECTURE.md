# Architecture Overview

This document provides a comprehensive overview of the Observe-Yor-Estimates (OYE) system architecture, design decisions, and component interactions.

## 🏗️ System Architecture

### High-Level Architecture

```mermaid
graph TB
    subgraph "External Services"
        TC[TimeCamp API<br/>📅 Tasks & Time Entries]
        SA[Slack API<br/>💬 Commands & Responses]
    end
    
    subgraph "OYE Application"
        WS[Web Server<br/>🌐 HTTP Endpoints]
        SR[Smart Router<br/>🧠 Command Processing]
        CS[Cron Scheduler<br/>⏰ Automated Jobs]
        JQ[Job Queue<br/>📋 Async Processing]
    end
    
    subgraph "Data Layer"
        DB[(PostgreSQL<br/>🗄️ Database)]
        SS[Sync Status<br/>📊 Tracking]
    end
    
    subgraph "Background Processes"
        TS[Task Sync<br/>📥 Every 5 min]
        TES[Time Entry Sync<br/>📊 Every 10 min]
        RS[Report Scheduler<br/>📧 Daily/Weekly]
    end
    
    TC -->|Fetch Data| SR
    SA -->|Slash Commands| WS
    WS -->|Route Commands| SR
    SR -->|Queue Jobs| JQ
    JQ -->|Process| TS
    JQ -->|Process| TES
    JQ -->|Process| RS
    
    TS -->|Store| DB
    TES -->|Store| DB
    RS -->|Read| DB
    RS -->|Send Reports| SA
    
    CS -->|Trigger| TS
    CS -->|Trigger| TES
    CS -->|Trigger| RS
    
    DB -->|Track| SS
    SS -->|Monitor| CS
```

### 🎯 Monolithic Design Philosophy

OYE follows a **monolithic architecture** for several key reasons:

| Benefit | Description |
|---------|-------------|
| **Simplicity** | Single deployment unit, easier to manage and debug |
| **Performance** | No network latency between components |
| **Consistency** | Single database transaction scope ensures data integrity |
| **Development** | Easier testing, debugging, and feature development |
| **Resource Efficiency** | Lower overhead compared to microservices |

### 📊 Data Flow Visualization

```mermaid
graph LR
    subgraph "Data Sources"
        TC[TimeCamp API<br/>📅 Source Data]
        SC[Slack Commands<br/>💬 User Requests]
    end
    
    subgraph "OYE Processing"
        direction TB
        SR[Smart Router<br/>🧠 Command Logic]
        DS[Data Sync<br/>🔄 ETL Process]
        RA[Report Aggregator<br/>📊 Analytics]
        RF[Response Formatter<br/>🎨 Slack Blocks]
    end
    
    subgraph "Storage"
        DB[(PostgreSQL<br/>🗄️ Normalized Data)]
        SS[Sync Status<br/>📊 Metadata]
    end
    
    subgraph "Outputs"
        SR_OUT[Slack Reports<br/>📈 Formatted]
        UP[User Presence<br/>👥 Active Users]
        AL[Alerts<br/>⚠️ Thresholds]
    end
    
    TC -->|Raw Data| DS
    DS -->|Normalized| DB
    DB -->|Metadata| SS
    
    SC -->|Commands| SR
    SR -->|Trigger| DS
    SR -->|Query| DB
    
    DB -->|Aggregated| RA
    RA -->|Processed| RF
    RF -->|Formatted| SR_OUT
    
    DB -->|Monitor| UP
    DB -->|Check| AL
```

### 🔄 Request Processing Flow

```mermaid
sequenceDiagram
    participant User as 👤 Slack User
    participant Slack as 💬 Slack API
    participant OYE as 🎯 OYE Server
    participant TC as 📅 TimeCamp API
    participant DB as 🗄️ Database
    
    User->>Slack: /oye daily
    Slack->>OYE: POST /slack/oye
    
    Note over OYE: Verify token &<br/>parse command
    
    OYE->>Slack: 200 OK: "⏳ Processing..."
    OYE->>OYE: Queue async job
    
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
    
    Note over OYE: Format report<br/>with Slack blocks
    
    OYE->>Slack: POST response_url
    Slack->>User: 📊 Daily Report
    
    Note over User: Report visible<br/>in Slack channel
```

## 🔧 Core Components

### 1. HTTP Server (`server.go`)

**Purpose**: 🌐 Handles external HTTP requests from Slack

**Key Features**:
- ✅ RESTful endpoints for Slack slash commands
- 🔒 Request validation and authentication
- 🚀 Asynchronous job processing via Smart Router
- 🛑 Graceful shutdown handling

**Endpoints**:
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/slack/oye` | POST | Unified OYE command handler |
| `/health` | GET | Health check endpoint |

**Request Flow**:
1. 📥 Receives Slack slash command
2. 🔍 Validates request token
3. 🔄 Routes to Smart Router
4. ⚡ Returns immediate acknowledgment
5. 🔄 Processes request asynchronously

### 2. Smart Router (`smart_router.go`)

**Purpose**: 🧠 Intelligent request routing and job management

**Key Features**:
- 📝 Command parsing and classification
- 📋 Asynchronous job queuing
- 📊 Progress tracking and reporting
- 🎯 Context-aware responses

**Command Classification**:
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
1. 📝 Parse command text
2. 🔍 Determine command type
3. 📋 Queue appropriate job
4. ⚡ Send immediate response
5. 🔄 Execute job asynchronously
6. 📊 Send progress updates

### 3. Data Synchronization Layer

#### 📅 TimeCamp Integration (`sync_*.go`)

**Tasks Sync** (`sync_tasks_to_db.go`):
- 📥 Fetches project/task hierarchy from TimeCamp
- 🔄 Updates local database with latest task information
- 🔗 Handles project assignments and task relationships

**Time Entries Sync** (`sync_time_entries_to_db.go`):
- 📊 Fetches time entries for specified date ranges
- 🔄 Processes and stores time tracking data
- 🧹 Handles orphaned entries and data cleanup

**Full Sync** (`full_sync.go`):
- 🎯 Orchestrates complete data synchronization
- 📊 Coordinates tasks and time entries sync
- 📈 Provides progress feedback during sync

#### 🔄 Sync Strategies

| Strategy | Use Case | Frequency |
|----------|----------|-----------|
| **Incremental Sync** | Regular operations | Every 5-10 minutes |
| **Full Sync** | Complete data refresh | On-demand or daily |
| **Date Range Sync** | Specific periods | User-triggered |

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
- 📊 **Data Sync Jobs**: Keep database current with TimeCamp
- 📈 **Report Jobs**: Generate and send Slack updates
- 🧹 **Maintenance Jobs**: Clean orphaned data, optimize database

### 5. Database Layer

**Technology**: 🗄️ PostgreSQL with `lib/pq` driver

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
- 🔗 **Foreign Key Constraints**: Ensure data integrity
- 🚀 **Indexes**: Optimize query performance
- 🔄 **Transactions**: Atomic operations for data consistency

### 6. User Management (`user_management.go`)

**Purpose**: 👥 Resolve user IDs to human-readable names

**Features**:
- ✅ User CRUD operations
- 📊 Bulk user import/export
- 🎯 Display name preferences
- 🔄 Fallback to user ID if name unavailable

**CLI Commands**:
```bash
./oye add-user <id> <username> <display_name>
./oye list-users
./oye active-users
./oye populate-users
```

## 🎯 Design Patterns

### 1. Repository Pattern

**Implementation**: Centralized database access

```go
// Examples
func GetTasksFromDB() ([]Task, error)
func UpsertTimeEntry(entry TimeEntry) error
func GetUserByID(userID int) (*User, error)
```

**Benefits**:
- ✅ Consistent database access
- 🧪 Easy to test with mocks
- 🔄 Centralized error handling

### 2. Command Pattern

**Implementation**: Slack commands as discrete operations

```go
type CommandHandler interface {
    Execute(req *SlackCommandRequest) error
    GetDescription() string
}
```

**Benefits**:
- 🎯 Clear command separation
- 🔄 Easy to extend with new commands
- 🧪 Testable command logic

### 3. Observer Pattern

**Implementation**: Event-driven sync status updates

```go
type SyncObserver interface {
    OnSyncStart(syncType string)
    OnSyncProgress(syncType string, progress int)
    OnSyncComplete(syncType string, result SyncResult)
}
```

**Benefits**:
- 📊 Real-time progress tracking
- 🔄 Decoupled status reporting
- 📈 Easy monitoring integration

## 🔧 Configuration Management

### Environment Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `DATABASE_URL` | Database connection | `postgresql://user:pass@host:5432/db` |
| `TIMECAMP_API_KEY` | TimeCamp API access | `abc123def456...` |
| `SLACK_BOT_TOKEN` | Slack bot authentication | `xoxb-123-456-789...` |
| `SLACK_VERIFICATION_TOKEN` | Request verification | `abc123def456...` |

### Schedule Configuration

```bash
# Custom sync schedules (cron format)
TASK_SYNC_SCHEDULE="*/5 * * * *"
TIME_ENTRIES_SYNC_SCHEDULE="*/10 * * * *"
DAILY_UPDATE_SCHEDULE="0 6 * * *"
WEEKLY_UPDATE_SCHEDULE="0 8 * * 1"
MONTHLY_UPDATE_SCHEDULE="0 9 1 * *"
```

## 🚀 Performance Considerations

### Database Optimization

- 📊 **Indexes**: On frequently queried columns
- 🔄 **Connection Pooling**: Efficient resource usage
- 🧹 **Regular Cleanup**: Remove old orphaned data

### API Rate Limiting

- ⏰ **Intelligent Backoff**: Respect TimeCamp rate limits
- 🔄 **Batch Operations**: Reduce API calls
- 📊 **Caching**: Store frequently accessed data

### Memory Management

- 🧹 **Garbage Collection**: Efficient Go runtime usage
- 📊 **Batch Processing**: Handle large datasets efficiently
- 🔄 **Stream Processing**: For large sync operations

## 🛡️ Security Architecture

### Authentication Flow

```mermaid
graph TD
    Request[📥 Slack Request] --> Token{🔍 Valid Token?}
    Token -->|Yes| Process[🔄 Process Command]
    Token -->|No| Reject[❌ 401 Unauthorized]
    Process --> Response[📤 Send Response]
```

### Security Measures

- 🔒 **Token Validation**: All requests verified
- 🔐 **Environment Variables**: Secure credential storage
- 🛡️ **Input Sanitization**: Prevent injection attacks
- 📊 **Audit Logging**: Track all operations

## 🔍 Monitoring & Observability

### Health Checks

- 🩺 **Database Connectivity**: Verify database access
- 🌐 **API Endpoints**: Test external service availability
- 📊 **Sync Status**: Monitor data synchronization health

### Logging Strategy

```go
// Structured logging levels
logger.Info("Command processed", "user", userID, "command", cmd)
logger.Error("Sync failed", "error", err, "syncType", syncType)
logger.Debug("Database query", "query", query, "duration", duration)
```

### Metrics Collection

- 📊 **Command Usage**: Track popular commands
- ⏰ **Response Times**: Monitor performance
- 🔄 **Sync Statistics**: Data processing metrics
- ⚠️ **Error Rates**: Identify issues early

## 🎯 Scalability Considerations

### Horizontal Scaling

- 📋 **Stateless Design**: Easy to add instances
- 🔄 **Database Sharing**: Single source of truth
- 📊 **Load Distribution**: Nginx/HAProxy integration

### Vertical Scaling

- 💾 **Memory Optimization**: Efficient data structures
- 🔄 **CPU Utilization**: Concurrent processing
- 📊 **Database Tuning**: Query optimization

## 📚 Related Documentation

- [Installation Guide](INSTALLATION.md) - Setup instructions
- [API Reference](API_REFERENCE.md) - Endpoint documentation
- [CLI Commands](CLI_COMMANDS.md) - Command reference
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues

---

*This architecture documentation is maintained to reflect the current system design. Last updated: $(date)* 