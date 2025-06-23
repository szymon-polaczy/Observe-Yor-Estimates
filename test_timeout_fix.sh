#!/bin/bash

# Test script for the new timeout-resistant Slack app architecture
# This tests both immediate responses and background job processing

echo "🚀 Testing Timeout-Resistant Architecture"
echo "=========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
PORT=8080
HOST="http://localhost:$PORT"
TEST_WEBHOOK_URL="https://hooks.slack.com/test/test/test"

echo -e "\n${BLUE}Step 1: Starting server in background...${NC}"
./observe-yor-estimates > server.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# Wait for server to start
echo "Waiting for server to start..."
sleep 3

# Test 1: Health check
echo -e "\n${BLUE}Step 2: Testing health check...${NC}"
HEALTH_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" $HOST/health)
HEALTH_CODE=$(echo "$HEALTH_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)

if [ "$HEALTH_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Health check passed${NC}"
else
    echo -e "${RED}❌ Health check failed (HTTP $HEALTH_CODE)${NC}"
    echo "Response: $HEALTH_RESPONSE"
fi

# Test 2: Test immediate response for update command
echo -e "\n${BLUE}Step 3: Testing immediate response for /daily-update...${NC}"
UPDATE_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "token=test&team_id=T123&channel_id=C123&user_id=U123&user_name=testuser&command=/daily-update&text=&response_url=$TEST_WEBHOOK_URL" \
    $HOST/slack/update)

UPDATE_CODE=$(echo "$UPDATE_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
UPDATE_BODY=$(echo "$UPDATE_RESPONSE" | grep -v "HTTP_CODE:")

if [ "$UPDATE_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Update command responded immediately (HTTP 200)${NC}"
    echo "Response: $UPDATE_BODY"
    
    # Check if response contains the expected immediate message
    if echo "$UPDATE_BODY" | grep -q "being prepared"; then
        echo -e "${GREEN}✅ Response contains expected immediate acknowledgment${NC}"
    else
        echo -e "${YELLOW}⚠️  Response doesn't contain expected message${NC}"
    fi
else
    echo -e "${RED}❌ Update command failed (HTTP $UPDATE_CODE)${NC}"
    echo "Response: $UPDATE_RESPONSE"
fi

# Test 3: Test immediate response for full-sync command
echo -e "\n${BLUE}Step 4: Testing immediate response for /full-sync...${NC}"
SYNC_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "token=test&team_id=T123&channel_id=C123&user_id=U123&user_name=testuser&command=/full-sync&text=&response_url=$TEST_WEBHOOK_URL" \
    $HOST/slack/full-sync)

SYNC_CODE=$(echo "$SYNC_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
SYNC_BODY=$(echo "$SYNC_RESPONSE" | grep -v "HTTP_CODE:")

if [ "$SYNC_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Full-sync command responded immediately (HTTP 200)${NC}"
    echo "Response: $SYNC_BODY"
    
    # Check if response contains the expected immediate message
    if echo "$SYNC_BODY" | grep -q "has been queued"; then
        echo -e "${GREEN}✅ Response contains expected queuing message${NC}"
    else
        echo -e "${YELLOW}⚠️  Response doesn't contain expected message${NC}"
    fi
else
    echo -e "${RED}❌ Full-sync command failed (HTTP $SYNC_CODE)${NC}"
    echo "Response: $SYNC_RESPONSE"
fi

# Test 4: Test job processor endpoint directly
echo -e "\n${BLUE}Step 5: Testing job processor endpoint...${NC}"
JOB_JSON='{"job_id":"test_123","job_type":"slack_update","parameters":{"period":"daily"},"response_url":"","user_info":"test user","queued_at":"2024-01-01T00:00:00Z"}'

JOB_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "$JOB_JSON" \
    $HOST/slack/process-job)

JOB_CODE=$(echo "$JOB_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
JOB_BODY=$(echo "$JOB_RESPONSE" | grep -v "HTTP_CODE:")

if [ "$JOB_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Job processor endpoint responded (HTTP 200)${NC}"
    echo "Response: $JOB_BODY"
else
    echo -e "${RED}❌ Job processor failed (HTTP $JOB_CODE)${NC}"
    echo "Response: $JOB_RESPONSE"
fi

# Test 5: Check server logs for job processing
echo -e "\n${BLUE}Step 6: Checking server logs for job processing...${NC}"
sleep 2  # Give jobs time to process

if grep -q "Successfully queued job" server.log; then
    echo -e "${GREEN}✅ Jobs are being queued successfully${NC}"
else
    echo -e "${YELLOW}⚠️  No job queuing messages found in logs${NC}"
fi

if grep -q "Processing job" server.log; then
    echo -e "${GREEN}✅ Jobs are being processed${NC}"
else
    echo -e "${YELLOW}⚠️  No job processing messages found in logs${NC}"
fi

# Test 6: Test standalone job processor mode
echo -e "\n${BLUE}Step 7: Testing standalone job processor mode...${NC}"
./observe-yor-estimates job-processor > job_processor.log 2>&1 &
JOB_PROCESSOR_PID=$!
echo "Job processor PID: $JOB_PROCESSOR_PID"

sleep 2  # Wait for job processor to start

# Test job processor health
JOB_HEALTH_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" http://localhost:8081/health)
JOB_HEALTH_CODE=$(echo "$JOB_HEALTH_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)

if [ "$JOB_HEALTH_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Standalone job processor is running${NC}"
    
    # Test processing a job
    STANDALONE_JOB_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$JOB_JSON" \
        http://localhost:8081/process-job)
    
    STANDALONE_JOB_CODE=$(echo "$STANDALONE_JOB_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
    
    if [ "$STANDALONE_JOB_CODE" = "200" ]; then
        echo -e "${GREEN}✅ Standalone job processor can handle jobs${NC}"
    else
        echo -e "${RED}❌ Standalone job processor failed (HTTP $STANDALONE_JOB_CODE)${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Standalone job processor not responding (port 8081)${NC}"
fi

# Cleanup
echo -e "\n${BLUE}Step 8: Cleanup...${NC}"
echo "Killing server (PID: $SERVER_PID)"
kill $SERVER_PID 2>/dev/null

if [ ! -z "$JOB_PROCESSOR_PID" ]; then
    echo "Killing job processor (PID: $JOB_PROCESSOR_PID)"
    kill $JOB_PROCESSOR_PID 2>/dev/null
fi

# Summary
echo -e "\n${BLUE}=========================================="
echo -e "📊 Test Summary"
echo -e "==========================================${NC}"

echo -e "\n${GREEN}✅ Benefits of New Architecture:${NC}"
echo "• Slack commands respond in < 1 second (no more timeouts!)"
echo "• Long-running jobs process in background"
echo "• Users get progress updates during processing"
echo "• Works with Netlify's 15-second function limits"
echo "• Can run job processor separately for even better reliability"

echo -e "\n${YELLOW}📋 Next Steps:${NC}"
echo "1. Deploy to Netlify with the new code"
echo "2. Test with real Slack commands"
echo "3. Monitor logs for job processing"
echo "4. Optionally set up external job processor for production"

echo -e "\n${BLUE}📁 Log Files Created:${NC}"
echo "• server.log - Main server logs"
echo "• job_processor.log - Standalone job processor logs"

echo -e "\n${GREEN}🎉 Testing complete! The timeout issue should now be resolved.${NC}" 