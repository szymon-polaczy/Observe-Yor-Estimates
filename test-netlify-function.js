#!/usr/bin/env node

// Test script to simulate how Netlify would call the slack-command function
const handler = require('./functions/slack-command.js').handler;

// Mock Netlify event for daily-update command
const mockEvent = {
  httpMethod: 'POST',
  path: '/slack/daily-update',
  body: 'token=test-token&team_id=T123&team_domain=test&channel_id=C123&channel_name=general&user_id=U123&user_name=testuser&command=/daily-update&text=&response_url=https://hooks.slack.com/commands/1234/5678&trigger_id=123.456.789'
};

const mockContext = {};

console.log('Testing Netlify function with mock Slack slash command...');
console.log('Event path:', mockEvent.path);
console.log('Event body:', mockEvent.body);

handler(mockEvent, mockContext)
  .then(result => {
    console.log('\nFunction response:');
    console.log('Status:', result.statusCode);
    console.log('Body:', result.body);
    
    // Give some time for the background process to run
    console.log('\nWaiting for background process to complete...');
    setTimeout(() => {
      console.log('Test completed. Check the logs above for the Go binary execution.');
      process.exit(0);
    }, 5000);
  })
  .catch(error => {
    console.error('Function error:', error);
    process.exit(1);
  });
