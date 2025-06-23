const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

exports.handler = async (event, context) => {
  try {
    console.log('Debug function called');
    
    const debugInfo = {
      timestamp: new Date().toISOString(),
      event: {
        httpMethod: event.httpMethod,
        headers: event.headers,
        queryStringParameters: event.queryStringParameters,
        hasBody: !!event.body,
        bodyLength: event.body?.length || 0
      },
      context: {
        functionName: context.functionName,
        functionVersion: context.functionVersion,
        remainingTimeInMillis: context.getRemainingTimeInMillis()
      },
      environment: {
        NODE_ENV: process.env.NODE_ENV,
        hasSlackWebhook: !!process.env.SLACK_WEBHOOK_URL,
        hasSlackToken: !!process.env.SLACK_VERIFICATION_TOKEN,
        hasDatabaseUrl: !!process.env.DATABASE_URL,
        hasTimecampKey: !!process.env.TIMECAMP_API_KEY,
        cwd: process.cwd()
      },
      filesystem: {},
      goTest: {}
    };

    // Check filesystem
    try {
      const binaryPath = './bin/observe-yor-estimates';
      debugInfo.filesystem.binaryExists = fs.existsSync(binaryPath);
      
      if (debugInfo.filesystem.binaryExists) {
        const stats = fs.statSync(binaryPath);
        debugInfo.filesystem.binarySize = stats.size;
        debugInfo.filesystem.binaryMode = stats.mode.toString(8);
        debugInfo.filesystem.binaryModified = stats.mtime.toISOString();
      }

      // List files in current directory
      debugInfo.filesystem.rootFiles = fs.readdirSync('.').filter(f => !f.startsWith('.'));
      
      // List bin directory if exists
      if (fs.existsSync('./bin')) {
        debugInfo.filesystem.binFiles = fs.readdirSync('./bin');
      }

      // List netlify/functions directory
      if (fs.existsSync('./netlify/functions')) {
        debugInfo.filesystem.functionFiles = fs.readdirSync('./netlify/functions');
      }

    } catch (error) {
      debugInfo.filesystem.error = error.message;
    }

    // Test Go binary
    if (debugInfo.filesystem.binaryExists) {
      try {
        const result = await testGoBinary();
        debugInfo.goTest = result;
      } catch (error) {
        debugInfo.goTest.error = error.message;
      }
    }

    return {
      statusCode: 200,
      headers: { 
        'Content-Type': 'application/json',
        'Cache-Control': 'no-cache'
      },
      body: JSON.stringify(debugInfo, null, 2)
    };

  } catch (error) {
    console.error('Debug function error:', error);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        error: error.message,
        stack: error.stack,
        timestamp: new Date().toISOString()
      }, null, 2)
    };
  }
};

function testGoBinary() {
  return new Promise((resolve) => {
    console.log('Testing Go binary execution');
    
    const child = spawn('./bin/observe-yor-estimates', ['--help'], {
      cwd: process.cwd(),
      stdio: ['pipe', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    child.on('close', (code) => {
      resolve({
        success: code === 0,
        exitCode: code,
        stdout: stdout.substring(0, 500), // Limit output
        stderr: stderr.substring(0, 500)
      });
    });

    child.on('error', (error) => {
      resolve({
        success: false,
        spawnError: error.message
      });
    });

    // Timeout after 10 seconds
    setTimeout(() => {
      if (!child.killed) {
        child.kill('SIGTERM');
        resolve({
          success: false,
          timeout: true
        });
      }
    }, 10000);
  });
} 