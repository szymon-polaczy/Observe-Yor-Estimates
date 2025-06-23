const { spawn } = require('child_process');

exports.handler = async (event, context) => {
  console.log('Unified OYE function called');
  console.log('Event method:', event.httpMethod);
  
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
    
    // Parse the request
    const params = new URLSearchParams(event.body);
    
    const slackData = {
      token: params.get('token'),
      team_id: params.get('team_id'),
      channel_id: params.get('channel_id'),
      channel_name: params.get('channel_name'),
      user_id: params.get('user_id'),
      user_name: params.get('user_name'),
      command: params.get('command'),
      text: params.get('text'),
      response_url: params.get('response_url'),
      trigger_id: params.get('trigger_id')
    };

    console.log('Parsed Slack data:', {
      command: slackData.command,
      user_name: slackData.user_name,
      text: slackData.text,
      has_response_url: !!slackData.response_url
    });

    // Validate Slack token
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

    // Check if Go binary exists
    const fs = require('fs');
    const binaryPath = './bin/observe-yor-estimates';
    if (!fs.existsSync(binaryPath)) {
      console.error('Go binary not found at:', binaryPath);
      return {
        statusCode: 500,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: '❌ Server configuration error: binary not found'
        })
      };
    }

    console.log('Starting unified OYE command processing');

    // Check if we have bot token for direct API responses
    if (!process.env.SLACK_BOT_TOKEN) {
      console.error('SLACK_BOT_TOKEN not configured - this is required for the new system');
      return {
        statusCode: 500,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: '❌ Bot token not configured. Please add SLACK_BOT_TOKEN to environment variables.'
        })
      };
    }

    console.log('Using unified Go server system with bot token');
    
    // Send immediate response
    const immediateResponse = {
      response_type: 'ephemeral',
      text: '⏳ Processing your request... I\'ll respond directly with progress updates!'
    };

    // Start background process using Go server
    setImmediate(() => {
      processUnifiedCommandInBackground(slackData);
    });

    console.log('Returning immediate response');
    return {
      statusCode: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(immediateResponse)
    };

  } catch (error) {
    console.error('Error in unified OYE function:', error);
    console.error('Error stack:', error.stack);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: '❌ An error occurred while processing your request.'
      })
    };
  }
};

async function processUnifiedCommandInBackground(slackData) {
  try {
    console.log(`Processing unified OYE command for ${slackData.user_name}: ${slackData.text}`);

    // Parse command to determine action
    const text = (slackData.text || '').toLowerCase().trim();
    
    if (text.includes('sync') || text === 'full-sync') {
      await executeGoCommand('full-sync', [], {
        SLACK_BOT_TOKEN: process.env.SLACK_BOT_TOKEN,
        CHANNEL_ID: slackData.channel_id,
        USER_ID: slackData.user_id
      });
    } else {
      // Determine period
      let period = 'daily';
      if (text.includes('weekly')) period = 'weekly';
      else if (text.includes('monthly')) period = 'monthly';
      
      await executeGoCommand('update', [period], {
        SLACK_BOT_TOKEN: process.env.SLACK_BOT_TOKEN,
        CHANNEL_ID: slackData.channel_id,
        USER_ID: slackData.user_id
      });
    }

    console.log('Successfully processed unified command via Go CLI');

  } catch (error) {
    console.error('Error in unified command processing:', error);
    await sendErrorToSlack(slackData.response_url, `Processing failed: ${error.message}`);
  }
}

async function executeGoCommand(command, args = [], envVars = {}) {
  return new Promise((resolve) => {
    console.log(`Executing Go command: ${command} ${args.join(' ')}`);
    
    const env = { ...process.env, ...envVars };
    const child = spawn('./bin/observe-yor-estimates', [command, ...args], { 
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
        resolve({ success: false, error: 'Command timeout after 2 minutes' });
      }
    }, 2 * 60 * 1000); // 2 minute timeout
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