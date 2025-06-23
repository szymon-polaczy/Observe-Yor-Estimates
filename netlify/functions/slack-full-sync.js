const { spawn } = require('child_process');
const path = require('path');

exports.handler = async (event, context) => {
  console.log('Full sync function called');
  console.log('Event method:', event.httpMethod);
  console.log('Event headers:', JSON.stringify(event.headers));
  
  try {
    // Check HTTP method
    if (event.httpMethod !== 'POST') {
      console.log('Invalid HTTP method:', event.httpMethod);
      return {
        statusCode: 405,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ error: 'Method not allowed' })
      };
    }

    // Check if body exists
    if (!event.body) {
      console.log('No body in request');
      return {
        statusCode: 400,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: '❌ Invalid request: no body'
        })
      };
    }

    console.log('Request body length:', event.body.length);
    
    // Parse the request
    const params = new URLSearchParams(event.body);
    
    const slackData = {
      token: params.get('token'),
      team_id: params.get('team_id'),
      channel_id: params.get('channel_id'),
      user_name: params.get('user_name'),
      command: params.get('command'),
      text: params.get('text'),
      response_url: params.get('response_url')
    };

    console.log('Parsed Slack data:', {
      command: slackData.command,
      user_name: slackData.user_name,
      has_response_url: !!slackData.response_url,
      has_token: !!slackData.token
    });

    // Validate Slack token (if configured)
    const expectedToken = process.env.SLACK_VERIFICATION_TOKEN;
    if (expectedToken && slackData.token !== expectedToken) {
      console.log('Token validation failed');
      return {
        statusCode: 401,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: '❌ Invalid verification token'
        })
      };
    }

    // Check if Go binary exists with multiple possible paths
    const fs = require('fs');
    const possiblePaths = [
      './bin/observe-yor-estimates',
      path.join(process.cwd(), 'bin', 'observe-yor-estimates'),
      path.join(__dirname, '..', '..', 'bin', 'observe-yor-estimates'),
      '/var/task/bin/observe-yor-estimates'
    ];
    
    let binaryPath = null;
    for (const testPath of possiblePaths) {
      if (fs.existsSync(testPath)) {
        binaryPath = testPath;
        console.log('Found Go binary at:', binaryPath);
        break;
      }
    }
    
    if (!binaryPath) {
      console.error('Go binary not found at any of these paths:', possiblePaths);
      console.log('Current working directory:', process.cwd());
      console.log('__dirname:', __dirname);
      
      // List available files for debugging
      try {
        const rootFiles = fs.readdirSync(process.cwd());
        console.log('Files in cwd:', rootFiles);
        
        if (fs.existsSync('./bin')) {
          const binFiles = fs.readdirSync('./bin');
          console.log('Files in bin directory:', binFiles);
        }
      } catch (e) {
        console.log('Error listing files:', e.message);
      }
      
      return {
        statusCode: 500,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: '❌ Server configuration error: Go binary not found'
        })
      };
    }

    console.log('Go binary found, starting background process');

    // Send immediate response
    const immediateResponse = {
      response_type: 'ephemeral',
      text: '⏳ Starting full synchronization... This may take a few minutes. I\'ll notify you when complete!'
    };

    // Start background job for actual processing (don't await this)
    setImmediate(() => {
      processFullSyncInBackground(slackData.response_url, slackData.user_name, binaryPath);
    });

    console.log('Returning immediate response');
    return {
      statusCode: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(immediateResponse)
    };

  } catch (error) {
    console.error('Error in slack-full-sync function:', error);
    console.error('Error stack:', error.stack);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: '❌ An error occurred while starting the sync.'
      })
    };
  }
};

async function processFullSyncInBackground(responseUrl, userName, binaryPath) {
  try {
    console.log(`Starting full sync background process for ${userName}`);

    // Add timeout to prevent infinite hanging
    const timeoutPromise = new Promise((_, reject) => {
      setTimeout(() => reject(new Error('Operation timed out after 5 minutes')), 5 * 60 * 1000);
    });

    // Execute the Go CLI tool for full sync
    const executionPromise = executeGoCommand('full-sync', [], {
      RESPONSE_URL: responseUrl,
      OUTPUT_JSON: 'true'
    }, binaryPath);

    const result = await Promise.race([executionPromise, timeoutPromise]);

    if (!result.success) {
      console.error('Go command failed:', result.error);
      await sendErrorToSlack(responseUrl, `Full sync failed: ${result.error}`);
    } else {
      console.log(`Successfully completed full sync for ${userName}`);
      console.log('Go command output:', result.output);
    }

  } catch (error) {
    console.error('Error in full sync background processing:', error);
    await sendErrorToSlack(responseUrl, `Full sync failed: ${error.message}`);
  }
}

async function executeGoCommand(command, args = [], envVars = {}, binaryPath = './bin/observe-yor-estimates') {
  return new Promise((resolve) => {
    console.log(`Executing Go command: ${binaryPath} ${command} ${args.join(' ')}`);
    
    const env = { ...process.env, ...envVars };
    const child = spawn(binaryPath, [command, ...args], { 
      env,
      cwd: process.cwd(),
      stdio: ['pipe', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      const output = data.toString();
      stdout += output;
      console.log('Go stdout:', output);
    });

    child.stderr.on('data', (data) => {
      const output = data.toString();
      stderr += output;
      console.log('Go stderr:', output);
    });

    child.on('close', (code) => {
      console.log(`Go command exited with code: ${code}`);
      if (code === 0) {
        resolve({ success: true, output: stdout });
      } else {
        resolve({ success: false, error: stderr || `Exit code: ${code}` });
      }
    });

    child.on('error', (error) => {
      console.error('Go command spawn error:', error);
      resolve({ success: false, error: error.message });
    });

    // Add timeout for the child process
    setTimeout(() => {
      if (!child.killed) {
        console.log('Killing Go process due to timeout');
        child.kill('SIGTERM');
        resolve({ success: false, error: 'Command timeout after 4 minutes' });
      }
    }, 4 * 60 * 1000); // 4 minute timeout
  });
}

async function sendErrorToSlack(responseUrl, errorMessage) {
  if (!responseUrl) {
    console.log('No response URL provided for error message');
    return;
  }

  try {
    console.log('Sending error to Slack:', errorMessage);
    const fetch = (await import('node-fetch')).default;
    
    const response = await fetch(responseUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: `❌ ${errorMessage}`
      })
    });

    if (!response.ok) {
      console.error('Failed to send error to Slack:', response.status, await response.text());
    } else {
      console.log('Successfully sent error message to Slack');
    }
  } catch (error) {
    console.error('Error sending error message to Slack:', error);
  }
} 