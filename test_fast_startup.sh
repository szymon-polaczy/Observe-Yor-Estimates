#!/bin/bash

# Test script to verify fast startup (avoiding Netlify init timeout)
echo "🚀 Testing Fast Startup for Netlify Functions"
echo "=============================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test that server starts in under 5 seconds (Netlify requires fast init)
echo -e "\n${BLUE}Testing server startup time...${NC}"

START_TIME=$(date +%s%N)
./observe-yor-estimates > startup.log 2>&1 &
SERVER_PID=$!

# Wait for server to respond (max 5 seconds)
TIMEOUT=5
COUNTER=0
SUCCESS=false

while [ $COUNTER -lt $TIMEOUT ]; do
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        SUCCESS=true
        break
    fi
    sleep 1
    COUNTER=$((COUNTER + 1))
done

END_TIME=$(date +%s%N)
STARTUP_TIME=$(( (END_TIME - START_TIME) / 1000000 )) # Convert to milliseconds

if [ "$SUCCESS" = true ]; then
    echo -e "${GREEN}✅ Server started successfully in ${STARTUP_TIME}ms${NC}"
    
    if [ $STARTUP_TIME -lt 5000 ]; then
        echo -e "${GREEN}✅ Startup time under 5 seconds - Good for Netlify!${NC}"
    else
        echo -e "${YELLOW}⚠️  Startup time over 5 seconds - May cause Netlify timeout${NC}"
    fi
    
    # Test health endpoint
    echo -e "\n${BLUE}Testing health endpoint...${NC}"
    HEALTH_RESPONSE=$(curl -s http://localhost:8080/health)
    echo "Health response: $HEALTH_RESPONSE"
    
    # Test immediate Slack response
    echo -e "\n${BLUE}Testing immediate Slack response...${NC}"
    SLACK_RESPONSE=$(curl -s -X POST \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "token=test&command=/daily-update&response_url=https://test.com" \
        http://localhost:8080/slack/update)
    
    echo "Slack response: $SLACK_RESPONSE"
    
    if echo "$SLACK_RESPONSE" | grep -q "being prepared"; then
        echo -e "${GREEN}✅ Slack command responds immediately${NC}"
    else
        echo -e "${YELLOW}⚠️  Slack response doesn't contain expected message${NC}"
    fi
    
else
    echo -e "${RED}❌ Server failed to start within $TIMEOUT seconds${NC}"
    echo -e "${RED}Startup time: ${STARTUP_TIME}ms${NC}"
fi

# Show recent logs
echo -e "\n${BLUE}Recent startup logs:${NC}"
tail -10 startup.log

# Cleanup
echo -e "\n${BLUE}Cleaning up...${NC}"
kill $SERVER_PID 2>/dev/null

# Summary
echo -e "\n${BLUE}=============================================="
echo -e "📊 Startup Test Summary"
echo -e "=============================================="

if [ "$SUCCESS" = true ] && [ $STARTUP_TIME -lt 5000 ]; then
    echo -e "${GREEN}✅ PASS: Fast startup suitable for Netlify Functions${NC}"
    echo -e "   Startup time: ${STARTUP_TIME}ms (under 5 second limit)"
    echo -e "   Server responds immediately"
    echo -e "   Ready for deployment!"
elif [ "$SUCCESS" = true ]; then
    echo -e "${YELLOW}⚠️  SLOW: Startup may cause Netlify timeout${NC}"
    echo -e "   Startup time: ${STARTUP_TIME}ms (over 5 second limit)"
    echo -e "   Consider further optimization"
else
    echo -e "${RED}❌ FAIL: Server startup failed${NC}"
    echo -e "   Check logs for errors"
fi

echo -e "\n${BLUE}💡 For Netlify deployment:${NC}"
echo "• Functions must start in under 10 seconds"
echo "• Initialization should be minimal"
echo "• Heavy work should be deferred to background"
echo -e "• Database connections should be lazy-loaded\n" 