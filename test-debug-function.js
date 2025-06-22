#!/usr/bin/env node

// Test the debug function to see what the Netlify environment looks like
const handler = require('./functions/debug.js').handler;

console.log('Testing debug function...');

const mockEvent = {
  httpMethod: 'GET',
  path: '/debug',
};

const mockContext = {
  functionName: 'debug',
  functionVersion: '1',
  memoryLimitInMB: 128,
  getRemainingTimeInMillis: () => 5000,
};

handler(mockEvent, mockContext)
  .then(result => {
    console.log('\nDebug function response:');
    console.log('Status:', result.statusCode);
    
    try {
      const parsed = JSON.parse(result.body);
      console.log('\nEnvironment Debug Info:');
      console.log(JSON.stringify(parsed, null, 2));
    } catch (error) {
      console.log('Raw body:', result.body);
    }
  })
  .catch(error => {
    console.error('Debug function error:', error);
  });
