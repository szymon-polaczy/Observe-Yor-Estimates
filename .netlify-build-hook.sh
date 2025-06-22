#!/bin/bash

# Netlify build hook to ensure binary is properly set up
echo "=== Netlify Build Hook - Setting up binary ==="

# Make sure the binary exists and is executable
if [ -f "observe-yor-estimates" ]; then
    echo "✓ Binary found: observe-yor-estimates"
    chmod +x observe-yor-estimates
    ls -la observe-yor-estimates
    
    # Test that the binary works
    if ./observe-yor-estimates --build-test; then
        echo "✓ Binary test passed"
    else
        echo "✗ Binary test failed"
        exit 1
    fi
else
    echo "✗ Binary not found!"
    echo "Available files:"
    ls -la
    exit 1
fi

echo "=== Binary setup complete ==="
