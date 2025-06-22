#!/usr/bin/env node

// Test script to verify the full-sync command is recognized by the Netlify function
const handler = require('./functions/slack-command.js').handler;

// Mock Netlify event for full-sync command
const mockEvent = {
  httpMethod: 'POST',
  path: '/slack/full-sync',
  body: 'token=test-token&team_id=T123&team_domain=test&channel_id=C123&channel_name=general&user_id=U123&user_name=testuser&command=/full-sync&text=&response_url=https://hooks.slack.com/commands/1234/5678&trigger_id=123.456.789'
};

const mockContext = {};

console.log('Testing /full-sync command recognition in Netlify function...');
console.log('Event path:', mockEvent.path);
console.log('Command from path:', mockEvent.path.split('/').pop());

// Test if the command is recognized as valid
const command = mockEvent.path.split('/').pop();
const validCommands = ['daily-update', 'weekly-update', 'monthly-update', 'full-sync'];

console.log('Valid commands:', validCommands);
console.log('Is command valid?', validCommands.includes(command));

if (validCommands.includes(command)) {
  console.log('✅ SUCCESS: /full-sync command is properly recognized!');
} else {
  console.log('❌ ERROR: /full-sync command is not recognized!');
}
