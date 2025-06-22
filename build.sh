#!/bin/bash

# Observe Your Estimates - Build Script
# This script builds the Go application for production deployment

set -e  # Exit on any error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_NAME="observe-yor-estimates"
BUILD_DIR="${SCRIPT_DIR}/build"

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

# Build for current platform
build_local() {
    log_section "Building for Current Platform"
    
    log_info "Building Go binary..."
    go build -o "${BINARY_NAME}" .
    
    if [[ -f "${BINARY_NAME}" ]]; then
        log_success "Binary built successfully: ${BINARY_NAME}"
        
        # Make executable
        chmod +x "${BINARY_NAME}"
        
        # Show binary info
        log_info "Binary size: $(du -h "${BINARY_NAME}" | cut -f1)"
        log_info "Binary location: $(pwd)/${BINARY_NAME}"
    else
        log_error "Failed to build binary"
        exit 1
    fi
}

# Build for Linux (common deployment target)
build_linux() {
    log_section "Building for Linux"
    
    log_info "Building Linux binary..."
    GOOS=linux GOARCH=amd64 go build -o "${BUILD_DIR}/${BINARY_NAME}-linux" .
    
    if [[ -f "${BUILD_DIR}/${BINARY_NAME}-linux" ]]; then
        log_success "Linux binary built successfully: ${BUILD_DIR}/${BINARY_NAME}-linux"
        log_info "Binary size: $(du -h "${BUILD_DIR}/${BINARY_NAME}-linux" | cut -f1)"
    else
        log_error "Failed to build Linux binary"
        exit 1
    fi
}

# Build for multiple platforms
build_all() {
    log_section "Building for Multiple Platforms"
    
    # Create build directory
    mkdir -p "${BUILD_DIR}"
    
    # Linux
    log_info "Building for Linux (amd64)..."
    GOOS=linux GOARCH=amd64 go build -o "${BUILD_DIR}/${BINARY_NAME}-linux-amd64" .
    
    # macOS
    log_info "Building for macOS (amd64)..."
    GOOS=darwin GOARCH=amd64 go build -o "${BUILD_DIR}/${BINARY_NAME}-darwin-amd64" .
    
    # macOS (Apple Silicon)
    log_info "Building for macOS (arm64)..."
    GOOS=darwin GOARCH=arm64 go build -o "${BUILD_DIR}/${BINARY_NAME}-darwin-arm64" .
    
    # Windows
    log_info "Building for Windows (amd64)..."
    GOOS=windows GOARCH=amd64 go build -o "${BUILD_DIR}/${BINARY_NAME}-windows-amd64.exe" .
    
    log_success "All binaries built successfully in ${BUILD_DIR}/"
    ls -la "${BUILD_DIR}/"
}

# Test the binary
test_binary() {
    local binary_path="$1"
    if [[ -z "$binary_path" ]]; then
        binary_path="./${BINARY_NAME}"
    fi
    
    log_section "Testing Binary"
    
    if [[ ! -f "$binary_path" ]]; then
        log_error "Binary not found: $binary_path"
        exit 1
    fi
    
    log_info "Running build test..."
    if "$binary_path" --build-test; then
        log_success "Binary test passed"
    else
        log_error "Binary test failed"
        exit 1
    fi
}

# Clean build artifacts
clean() {
    log_section "Cleaning Build Artifacts"
    
    # Remove local binary
    if [[ -f "${BINARY_NAME}" ]]; then
        rm "${BINARY_NAME}"
        log_info "Removed local binary: ${BINARY_NAME}"
    fi
    
    # Remove build directory
    if [[ -d "${BUILD_DIR}" ]]; then
        rm -rf "${BUILD_DIR}"
        log_info "Removed build directory: ${BUILD_DIR}"
    fi
    
    log_success "Clean completed"
}

# Show help
show_help() {
    echo "Observe Your Estimates - Build Script"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  build, local    - Build for current platform (default)"
    echo "  linux          - Build for Linux (amd64)"
    echo "  all            - Build for all platforms"
    echo "  test           - Test the built binary"
    echo "  clean          - Clean build artifacts"
    echo "  help           - Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0              # Build for current platform"
    echo "  $0 linux        # Build for Linux"
    echo "  $0 all          # Build for all platforms"
    echo "  $0 test         # Test the binary"
    echo ""
}

# Main execution
main() {
    local command="${1:-build}"
    
    case "$command" in
        "build"|"local")
            build_local
            ;;
        "linux")
            mkdir -p "${BUILD_DIR}"
            build_linux
            ;;
        "all")
            build_all
            ;;
        "test")
            if [[ -f "./${BINARY_NAME}" ]]; then
                test_binary "./${BINARY_NAME}"
            elif [[ -f "${BUILD_DIR}/${BINARY_NAME}-linux-amd64" ]]; then
                test_binary "${BUILD_DIR}/${BINARY_NAME}-linux-amd64"
            else
                log_error "No binary found to test. Run './build.sh build' first."
                exit 1
            fi
            ;;
        "clean")
            clean
            ;;
        "help"|"--help"|"-h")
            show_help
            ;;
        *)
            log_error "Unknown command: $command"
            show_help
            exit 1
            ;;
    esac
}

# Check if Go is installed
if ! command -v go &> /dev/null; then
    log_error "Go is not installed or not in PATH"
    exit 1
fi

# Show Go version
log_info "Go version: $(go version)"

# Run main function
main "$@"