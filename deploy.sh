#!/bin/bash

# Observe Your Estimates - Netlify Deployment Script
# This script handles Netlify deployment including database management and versioning

set -e  # Exit on any error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATABASE_VERSION="1.0.1"  # Increment this to force database recreation
VERSION_FILE="${SCRIPT_DIR}/.db_version"
DEFAULT_DB_PATH="${SCRIPT_DIR}/oye.db"
BINARY_NAME="observe-yor-estimates"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# Check if running on Netlify
is_netlify() {
    [[ -n "${NETLIFY:-}" ]] || [[ -n "${DEPLOY_URL:-}" ]] || [[ -n "${NETLIFY_BUILD_BASE:-}" ]]
}

# Get database path from environment or use default
get_db_path() {
    echo "${DATABASE_PATH:-$DEFAULT_DB_PATH}"
}

# Check if database is accessible (for PostgreSQL/Supabase)
database_exists() {
    # For PostgreSQL, we check connectivity rather than file existence
    if [[ -f "$BINARY_NAME" ]]; then
        ./"$BINARY_NAME" --db-check >/dev/null 2>&1
    else
        # If binary doesn't exist yet, assume database needs setup
        return 1
    fi
}

# Check database version
check_database_version() {
    local current_version
    if [[ -f "$VERSION_FILE" ]]; then
        current_version=$(cat "$VERSION_FILE")
    else
        current_version="0.0.0"
    fi
    
    if [[ "$current_version" != "$DATABASE_VERSION" ]]; then
        log_info "Database version mismatch: current=$current_version, required=$DATABASE_VERSION"
        return 1
    fi
    return 0
}

# Update database version file
update_database_version() {
    echo "$DATABASE_VERSION" > "$VERSION_FILE"
    log_info "Updated database version to $DATABASE_VERSION"
}

# Clear database version (for PostgreSQL, we don't delete the database itself)
remove_database() {
    # For PostgreSQL/Supabase, we only remove the version file
    # The actual database clearing would be done through SQL commands if needed
    if [[ -f "$VERSION_FILE" ]]; then
        rm -f "$VERSION_FILE"
        log_info "Removed version file - database will be resynced"
    fi
    log_info "Database version reset - full sync will be performed"
}

# Check if database has data
check_database_data() {
    local db_path=$(get_db_path)
    
    if ! database_exists; then
        log_warning "Database file does not exist"
        return 1
    fi
    
    # Check if Go binary exists
    if [[ ! -f "$BINARY_NAME" ]]; then
        log_error "Binary $BINARY_NAME not found. Please build the project first."
        return 1
    fi
    
    # Create a simple Go program to check database contents
    log_info "Checking database for existing data..."
    
    # For PostgreSQL/Supabase, we'll test database connectivity through the Go application
    # since we don't have direct database access without credentials
    log_info "Using Go application to check PostgreSQL database connectivity..."
    
    # Try to connect to the database and check for data
    if ./"$BINARY_NAME" --db-check 2>/dev/null; then
        log_info "Database is accessible and contains data"
        return 0
    else
        log_info "Database needs population or is not accessible"
        return 1
    fi
}

# Build the Go application
build_application() {
    log_section "Building Application"
    
    # Ensure we have Go
    if ! command -v go >/dev/null 2>&1; then
        log_error "Go is not installed or not in PATH"
        return 1
    fi
    
    # Install dependencies
    log_info "Installing Go dependencies..."
    go mod download
    go mod tidy
    
    # Build the application
    log_info "Building application..."
    
    # For Netlify deployments, ensure we build for Linux
    if is_netlify; then
        log_info "Netlify environment detected - building for Linux"
        GOOS=linux GOARCH=amd64 go build -o "$BINARY_NAME" .
    else
        # For local development, build for current platform
        go build -o "$BINARY_NAME" .
    fi
    
    if [[ -f "$BINARY_NAME" ]]; then
        log_success "Application built successfully: $BINARY_NAME"
        # Make it executable
        chmod +x "$BINARY_NAME"
        
        # Show binary info for debugging
        if command -v file >/dev/null 2>&1; then
            log_info "Binary info: $(file "$BINARY_NAME")"
        fi
    else
        log_error "Failed to build application"
        return 1
    fi
}

