#!/bin/bash

echo "🚀 Starting build process..."

# Create bin directory
mkdir -p bin

# Build Go binary
echo "📦 Building Go binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/observe-yor-estimates .

# Check if binary was created
if [ -f "./bin/observe-yor-estimates" ]; then
    echo "✅ Go binary built successfully"
    ls -la ./bin/observe-yor-estimates
    echo "📊 Binary size: $(du -h ./bin/observe-yor-estimates | cut -f1)"
    
    # Test the binary
    echo "🧪 Testing Go binary..."
    ./bin/observe-yor-estimates --help
    
    if [ $? -eq 0 ]; then
        echo "✅ Go binary test passed"
    else
        echo "❌ Go binary test failed"
        exit 1
    fi
else
    echo "❌ Go binary build failed"
    exit 1
fi

echo "🎉 Build completed successfully!" 