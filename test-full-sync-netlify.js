#!/usr/bin/env node

// Test script to verify the full-sync command works with Netlify function
console.log('Loading handler...');
const handler = require('./functions/slack-command.js').handler;
console.log('Handler loaded successfully');

// Mock Netlify event for full-sync command
const mockEvent = {
  httpMethod: 'POST',
  path: '/slack/full-sync',
  body: 'token=test-token&team_id=T123&team_domain=test&channel_id=C123&channel_name=general&user_id=U123&user_name=testuser&command=/full-sync&text=&response_url=https://hooks.slack.com/commands/1234/5678&trigger_id=123.456.789'
};

const mockContext = {};

console.log('Testing Netlify function with /full-sync command...');
console.log('Event path:', mockEvent.path);

handler(mockEvent, mockContext)
  .then(result => {
    console.log('\nFunction response:');
    console.log('Status:', result.statusCode);
    
    if (result.statusCode === 400) {
      console.log('Command validation failed - this is expected behavior for the test');
    }
    
    try {
      const parsed = JSON.parse(result.body);
      console.log('Response type:', parsed.response_type || 'not set');
      console.log('Text:', parsed.text);
      if (parsed.blocks && parsed.blocks.length > 0) {
        console.log('Has blocks:', parsed.blocks.length);
      }
    } catch (error) {
      console.log('Raw body:', result.body);
    }
  })
  .catch(error => {
    console.error('Function error:', error);
  });