# Validate environment variables
validate_environment() {
    log_section "Validating Environment"
    
    # For Netlify deployments, we don't require API keys at build time
    # They're only needed for runtime operations
    if is_netlify; then
        log_info "Netlify environment detected - skipping API key validation"
        log_info "API keys will be validated at runtime when functions are called"
    else
        # For non-Netlify deployments, validate API keys
        local required_vars=(
            "TIMECAMP_API_KEY"
            "SLACK_WEBHOOK_URL"
        )
        
        local missing_vars=()
        
        for var in "${required_vars[@]}"; do
            if [[ -z "${!var:-}" ]]; then
                missing_vars+=("$var")
            else
                log_success "$var is set"
            fi
        done
        
        if [[ ${#missing_vars[@]} -gt 0 ]]; then
            log_error "Missing required environment variables:"
            for var in "${missing_vars[@]}"; do
                log_error "  - $var"
            done
            log_error "For local development, these are required. For Netlify, set them in the site settings."
            return 1
        fi
    fi
    
    # Optional variables with defaults
    local optional_vars=(
        "DATABASE_PATH:$DEFAULT_DB_PATH"
        "TIMECAMP_API_URL:https://app.timecamp.com/third_party/api"
        "SLACK_API_URL:https://slack.com/api/apps.connections.open"
    )
    
    for var_default in "${optional_vars[@]}"; do
        var="${var_default%%:*}"
        default="${var_default#*:}"
        if [[ -z "${!var:-}" ]]; then
            export "$var"="$default"
            log_info "$var not set, using default: $default"
        else
            log_success "$var is set: ${!var}"
        fi
    done
}

# Perform full synchronization
full_sync() {
    log_section "Performing Full Synchronization"
    
    # For Netlify deployments, skip full sync at build time since we don't have API keys
    if is_netlify; then
        log_info "Netlify environment detected - skipping full sync at build time"
        log_info "Database will be initialized when functions are first called"
        
        # Create an empty database with the right structure
        log_info "Initializing empty database structure..."
        if ./"$BINARY_NAME" --build-test >/dev/null 2>&1; then
            log_success "Binary is working, database will be initialized on first use"
        else
            log_error "Binary test failed"
            return 1
        fi
        return 0
    fi
    
    log_info "Starting full synchronization with TimeCamp..."
    
    # Run full sync command
    if ./"$BINARY_NAME" full-sync; then
        log_success "Full synchronization completed successfully"
    else
        log_error "Full synchronization failed"
        return 1
    fi
}

# Test database connectivity and basic operations
test_database() {
    log_section "Testing Database"
    
    # For Netlify, skip database tests since we don't have API keys at build time
    if is_netlify; then
        log_info "Netlify environment detected - skipping database tests at build time"
        log_info "Database connectivity will be tested when functions are called"
        return 0
    fi
    
    log_info "Testing database connectivity..."
    
    # Try a simple daily update command to test the database
    if ./"$BINARY_NAME" daily-update >/dev/null 2>&1; then
        log_success "Database connectivity test passed"
    else
        log_warning "Database connectivity test failed (this may be normal for new deployments)"
    fi
}

# Setup for Netlify Functions
setup_netlify() {
    log_section "Netlify Setup"
    
    if is_netlify; then
        log_info "Netlify environment detected"
        
        # Ensure binary is in the right location for Functions
        if [[ -f "$BINARY_NAME" ]]; then
            log_info "Binary found: $BINARY_NAME"
            # Make sure it's executable
            chmod +x "$BINARY_NAME"
            
            # Show binary info for debugging
            if command -v file >/dev/null 2>&1; then
                log_info "Binary info: $(file "$BINARY_NAME")"
            fi
            
            # Create a proper SQLite database file for Netlify deployment
            # This ensures the database file exists with proper structure in the deployment package
            local db_path=$(get_db_path)
            if [[ ! -f "$db_path" ]]; then
                log_info "Creating SQLite database file for Netlify deployment: $db_path"
                # Use the new --init-db command to create a proper SQLite database with tables
                export DATABASE_PATH="$db_path"
                if ./"$BINARY_NAME" --init-db 2>/dev/null; then
                    log_success "Database file created and initialized: $db_path"
                else
                    log_warning "Failed to initialize database with --init-db, trying build test"
                    # Fallback to build test and create empty file
                    if ./"$BINARY_NAME" --build-test 2>/dev/null; then
                        touch "$db_path"
                        log_info "Created empty database file as fallback: $db_path"
                    else
                        log_error "Both database initialization methods failed"
                        return 1
                    fi
                fi
            else
                log_info "Database file already exists: $db_path"
            fi
            
            # Check if binary can execute basic commands
            if ./"$BINARY_NAME" --build-test >/dev/null 2>&1; then
                log_success "Binary test passed - ready for Netlify Functions"
            else
                log_warning "Binary test failed - functions may not work properly"
            fi
        else
            log_error "Binary not found - Netlify Functions will fail"
            return 1
        fi
        
        log_info "Functions are available in ./functions/ directory"
        log_success "Netlify setup complete"
    else
        log_info "Not running on Netlify - skipping Netlify-specific setup"
    fi
}

# Main deployment logic
main() {
    log_section "Observe Your Estimates - Deployment Script"
    log_info "Database Version: $DATABASE_VERSION"
    log_info "Working Directory: $SCRIPT_DIR"
    
    # Step 1: Validate environment
    if ! validate_environment; then
        log_error "Environment validation failed"
        exit 1
    fi
    
    # Step 2: Build application
    if ! build_application; then
        log_error "Application build failed"
        exit 1
    fi
    
    # Step 3: Check database version and decide on recreation
    local needs_db_recreation=false
    local needs_full_sync=false
    
    if is_netlify; then
        log_info "Netlify environment - skipping database checks at build time"
        needs_db_recreation=false
        needs_full_sync=false
    elif ! check_database_version; then
        log_warning "Database version check failed - recreation required"
        needs_db_recreation=true
        needs_full_sync=true
    elif ! database_exists; then
        log_info "Database does not exist - will create and populate"
        needs_full_sync=true
    elif ! check_database_data; then
        log_info "Database exists but appears empty - will populate"
        needs_full_sync=true
    else
        log_success "Database exists, is current version, and has data"
    fi
    
    # Step 4: Handle database recreation if needed
    if [[ "$needs_db_recreation" == "true" ]]; then
        log_section "Database Recreation"
        log_warning "Removing existing database due to version change"
        remove_database
    fi
    
    # Step 5: Perform full sync if needed
    if [[ "$needs_full_sync" == "true" ]]; then
        if ! full_sync; then
            log_error "Failed to populate database"
            exit 1
        fi
        
        # Update version after successful sync
        update_database_version
    fi
    
    # Step 6: Test database functionality
    test_database
    
    # Step 7: Setup Netlify
    setup_netlify
    
    # Final status
    log_section "Deployment Complete"
    log_success "Application deployed successfully!"
    log_info "Binary: $BINARY_NAME"
    
    if is_netlify; then
        log_info "Netlify deployment complete"
        log_info "Functions deployed: health, slack-command"
        log_info "Health check: /.netlify/functions/health"
        log_info "Slack commands: /slack/{daily-update,weekly-update,monthly-update}"
        log_info ""
        log_info "Note: Set TIMECAMP_API_KEY and SLACK_WEBHOOK_URL in Netlify site settings"
        log_info "for the functions to work properly at runtime."
    else
        log_info "Database: $(get_db_path)"
        log_info "Version: $DATABASE_VERSION"
        log_info "To start the application in daemon mode:"
        log_info "  ./$BINARY_NAME"
        log_info ""
        log_info "To run manual commands:"
        log_info "  ./$BINARY_NAME daily-update"
        log_info "  ./$BINARY_NAME weekly-update"
        log_info "  ./$BINARY_NAME monthly-update"
    fi
}

# Handle script arguments
case "${1:-}" in
    "--help"|"-h"|"help")
        echo "Observe Your Estimates - Deployment Script"
        echo ""
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  (no command)  - Full deployment process"
        echo "  --force-sync  - Force database recreation and full sync"
        echo "  --build-only  - Only build the application"
        echo "  --test        - Test database and environment"
        echo "  --help        - Show this help message"
        echo ""
        echo "Environment Variables:"
        echo "  DATABASE_VERSION  - Set to force database recreation (default: $DATABASE_VERSION)"
        echo "  DATABASE_PATH     - Custom database path (default: $DEFAULT_DB_PATH)"
        echo "  TIMECAMP_API_KEY  - Required: TimeCamp API key"
        echo "  SLACK_WEBHOOK_URL - Required: Slack webhook URL"
        echo ""
        exit 0
        ;;
    "--force-sync")
        log_info "Force sync requested - will recreate database"
        export FORCE_SYNC=true
        remove_database
        ;;
    "--build-only")
        log_section "Build Only Mode"
        validate_environment
        build_application
        exit $?
        ;;
    "--test")
        log_section "Test Mode"
        validate_environment
        if database_exists; then
            test_database
        else
            log_warning "Database does not exist"
        fi
        exit 0
        ;;
    "")
        # Normal deployment
        ;;
    *)
        log_error "Unknown command: $1"
        log_info "Use --help for usage information"
        exit 1
        ;;
esac

# Run main deployment if no specific command was given
main
