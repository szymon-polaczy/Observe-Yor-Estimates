const { spawn } = require('child_process');

exports.handler = async (event, context) => {
  console.log('Slack update function called');
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
      user_name: params.get('user_name'),
      command: params.get('command'),
      text: params.get('text'),
      response_url: params.get('response_url')
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

    // Determine the period from command text or command name
    let period;
    const command = slackData.command;
    if (command && command.endsWith('-update') && command.length > 8) { // > '/-update'.length
        period = command.substring(1, command.length - '-update'.length);
    } else {
        period = slackData.text?.trim() || 'daily';
    }

    if (!['daily', 'weekly', 'monthly'].includes(period)) {
      console.log('Invalid period:', period);
      return {
        statusCode: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: `❌ Invalid period "${period}". Use: daily, weekly, or monthly`
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

    console.log(`Starting ${period} update background process`);

    // Send immediate response
    const immediateResponse = {
      response_type: 'ephemeral',
      text: `⏳ Preparing your ${period} update... I'll send the results shortly!`
    };

    // Start background job for actual processing
    setImmediate(() => {
      processUpdateInBackground(period, slackData.response_url, slackData.user_name);
    });

    console.log('Returning immediate response');
    return {
      statusCode: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(immediateResponse)
    };

  } catch (error) {
    console.error('Error in slack-update function:', error);
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

async function processUpdateInBackground(period, responseUrl, userName) {
  try {
    console.log(`Starting ${period} update background process for ${userName}`);

    // Add timeout to prevent infinite hanging
    const timeoutPromise = new Promise((_, reject) => {
      setTimeout(() => reject(new Error('Operation timed out after 3 minutes')), 3 * 60 * 1000);
    });

    // Execute the Go CLI tool
    const executionPromise = executeGoCommand('update', [period], {
      RESPONSE_URL: responseUrl,
      OUTPUT_JSON: 'true'
    });

    const result = await Promise.race([executionPromise, timeoutPromise]);

    if (!result.success) {
      console.error('Go command failed:', result.error);
      await sendErrorToSlack(responseUrl, `Failed to generate ${period} update: ${result.error}`);
    } else {
      console.log(`Successfully completed ${period} update for ${userName}`);
      await sendResponseToSlack(responseUrl, result.output);
    }

  } catch (error) {
    console.error('Error in background processing:', error);
    await sendErrorToSlack(responseUrl, `Background processing failed: ${error.message}`);
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

async function sendResponseToSlack(responseUrl, jsonOutput) {
  if (!responseUrl) {
    console.log('No response URL, cannot send Slack message.');
    return;
  }

  try {
    const message = JSON.parse(jsonOutput);
    const response_type = message.blocks ? 'in_channel' : 'ephemeral';

    const payload = {
      ...message,
      response_type: response_type,
    };
    
    console.log('Sending message to Slack:', JSON.stringify(payload, null, 2));

    const fetch = (await import('node-fetch')).default;
    const response = await fetch(responseUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (response.ok) {
      console.log('Successfully sent message to Slack.');
    } else {
      const errorBody = await response.text();
      console.error(`Failed to send message to Slack. Status: ${response.status}. Body: ${errorBody}`);
      await sendErrorToSlack(responseUrl, `Failed to send formatted message. Status: ${response.status}`);
    }
  } catch (error) {
    console.error('Error parsing or sending Slack message:', error);
    await sendErrorToSlack(responseUrl, `There was an error processing the update from the Go binary. Raw output: ${jsonOutput}`);
  }
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